package server

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"mprelay/internal/wire"
)

const (
	opaqueGameHello     uint32 = 1
	opaqueGameWelcome   uint32 = 2
	opaqueGameHeartbeat uint32 = 4
	opaqueGameRoomState uint32 = 5
)

type testPeer struct {
	t    *testing.T
	conn net.Conn
}

func startTestServer(t *testing.T) (string, *Server) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.HeartbeatInterval = time.Hour
	config.HeartbeatIntervalMS = 0
	config.HeartbeatTimeout = 5 * time.Second
	config.HeartbeatTimeoutMS = 0
	relay := New(config, NewLogger(io.Discard))
	go func() { _ = relay.Serve(listener) }()
	t.Cleanup(func() { _ = relay.Close() })
	return listener.Addr().String(), relay
}

func connectPeer(t *testing.T, address string) *testPeer {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	peer := &testPeer{t: t, conn: conn}
	t.Cleanup(func() { _ = conn.Close() })
	return peer
}

func (p *testPeer) send(envelope *wire.WireEnvelope) {
	p.t.Helper()
	framed, err := marshalEnvelope(envelope, 16*1024*1024)
	if err != nil {
		p.t.Fatal(err)
	}
	if _, err := p.conn.Write(framed); err != nil {
		p.t.Fatal(err)
	}
}

func (p *testPeer) control(messageType uint32, payload proto.Message, roomID, clientID []byte) {
	p.t.Helper()
	envelope := &wire.WireEnvelope{RoomId: cloneBytes(roomID), ClientId: cloneBytes(clientID), Delivery: wire.Delivery_RELAY_CONTROL}
	if err := setEnvelopeBody(envelope, messageType, payload); err != nil {
		p.t.Fatal(err)
	}
	p.send(envelope)
}

func (p *testPeer) receiveType(messageType uint32) *wire.WireEnvelope {
	p.t.Helper()
	_ = p.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		envelope, err := readEnvelope(p.conn, 16*1024*1024)
		if err != nil {
			p.t.Fatalf("receive message %d: %v", messageType, err)
		}
		if envelopeType(envelope) == messageType {
			return envelope
		}
	}
}

func responseFrom(t *testing.T, envelope *wire.WireEnvelope) *wire.RelayControlResponse {
	t.Helper()
	response := new(wire.RelayControlResponse)
	if err := unmarshalPayload(envelopeBody(envelope), response); err != nil {
		t.Fatal(err)
	}
	return response
}

func registerHost(t *testing.T, peer *testPeer, roomID []byte, password string) []byte {
	t.Helper()
	peer.control(MessageRelayRegisterRoom, &wire.RelayRegisterRoomRequest{RequestId: 1, RoomId: roomID, RoomName: "Test room", HostName: "Host", Password: password, PluginVersion: "test", GameFingerprint: "game", ContentFingerprint: "content", MaxParticipants: 4}, nil, nil)
	response := responseFrom(t, peer.receiveType(MessageRelayControlResponse))
	if !response.Success || len(response.ClientId) != 16 {
		t.Fatalf("register failed: %+v", response)
	}
	return response.ClientId
}

func joinClient(t *testing.T, peer *testPeer, roomID []byte, password string, requestID uint64) *wire.RelayControlResponse {
	t.Helper()
	peer.control(MessageRelayJoinRoom, &wire.RelayJoinRoomRequest{RequestId: requestID, RoomId: roomID, Password: password}, nil, nil)
	return responseFrom(t, peer.receiveType(MessageRelayControlResponse))
}

