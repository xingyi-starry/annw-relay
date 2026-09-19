package server

import (
	"context"
	"errors"
	"io"
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

type connectionState uint32

const (
	connectionOpen connectionState = iota
	connectionDraining
	connectionClosed
)

type disconnectCause uint8

const (
	disconnectRemote disconnectCause = iota
	disconnectTimeout
	disconnectProtocolViolation
	disconnectWriteFailure
	disconnectExplicitLeave
	disconnectRoomClosed
	disconnectServerShutdown
)

type outbound struct {
	envelope   *wire.WireEnvelope
	closeAfter bool
	cause      disconnectCause
}

type connection struct {
	server *Server
	conn   net.Conn
	connID uint64

	mu        sync.RWMutex
	role      connectionRole
	id        []byte
	roomID    []byte
	send      chan outbound
	sendMu    sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	state     atomic.Uint32
	wg        sync.WaitGroup

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
	ctx, cancel := context.WithCancel(server.ctx)
	return &connection{
		server: server, conn: conn, connID: server.nextConnID.Add(1),
		send: make(chan outbound, server.config.WriteQueueMessages), ctx: ctx, cancel: cancel,
	}
}

func (c *connection) start() {
	c.wg.Add(2)
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

func (c *connection) enqueue(envelope *wire.WireEnvelope) {
	if envelope == nil {
		return
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if connectionState(c.state.Load()) != connectionOpen {
		return
	}
	select {
	case c.send <- outbound{envelope: envelope}:
	case <-c.ctx.Done():
	}
}

func (c *connection) drainAndClose(envelope *wire.WireEnvelope, cause disconnectCause) {
	if envelope == nil {
		return
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if !c.state.CompareAndSwap(uint32(connectionOpen), uint32(connectionDraining)) {
		return
	}
	_ = c.conn.SetReadDeadline(time.Now())
	select {
	case c.send <- outbound{envelope: envelope, closeAfter: true, cause: cause}:
	case <-c.ctx.Done():
	}
}

func (c *connection) readLoop() {
	defer c.wg.Done()
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.server.config.HeartbeatTimeout)); err != nil {
			c.abort(disconnectRemote)
			return
		}
		envelope, err := readEnvelope(c.conn, c.server.config.MaxPacketBytes)
		if err != nil {
			if connectionState(c.state.Load()) == connectionDraining {
				return
			}
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				c.server.logFor(c).WithField("event", "read_failed").WithError(err).Warn("connection read failed")
			}
			cause := disconnectRemote
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				cause = disconnectTimeout
			}
			c.abort(cause)
			return
		}
		if !c.allowMessage() {
			c.server.logFor(c).WithField("event", "rate_limit_exceeded").Warn("message rate limit exceeded")
			c.abort(disconnectProtocolViolation)
			return
		}
		if err := c.server.handleEnvelope(c, envelope); err != nil {
			c.server.logFor(c).WithField("event", "protocol_violation").WithError(err).Warn("invalid protocol message")
			c.abort(disconnectProtocolViolation)
			return
		}
	}
}

func (c *connection) writeLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.server.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case message := <-c.send:
			framed, err := marshalEnvelope(message.envelope, c.server.config.MaxPacketBytes)
			if err != nil {
				c.server.logFor(c).WithField("event", "encode_failed").WithError(err).Warn("outbound envelope encoding failed")
				c.abort(disconnectWriteFailure)
				return
			}
			if err := c.write(framed); err != nil {
				c.server.logFor(c).WithField("event", "write_failed").WithError(err).Warn("connection write failed")
				c.abort(disconnectWriteFailure)
				return
			}
			if message.closeAfter {
				c.finish(message.cause)
				return
			}
		case <-ticker.C:
			if connectionState(c.state.Load()) != connectionOpen {
				continue
			}
			heartbeat := &wire.WireEnvelope{MessageType: MessageRelayHeartbeat, Delivery: wire.Delivery_RELAY_CONTROL}
			framed, err := marshalEnvelope(heartbeat, c.server.config.MaxPacketBytes)
			if err != nil || c.write(framed) != nil {
				c.abort(disconnectWriteFailure)
				return
			}
		case <-c.ctx.Done():
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

func (c *connection) abort(cause disconnectCause) {
	c.state.Store(uint32(connectionClosed))
	c.finish(cause)
}

func (c *connection) finish(cause disconnectCause) {
	c.closeOnce.Do(func() {
		c.state.Store(uint32(connectionClosed))
		c.cancel()
		_ = c.conn.Close()
		c.server.onConnectionClosed(c, cause)
	})
}

func (c *connection) wait() { c.wg.Wait() }
