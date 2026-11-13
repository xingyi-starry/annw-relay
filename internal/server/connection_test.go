package server

import (
	"net"
	"testing"
	"time"

	"mprelay/internal/wire"
)

func TestOutboundQueueCarriesEnvelopeAndAppliesBackpressure(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
	config := DefaultConfig()
	config.WriteQueueMessages = 1
	connection := newConnection(New(config, NewLogger(testDiscard{})), left)
	first := &wire.WireEnvelope{Body: &wire.WireEnvelope_Heartbeat{Heartbeat: &wire.Int64Value{Value: 101}}}
	second := &wire.WireEnvelope{Body: &wire.WireEnvelope_Heartbeat{Heartbeat: &wire.Int64Value{Value: 102}}}
	connection.enqueue(first)
	if cap(connection.send) != 1 || len(connection.send) != 1 {
		t.Fatalf("unexpected bounded queue state: cap=%d len=%d", cap(connection.send), len(connection.send))
	}
	done := make(chan struct{})
	go func() { connection.enqueue(second); close(done) }()
	select {
	case <-done:
		t.Fatal("enqueue returned instead of waiting for bounded channel capacity")
	case <-time.After(20 * time.Millisecond):
	}
	if got := <-connection.send; got.envelope != first {
		t.Fatal("queue did not preserve the original WireEnvelope")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked enqueue did not resume after capacity became available")
	}
	if got := <-connection.send; got.envelope != second {
		t.Fatal("queue did not preserve FIFO order")
	}
}

func TestDrainAndCloseWritesFinalEnvelopeBeforeClosing(t *testing.T) {
	left, right := net.Pipe()
	config := DefaultConfig()
	config.HeartbeatInterval = time.Hour
	server := New(config, NewLogger(testDiscard{}))
	peer := newConnection(server, left)
	server.conns[peer] = struct{}{}
	peer.start()
	final := &wire.WireEnvelope{Body: &wire.WireEnvelope_RelayControlResponse{RelayControlResponse: &wire.RelayControlResponse{RequestId: 777}}, Delivery: wire.Delivery_RELAY_CONTROL}
	peer.drainAndClose(final, disconnectExplicitLeave)
	got, err := readEnvelope(right, config.MaxPacketBytes)
	if err != nil {
		t.Fatalf("read final envelope: %v", err)
	}
	if envelopeType(got) != envelopeType(final) {
		t.Fatalf("unexpected final envelope: %d", envelopeType(got))
	}
	peer.wait()
	if connectionState(peer.state.Load()) != connectionClosed {
		t.Fatal("connection did not reach closed state")
	}
	_ = right.Close()
}

func TestServerCloseSkipsRuntimeDisconnectEffects(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = right.Close() })
	server := New(DefaultConfig(), NewLogger(testDiscard{}))
	peer := newConnection(server, left)
	roomID := make([]byte, 16)
	clientID := make([]byte, 16)
	clientID[0] = 1
	peer.bind(roleClient, clientID, roomID)
	server.conns[peer] = struct{}{}
	peer.start()
	server.rooms[string(roomID)] = &room{
		id: roomID, clients: map[string]*connection{string(clientID): peer},
		reservations: make(map[string]time.Time), matchID: make([]byte, 16),
		info: wire.RelayRoomInfo{Status: wire.RelayRoomStatus_RELAY_ROOM_PLAYING},
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if len(server.rooms) != 0 || len(server.conns) != 0 {
		t.Fatalf("shutdown retained runtime state: rooms=%d conns=%d", len(server.rooms), len(server.conns))
	}
}

type testDiscard struct{}

func (testDiscard) Write(data []byte) (int, error) { return len(data), nil }