func TestRoomLifecycleAndRouting(t *testing.T) {
	address, _ := startTestServer(t)
	roomID := bytes.Repeat([]byte{0x11}, 16)
	host := connectPeer(t, address)
	hostID := registerHost(t, host, roomID, "secret")

	browser := connectPeer(t, address)
	browser.control(MessageRelayListRooms, &wire.RelayListRoomsRequest{RequestId: 2}, nil, nil)
	listEnvelope := browser.receiveType(MessageRelayRoomList)
	list := new(wire.RelayListRoomsResponse)
	if err := unmarshalPayload(envelopeBody(listEnvelope), list); err != nil {
		t.Fatal(err)
	}
	if len(list.Rooms) != 1 || !list.Rooms[0].HasPassword || list.Rooms[0].ConnectedPlayers != 1 || list.Rooms[0].MaxParticipants != 4 {
		t.Fatalf("unexpected room list: %+v", list.Rooms)
	}

	bad := connectPeer(t, address)
	if response := joinClient(t, bad, roomID, "wrong", 3); response.Success || response.Reason != "invalid password" {
		t.Fatalf("unexpected password response: %+v", response)
	}

	client := connectPeer(t, address)
	joined := joinClient(t, client, roomID, "secret", 4)
	if !joined.Success || len(joined.ClientId) != 16 {
		t.Fatalf("join failed: %+v", joined)
	}
	joinNotice := host.receiveType(MessageRelayPeerJoined)
	peerNotice := new(wire.RelayPeerNotice)
	if err := unmarshalPayload(envelopeBody(joinNotice), peerNotice); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(peerNotice.ClientId, joined.ClientId) {
		t.Fatal("host received wrong joined client id")
	}

	toHost := testGameEnvelope(opaqueGameHello, []byte("opaque"), roomID, joined.ClientId, nil, wire.Delivery_TO_HOST)
	client.send(toHost)
	forwarded := host.receiveType(opaqueGameHello)
	if !proto.Equal(toHost, forwarded) {
		t.Fatalf("TO_HOST envelope changed: %v", forwarded)
	}

	toClient := testGameEnvelope(opaqueGameWelcome, []byte("reply"), roomID, hostID, joined.ClientId, wire.Delivery_TO_CLIENT)
	host.send(toClient)
	if forwarded := client.receiveType(opaqueGameWelcome); !proto.Equal(toClient, forwarded) {
		t.Fatalf("TO_CLIENT envelope changed: %v", forwarded)
	}
	logicalHeartbeat := testGameEnvelope(opaqueGameHeartbeat, nil, roomID, joined.ClientId, nil, wire.Delivery_TO_HOST)
	client.send(logicalHeartbeat)
	if forwarded := host.receiveType(opaqueGameHeartbeat); !proto.Equal(logicalHeartbeat, forwarded) {
		t.Fatalf("TO_HOST heartbeat changed: %v", forwarded)
	}

	client2 := connectPeer(t, address)
	joined2 := joinClient(t, client2, roomID, "secret", 5)
	if !joined2.Success {
		t.Fatalf("second join failed: %+v", joined2)
	}
	_ = host.receiveType(MessageRelayPeerJoined)
	host.send(testGameEnvelope(opaqueGameWelcome, nil, roomID, hostID, joined2.ClientId, wire.Delivery_TO_CLIENT))
	_ = client2.receiveType(opaqueGameWelcome)
	broadcast := testGameEnvelope(opaqueGameRoomState, []byte("all"), roomID, hostID, nil, wire.Delivery_BROADCAST)
	host.send(broadcast)
	if !proto.Equal(broadcast, client.receiveType(opaqueGameRoomState)) || !proto.Equal(broadcast, client2.receiveType(opaqueGameRoomState)) {
		t.Fatal("broadcast was not preserved")
	}

	host.control(MessageRelayCloseJoining, &wire.RelayRoomRequest{RequestId: 6, RoomId: roomID}, roomID, hostID)
	if response := responseFrom(t, host.receiveType(MessageRelayControlResponse)); !response.Success {
		t.Fatalf("close joining failed: %+v", response)
	}
	host.control(MessageRelayUpdateRoom, &wire.RelayUpdateRoomRequest{RequestId: 61, RoomId: roomID, Status: wire.RelayRoomStatus_RELAY_ROOM_WAITING}, roomID, hostID)
	if response := responseFrom(t, host.receiveType(MessageRelayControlResponse)); response.Success {
		t.Fatal("room update reopened a closed room")
	}
	browser.control(MessageRelayListRooms, &wire.RelayListRoomsRequest{RequestId: 62}, nil, nil)
	closedList := new(wire.RelayListRoomsResponse)
	if err := unmarshalPayload(envelopeBody(browser.receiveType(MessageRelayRoomList)), closedList); err != nil {
		t.Fatal(err)
	}
	if len(closedList.Rooms) != 1 || closedList.Rooms[0].Status != wire.RelayRoomStatus_RELAY_ROOM_CLOSED {
		t.Fatalf("room status changed after rejected reopen: %+v", closedList.Rooms)
	}
	late := connectPeer(t, address)
	if response := joinClient(t, late, roomID, "secret", 7); response.Success {
		t.Fatal("closed room accepted a player")
	}

	host.control(MessageRelayCloseRoom, &wire.RelayRoomRequest{RequestId: 8, RoomId: roomID}, roomID, hostID)
	if response := responseFrom(t, host.receiveType(MessageRelayControlResponse)); !response.Success {
		t.Fatalf("close room failed: %+v", response)
	}
	for _, peer := range []*testPeer{client, client2} {
		notice := peer.receiveType(MessageRelayRoomClosed)
		if notice.Delivery != wire.Delivery_RELAY_CONTROL || !bytes.Equal(notice.RoomId, roomID) {
			t.Fatalf("bad close notice: %v", notice)
		}
	}
}

