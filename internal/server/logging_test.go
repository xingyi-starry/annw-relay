package server

import (
	"bytes"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"mprelay/internal/wire"
)

type safeLogBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *safeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}
func (b *safeLogBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.data.String() }

func TestLineFormatterFieldsAndEscaping(t *testing.T) {
	var output safeLogBuffer
	logger := NewLogger(&output)
	logger.WithFields(logrus.Fields{
		"event": "control_rejected", "room_id": "room", "client_id": "client",
		"match_id": "match", "request_id": 42, "reason": "invalid password\nsecret",
	}).Warn("control request rejected")
	line := output.String()
	if strings.Count(line, "\n") != 1 || !strings.Contains(line, "level=warning event=control_rejected room_id=room client_id=client match_id=match request_id=42") ||
		!strings.Contains(line, `reason="invalid password\nsecret"`) || !strings.Contains(line, "message=\"control request rejected\"") {
		t.Fatalf("unexpected log line: %q", line)
	}
	if !strings.Contains(line, "time=") || strings.Contains(line, "password=secret") {
		t.Fatalf("missing UTC timestamp or leaked sensitive field: %q", line)
	}
}

func TestFormatIDUsesCSharpGuidLayout(t *testing.T) {
	raw := []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	if got := formatID(raw); got != "00112233445566778899aabbccddeeff" {
		t.Fatalf("unexpected GUID: %s", got)
	}
}

func TestRoomLifecycleLogsIdentityWithoutPasswordOrPayload(t *testing.T) {
	var output safeLogBuffer
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.HeartbeatInterval = time.Hour
	config.HeartbeatTimeout = 5 * time.Second
	relay := New(config, NewLogger(&output))
	go func() { _ = relay.Serve(listener) }()
	t.Cleanup(func() { _ = relay.Close() })
	host := connectPeer(t, listener.Addr().String())
	roomID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	hostID := registerHost(t, host, roomID, "top-secret-credential")
	host.control(MessageRelayUpdateRoom, &wire.RelayUpdateRoomRequest{RequestId: 55, RoomId: roomID, Status: wire.RelayRoomStatus_RELAY_ROOM_WAITING}, roomID, hostID)
	_ = host.receiveType(MessageRelayControlResponse)
	line := output.String()
	if !strings.Contains(line, "event=room_created") || !strings.Contains(line, "room_id="+formatID(roomID)) ||
		!strings.Contains(line, "client_id="+formatID(hostID)) || !strings.Contains(line, "request_id=55") {
		t.Fatalf("missing lifecycle identity: %s", line)
	}
	if strings.Contains(line, "top-secret-credential") || strings.Contains(line, "payload=") {
		t.Fatalf("sensitive data appeared in log: %s", line)
	}
}
