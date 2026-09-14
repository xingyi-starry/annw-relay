package server

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"mprelay/internal/wire"
)

type connectionRole uint8

const (
	roleUnbound connectionRole = iota
	roleHost
	roleClient
)

type sendItem struct {
	data       []byte
	closeAfter bool
}

type connection struct {
	server *Server
	conn   net.Conn
	logger *log.Logger

	mu          sync.RWMutex
	role        connectionRole
	id          []byte
	roomID      []byte
	send        chan sendItem
	closed      chan struct{}
	closeOnce   sync.Once
	queuedBytes atomic.Int64

	rateMu                sync.Mutex
	rateSecond            int64
	rateCount             int
	passwordAttemptSecond int64
}

func (c *connection) allowPasswordAttempt() bool {
	now := time.Now().Unix()
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	if c.passwordAttemptSecond == now {
		return false
	}
	c.passwordAttemptSecond = now
	return true
}

func newConnection(server *Server, conn net.Conn) *connection {
	return &connection{
		server: server, conn: conn, logger: server.logger,
		send: make(chan sendItem, 128), closed: make(chan struct{}),
	}
}

func (c *connection) start() {
	go c.writeLoop()
	go c.readLoop()
}

func (c *connection) binding() (connectionRole, []byte, []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.role, cloneBytes(c.id), cloneBytes(c.roomID)
}

func (c *connection) bind(role connectionRole, id, roomID []byte) {
	c.mu.Lock()
	c.role, c.id, c.roomID = role, cloneBytes(id), cloneBytes(roomID)
	c.mu.Unlock()
}

func (c *connection) unbind() {
	c.bind(roleUnbound, nil, nil)
}

func (c *connection) allowMessage() bool {
	now := time.Now().Unix()
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	if c.rateSecond != now {
		c.rateSecond, c.rateCount = now, 0
	}
	c.rateCount++
	return c.rateCount <= c.server.config.MaxMessagesPerSecond
}

func (c *connection) enqueue(envelope *wire.WireEnvelope) bool {
	return c.enqueueMode(envelope, false)
}

func (c *connection) sendAndClose(envelope *wire.WireEnvelope) {
	if !c.enqueueMode(envelope, true) {
		c.close()
	}
}

func (c *connection) enqueueMode(envelope *wire.WireEnvelope, closeAfter bool) bool {
	framed, err := marshalEnvelope(envelope, c.server.config.MaxPacketBytes)
	if err != nil {
		return false
	}
	size := int64(len(framed))
	for {
		current := c.queuedBytes.Load()
		if current+size > c.server.config.WriteQueueBytes {
			go c.close()
			return false
		}
		if c.queuedBytes.CompareAndSwap(current, current+size) {
			break
		}
	}
	select {
	case c.send <- sendItem{data: framed, closeAfter: closeAfter}:
		return true
	case <-c.closed:
		c.queuedBytes.Add(-size)
		return false
	default:
		c.queuedBytes.Add(-size)
		go c.close()
		return false
	}
}

func (c *connection) readLoop() {
	defer c.close()
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.server.config.HeartbeatTimeout)); err != nil {
			return
		}
		envelope, err := readEnvelope(c.conn, c.server.config.MaxPacketBytes)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				c.logger.Printf("read %s: %v", c.conn.RemoteAddr(), err)
			}
			return
		}
		if !c.allowMessage() {
			c.logger.Printf("rate limit exceeded by %s", c.conn.RemoteAddr())
			return
		}
		if err := c.server.handleEnvelope(c, envelope); err != nil {
			c.logger.Printf("protocol violation from %s: %v", c.conn.RemoteAddr(), err)
			return
		}
	}
}

func (c *connection) writeLoop() {
	ticker := time.NewTicker(c.server.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case item := <-c.send:
			if err := c.write(item.data); err != nil {
				c.close()
				return
			}
			c.queuedBytes.Add(-int64(len(item.data)))
			if item.closeAfter {
				c.close()
				return
			}
		case <-ticker.C:
			heartbeat := &wire.WireEnvelope{MessageType: MessageRelayHeartbeat, Delivery: wire.Delivery_RELAY_CONTROL}
			framed, err := marshalEnvelope(heartbeat, c.server.config.MaxPacketBytes)
			if err != nil || c.write(framed) != nil {
				c.close()
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *connection) write(data []byte) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.server.config.HeartbeatTimeout))
	for len(data) > 0 {
		n, err := c.conn.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func (c *connection) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
		c.server.onDisconnect(c)
	})
}