func TestRoomStatusTransitions(t *testing.T) {
	waiting := wire.RelayRoomStatus_RELAY_ROOM_WAITING
	closed := wire.RelayRoomStatus_RELAY_ROOM_CLOSED
	playing := wire.RelayRoomStatus_RELAY_ROOM_PLAYING
	tests := []struct {
		current wire.RelayRoomStatus
		next    wire.RelayRoomStatus
		allowed bool
	}{
		{waiting, waiting, true}, {waiting, closed, false}, {waiting, playing, true},
		{closed, waiting, false}, {closed, closed, true}, {closed, playing, true},
		{playing, waiting, true}, {playing, closed, false}, {playing, playing, true},
	}
	for _, test := range tests {
		if actual := roomStatusTransitionAllowed(test.current, test.next); actual != test.allowed {
			t.Fatalf("transition %s -> %s: got %v, want %v", test.current, test.next, actual, test.allowed)
		}
	}
}

func TestFinishedRoomReturnsToLobbyAndStartsAnotherMatch(t *testing.T) {
	address, relay := startTestServer(t)
	roomID := bytes.Repeat([]byte{0x81}, 16)
	firstMatch := bytes.Repeat([]byte{0x82}, 16)
	secondMatch := bytes.Repeat([]byte{0x83}, 16)
	host := connectPeer(t, address)
	hostID := registerHost(t, host, roomID, "")
	client := connectPeer(t, address)
	joined := joinClient(t, client, roomID, "", 2)
	if !joined.Success {
		t.Fatal("join failed")
	}
	_ = host.receiveType(MessageRelayPeerJoined)
	update := func(requestID uint64, status wire.RelayRoomStatus, matchID []byte) {
		host.control(MessageRelayUpdateRoom, &wire.RelayUpdateRoomRequest{RequestId: requestID, RoomId: roomID,
			Status: status, MatchId: matchID, MaxParticipants: 4}, roomID, hostID)
		if response := responseFrom(t, host.receiveType(MessageRelayControlResponse)); !response.Success {
			t.Fatalf("room update failed: %+v", response)
		}
	}
	update(3, wire.RelayRoomStatus_RELAY_ROOM_PLAYING, firstMatch)
	dropped := connectPeer(t, address)
	droppedJoin := joinClient(t, dropped, roomID, "", 4)
	if !droppedJoin.Success {
		t.Fatal("second join failed")
	}
	_ = host.receiveType(MessageRelayPeerJoined)
	_ = dropped.conn.Close()
	_ = host.receiveType(MessageRelayPeerLeft)
	update(5, wire.RelayRoomStatus_RELAY_ROOM_WAITING, nil)
	relay.mu.RLock()
	r := relay.rooms[string(roomID)]
	clean := r != nil && len(r.matchID) == 0 && len(r.reservations) == 0 && len(r.clients) == 1
	relay.mu.RUnlock()
	if !clean {
		t.Fatal("finished room retained its old match or reservations, or lost the active client")
	}
	client.send(testGameEnvelope(opaqueGameRoomState, []byte("still-here"), roomID, joined.ClientId, nil, wire.Delivery_TO_HOST))
	if got := host.receiveType(opaqueGameRoomState); got.GetRoomState().MapTitle != "still-here" {
		t.Fatal("existing connection was lost")
	}
	update(6, wire.RelayRoomStatus_RELAY_ROOM_PLAYING, secondMatch)
	resuming := connectPeer(t, address)
	resuming.control(MessageRelayResumeRoom, &wire.RelayResumeRoomRequest{RequestId: 7, RoomId: roomID,
		ClientId: droppedJoin.ClientId, MatchId: firstMatch}, nil, nil)
	if response := responseFrom(t, resuming.receiveType(MessageRelayControlResponse)); response.Success {
		t.Fatal("previous match was resumable after a new game started")
	}
	host.send(testGameEnvelope(opaqueGameWelcome, []byte("next-game"), roomID, hostID, joined.ClientId, wire.Delivery_TO_CLIENT))
	if got := client.receiveType(opaqueGameWelcome); string(got.GetWelcome().ClientId) != "next-game" {
		t.Fatal("client identity changed between matches")
	}
}

func TestRelayControlHeartbeatKeepsUnboundConnectionUsable(t *testing.T) {
	address, _ := startTestServer(t)
	peer := connectPeer(t, address)
	peer.send(testGameEnvelope(MessageRelayHeartbeat, nil, nil, nil, nil, wire.Delivery_RELAY_CONTROL))
	peer.control(MessageRelayListRooms, &wire.RelayListRoomsRequest{RequestId: 30}, nil, nil)
	list := new(wire.RelayListRoomsResponse)
	if err := unmarshalPayload(envelopeBody(peer.receiveType(MessageRelayRoomList)), list); err != nil {
		t.Fatal(err)
	}
	if list.RequestId != 30 {
		t.Fatalf("unexpected list response request id %d", list.RequestId)
	}
}

func TestPlayingJoinTransparentRoutingAndFastReconnect(t *testing.T) {
	address, relay := startTestServer(t)
	relay.config.ReconnectGrace = 120 * time.Millisecond
	roomID := bytes.Repeat([]byte{0x71}, 16)
	matchID := bytes.Repeat([]byte{0x72}, 16)
	host := connectPeer(t, address)
	hostID := registerHost(t, host, roomID, "")
	host.control(MessageRelayUpdateRoom, &wire.RelayUpdateRoomRequest{RequestId: 70, RoomId: roomID, Status: wire.RelayRoomStatus_RELAY_ROOM_PLAYING, AvailableSeats: 0, MaxParticipants: 4, MatchId: matchID}, roomID, hostID)
	if response := responseFrom(t, host.receiveType(MessageRelayControlResponse)); !response.Success {
		t.Fatalf("playing update failed: %+v", response)
	}

	client := connectPeer(t, address)
	joined := joinClient(t, client, roomID, "", 71)
	if !joined.Success {
		t.Fatalf("playing room rejected spectator join when no seat was available: %+v", joined)
	}
	_ = host.receiveType(MessageRelayPeerJoined)
	unknown := testGameEnvelope(44, []byte("future-game-message"), roomID, joined.ClientId, nil, wire.Delivery_TO_HOST)
	client.send(unknown)
	if forwarded := host.receiveType(44); !proto.Equal(unknown, forwarded) {
		t.Fatal("game message was not forwarded unchanged")
	}

	_ = client.conn.Close()
	disconnected := new(wire.RelayPeerNotice)
	if err := unmarshalPayload(envelopeBody(host.receiveType(MessageRelayPeerLeft)), disconnected); err != nil {
		t.Fatal(err)
	}
	if disconnected.Reason != "reconnecting" {
		t.Fatalf("unexpected initial disconnect reason %q", disconnected.Reason)
	}

	resumed := connectPeer(t, address)
	resumed.control(MessageRelayResumeRoom, &wire.RelayResumeRoomRequest{RequestId: 72, RoomId: roomID, ClientId: joined.ClientId, MatchId: matchID}, nil, nil)
	if response := responseFrom(t, resumed.receiveType(MessageRelayControlResponse)); !response.Success || !bytes.Equal(response.ClientId, joined.ClientId) {
		t.Fatalf("resume failed: %+v", response)
	}
	_ = host.receiveType(MessageRelayPeerJoined)

	hijacker := connectPeer(t, address)
	hijacker.control(MessageRelayResumeRoom, &wire.RelayResumeRoomRequest{RequestId: 73, RoomId: roomID, ClientId: joined.ClientId, MatchId: matchID}, nil, nil)
	if response := responseFrom(t, hijacker.receiveType(MessageRelayControlResponse)); response.Success {
		t.Fatal("active connection was hijacked")
	}

	_ = resumed.conn.Close()
	if notice := host.receiveType(MessageRelayPeerLeft); func() string {
		value := new(wire.RelayPeerNotice)
		_ = unmarshalPayload(envelopeBody(notice), value)
		return value.Reason
	}() != "reconnecting" {
		t.Fatal("resumed disconnect did not enter grace")
	}
	timedOut := new(wire.RelayPeerNotice)
	if err := unmarshalPayload(envelopeBody(host.receiveType(MessageRelayPeerLeft)), timedOut); err != nil {
		t.Fatal(err)
	}
	if timedOut.Reason != "reconnect timeout" {
		t.Fatalf("unexpected expiry reason %q", timedOut.Reason)
	}
	expired := connectPeer(t, address)
	expired.control(MessageRelayResumeRoom, &wire.RelayResumeRoomRequest{RequestId: 74, RoomId: roomID, ClientId: joined.ClientId, MatchId: matchID}, nil, nil)
	if response := responseFrom(t, expired.receiveType(MessageRelayControlResponse)); response.Success {
		t.Fatal("expired reservation resumed")
	}

	leaver := connectPeer(t, address)
	leftJoin := joinClient(t, leaver, roomID, "", 75)
	if !leftJoin.Success {
		t.Fatal("leave-test join failed")
	}
	_ = host.receiveType(MessageRelayPeerJoined)
	leaver.control(MessageRelayLeaveRoom, &wire.RelayRoomRequest{RequestId: 76, RoomId: roomID}, roomID, leftJoin.ClientId)
	if response := responseFrom(t, leaver.receiveType(MessageRelayControlResponse)); !response.Success {
		t.Fatal("explicit leave failed")
	}
	left := new(wire.RelayPeerNotice)
	if err := unmarshalPayload(envelopeBody(host.receiveType(MessageRelayPeerLeft)), left); err != nil {
		t.Fatal(err)
	}
	if left.Reason != "left" {
		t.Fatalf("explicit leave was not immediate: %q", left.Reason)
	}
}

func TestChineseRoomMetadataCharacterLimits(t *testing.T) {
	address, _ := startTestServer(t)
	roomID := bytes.Repeat([]byte{0x41}, 16)
	host := connectPeer(t, address)
	host.control(MessageRelayRegisterRoom, &wire.RelayRegisterRoomRequest{
		RequestId: 40,
		RoomId:    roomID,
		RoomName:  strings.Repeat("房", 48),
		HostName:  strings.Repeat("主", 32),
		Password:  strings.Repeat("😀", 64),
	}, nil, nil)
	registered := responseFrom(t, host.receiveType(MessageRelayControlResponse))
	if !registered.Success {
		t.Fatalf("Chinese boundary metadata was rejected: %+v", registered)
	}

	host.control(MessageRelayUpdateRoom, &wire.RelayUpdateRoomRequest{RequestId: 41, RoomId: roomID, MapTitle: strings.Repeat("图", 128), Status: wire.RelayRoomStatus_RELAY_ROOM_WAITING}, roomID, registered.ClientId)
	if response := responseFrom(t, host.receiveType(MessageRelayControlResponse)); !response.Success {
		t.Fatalf("128-character map title was rejected: %+v", response)
	}
	host.control(MessageRelayUpdateRoom, &wire.RelayUpdateRoomRequest{RequestId: 42, RoomId: roomID, MapTitle: strings.Repeat("图", 129), Status: wire.RelayRoomStatus_RELAY_ROOM_WAITING}, roomID, registered.ClientId)
	if response := responseFrom(t, host.receiveType(MessageRelayControlResponse)); response.Success {
		t.Fatal("129-character map title was accepted")
	}

	tests := []struct {
		name     string
		roomName string
		hostName string
		password string
	}{
		{name: "room name", roomName: strings.Repeat("房", 49), hostName: "主机"},
		{name: "host name", roomName: "房间", hostName: strings.Repeat("主", 33)},
		{name: "password bytes", roomName: "房间", hostName: "主机", password: strings.Repeat("😀", 65)},
	}
	for i, test := range tests {
		peer := connectPeer(t, address)
		peer.control(MessageRelayRegisterRoom, &wire.RelayRegisterRoomRequest{
			RequestId: uint64(50 + i),
			RoomId:    bytes.Repeat([]byte{byte(0x50 + i)}, 16),
			RoomName:  test.roomName,
			HostName:  test.hostName,
			Password:  test.password,
		}, nil, nil)
		if response := responseFrom(t, peer.receiveType(MessageRelayControlResponse)); response.Success {
			t.Fatalf("over-limit %s was accepted", test.name)
		}
	}
}

func TestIdentityForgeryAndRoomIsolation(t *testing.T) {
	address, _ := startTestServer(t)
	roomA := bytes.Repeat([]byte{0x21}, 16)
	roomB := bytes.Repeat([]byte{0x22}, 16)
	hostA := connectPeer(t, address)
	_ = registerHost(t, hostA, roomA, "")
	hostB := connectPeer(t, address)
	_ = registerHost(t, hostB, roomB, "")
	clientA := connectPeer(t, address)
	joinedA := joinClient(t, clientA, roomA, "", 10)
	_ = hostA.receiveType(MessageRelayPeerJoined)

	clientA.send(testGameEnvelope(opaqueGameHello, nil, roomB, joinedA.ClientId, nil, wire.Delivery_TO_HOST))
	_ = clientA.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := readEnvelope(clientA.conn, 16*1024*1024); err == nil {
		t.Fatal("forged cross-room envelope did not disconnect sender")
	}

	hostB.conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if envelope, err := readEnvelope(hostB.conn, 16*1024*1024); err == nil && envelopeType(envelope) == opaqueGameHello {
		t.Fatal("cross-room message reached other host")
	}
}

func TestHostDisconnectClosesRoomAndNotifiesClients(t *testing.T) {
	address, _ := startTestServer(t)
	roomID := bytes.Repeat([]byte{0x31}, 16)
	host := connectPeer(t, address)
	_ = registerHost(t, host, roomID, "")
	client := connectPeer(t, address)
	if response := joinClient(t, client, roomID, "", 20); !response.Success {
		t.Fatal("join failed")
	}
	_ = host.receiveType(MessageRelayPeerJoined)
	_ = host.conn.Close()
	notice := client.receiveType(MessageRelayRoomClosed)
	payload := new(wire.RelayPeerNotice)
	if err := unmarshalPayload(envelopeBody(notice), payload); err != nil {
		t.Fatal(err)
	}
	if payload.Reason != "host disconnected" {
		t.Fatalf("unexpected reason %q", payload.Reason)
	}

	browser := connectPeer(t, address)
	browser.control(MessageRelayListRooms, &wire.RelayListRoomsRequest{RequestId: 21}, nil, nil)
	list := new(wire.RelayListRoomsResponse)
	if err := unmarshalPayload(envelopeBody(browser.receiveType(MessageRelayRoomList)), list); err != nil {
		t.Fatal(err)
	}
	if len(list.Rooms) != 0 {
		t.Fatal("host-disconnected room remained listed")
	}
}

func TestFramingRejectsInvalidLengths(t *testing.T) {
	for _, length := range []uint32{0, 1025} {
		var data bytes.Buffer
		if err := binary.Write(&data, binary.LittleEndian, length); err != nil {
			t.Fatal(err)
		}
		if _, err := readEnvelope(&data, 1024); err == nil {
			t.Fatalf("length %d was accepted", length)
		}
	}
}

func TestPasswordVerifier(t *testing.T) {
	verifier, err := derivePassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !verifier.enabled() || !verifier.matches("secret") || verifier.matches("other") {
		t.Fatal("password verifier mismatch")
	}
	if bytes.Equal(verifier.hash, []byte("secret")) {
		t.Fatal("password was stored verbatim")
	}
}

func testGameEnvelope(kind uint32, data, room, client, target []byte, delivery wire.Delivery) *wire.WireEnvelope {
	envelope := &wire.WireEnvelope{RoomId: room, ClientId: client, TargetClientId: target, Delivery: delivery}
	var body proto.Message
	switch kind {
	case 1:
		body = &wire.Hello{DisplayName: string(data)}
	case 2:
		body = &wire.Welcome{ClientId: data}
	case 4:
		body = &wire.Int64Value{}
	case 5:
		body = &wire.Room{MapTitle: string(data)}
	case 44:
		body = &wire.ChatEventMessage{Text: string(data)}
	default:
		panic("unsupported test envelope")
	}
	if err := setEnvelopeBody(envelope, kind, body); err != nil {
		panic(err)
	}
	return envelope
}
