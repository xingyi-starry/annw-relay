package server

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"mprelay/internal/wire"
)

type room struct {
	id       []byte
	hostID   []byte
	host     *connection
	clients  map[string]*connection
	password passwordVerifier
	info     wire.RelayRoomInfo
}

type Server struct {
	config   Config
	logger   *log.Logger
	mu       sync.RWMutex
	rooms    map[string]*room
	conns    map[*connection]struct{}
	listener net.Listener
	closed   bool
}

const (
	maxRoomNameRunes    = 48
	maxHostNameRunes    = 32
	maxMapTitleRunes    = 128
	maxVersionBytes     = 80
	maxFingerprintBytes = 256
	maxPasswordBytes    = 256
)

func New(config Config, logger *log.Logger) *Server {
	config.normalize()
	if logger == nil {
		logger = log.Default()
	}
	return &Server{config: config, logger: logger, rooms: make(map[string]*room), conns: make(map[*connection]struct{})}
}

func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	if s.listener != nil {
		s.mu.Unlock()
		return errors.New("server already started")
	}
	s.listener = listener
	s.mu.Unlock()
	for {
		conn, err := listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()
			if closed || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		peer := newConnection(s, conn)
		s.mu.Lock()
		if s.closed || len(s.conns) >= s.config.MaxConnections {
			s.mu.Unlock()
			_ = conn.Close()
			continue
		}
		s.conns[peer] = struct{}{}
		s.mu.Unlock()
		peer.start()
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	listener := s.listener
	peers := make([]*connection, 0, len(s.conns))
	for peer := range s.conns {
		peers = append(peers, peer)
	}
	s.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	for _, peer := range peers {
		peer.close()
	}
	return nil
}

func (s *Server) handleEnvelope(c *connection, envelope *wire.WireEnvelope) error {
	if envelope.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("protocol version %d is unsupported", envelope.ProtocolVersion)
	}
	if envelope.MessageType == MessageHeartbeat && envelope.Delivery == wire.Delivery_RELAY_CONTROL {
		return nil
	}
	if envelope.Delivery == wire.Delivery_RELAY_CONTROL {
		return s.handleControl(c, envelope)
	}
	if isRelayMessage(envelope.MessageType) {
		return errors.New("relay control message has routed delivery")
	}
	if !isGameMessage(envelope.MessageType) {
		return fmt.Errorf("unknown game message type %d", envelope.MessageType)
	}
	return s.routeGameMessage(c, envelope)
}

func (s *Server) handleControl(c *connection, envelope *wire.WireEnvelope) error {
	switch envelope.MessageType {
	case MessageRelayRegisterRoom:
		return s.registerRoom(c, envelope)
	case MessageRelayUpdateRoom:
		return s.updateRoom(c, envelope)
	case MessageRelayListRooms:
		return s.listRooms(c, envelope)
	case MessageRelayJoinRoom:
		return s.joinRoom(c, envelope)
	case MessageRelayCloseJoining:
		return s.closeJoining(c, envelope)
	case MessageRelayLeaveRoom:
		return s.leaveRoom(c, envelope)
	case MessageRelayCloseRoom:
		return s.closeRoomRequest(c, envelope)
	default:
		return fmt.Errorf("control message type %d is not accepted", envelope.MessageType)
	}
}

func (s *Server) registerRoom(c *connection, envelope *wire.WireEnvelope) error {
	if role, _, _ := c.binding(); role != roleUnbound {
		return errors.New("connection is already bound")
	}
	if len(envelope.RoomId) != 0 || len(envelope.ClientId) != 0 || len(envelope.TargetClientId) != 0 {
		return errors.New("unbound register envelope contains identity")
	}
	req := new(wire.RelayRegisterRoomRequest)
	if err := unmarshalPayload(envelope.Payload, req); err != nil {
		return err
	}
	if err := validateID(req.RoomId, "room id"); err != nil {
		return s.controlFailure(c, req.RequestId, err.Error(), nil)
	}
	roomName := strings.TrimSpace(req.RoomName)
	hostName := strings.TrimSpace(req.HostName)
	if roomName == "" || utf8.RuneCountInString(roomName) > maxRoomNameRunes || hostName == "" || utf8.RuneCountInString(hostName) > maxHostNameRunes || len(req.Password) > maxPasswordBytes || len(req.PluginVersion) > maxVersionBytes || len(req.GameFingerprint) > maxFingerprintBytes || len(req.ContentFingerprint) > maxFingerprintBytes {
		return s.controlFailure(c, req.RequestId, "invalid room or host name", nil)
	}
	verifier, err := derivePassword(req.Password)
	if err != nil {
		return err
	}
	hostID, err := randomID()
	if err != nil {
		return err
	}
	key := string(req.RoomId)
	s.mu.Lock()
	if len(s.rooms) >= s.config.MaxRooms {
		s.mu.Unlock()
		return s.controlFailure(c, req.RequestId, "room limit reached", nil)
	}
	if _, exists := s.rooms[key]; exists {
		s.mu.Unlock()
		return s.controlFailure(c, req.RequestId, "room id already exists", nil)
	}
	r := &room{id: cloneBytes(req.RoomId), hostID: hostID, host: c, clients: make(map[string]*connection), password: verifier}
	r.info = wire.RelayRoomInfo{RoomId: cloneBytes(req.RoomId), RoomName: roomName, HostName: hostName, ConnectedPlayers: 1, HasPassword: verifier.enabled(), Status: wire.RelayRoomStatus_RELAY_ROOM_WAITING, PluginVersion: req.PluginVersion, GameFingerprint: req.GameFingerprint, ContentFingerprint: req.ContentFingerprint}
	s.rooms[key] = r
	c.bind(roleHost, hostID, req.RoomId)
	s.mu.Unlock()
	return s.controlSuccess(c, req.RequestId, hostID, req.RoomId)
}

func (s *Server) updateRoom(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayUpdateRoomRequest)
	if err := unmarshalPayload(envelope.Payload, req); err != nil {
		return err
	}
	r, err := s.boundHost(c, envelope, req.RoomId)
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(req.MapTitle) > maxMapTitleRunes || req.HumanSeats < 0 {
		return s.controlFailure(c, req.RequestId, "invalid room update", r.id)
	}
	if req.Status < wire.RelayRoomStatus_RELAY_ROOM_WAITING || req.Status > wire.RelayRoomStatus_RELAY_ROOM_PLAYING {
		return s.controlFailure(c, req.RequestId, "invalid room status", r.id)
	}
	s.mu.Lock()
	if current := s.rooms[string(r.id)]; current != r {
		s.mu.Unlock()
		return errors.New("host room is missing")
	}
	if !roomStatusTransitionAllowed(r.info.Status, req.Status) {
		s.mu.Unlock()
		return s.controlFailure(c, req.RequestId, "room status transition is not allowed", r.id)
	}
	r.info.MapTitle = req.MapTitle
	r.info.HumanSeats = req.HumanSeats
	r.info.Status = req.Status
	r.info.ConnectedPlayers = int32(1 + len(r.clients))
	s.mu.Unlock()
	return s.controlSuccess(c, req.RequestId, nil, r.id)
}

func roomStatusTransitionAllowed(current, next wire.RelayRoomStatus) bool {
	switch current {
	case wire.RelayRoomStatus_RELAY_ROOM_WAITING:
		return next == wire.RelayRoomStatus_RELAY_ROOM_WAITING || next == wire.RelayRoomStatus_RELAY_ROOM_PLAYING
	case wire.RelayRoomStatus_RELAY_ROOM_CLOSED:
		return next == wire.RelayRoomStatus_RELAY_ROOM_CLOSED || next == wire.RelayRoomStatus_RELAY_ROOM_PLAYING
	case wire.RelayRoomStatus_RELAY_ROOM_PLAYING:
		return next == wire.RelayRoomStatus_RELAY_ROOM_PLAYING
	default:
		return false
	}
}

func (s *Server) listRooms(c *connection, envelope *wire.WireEnvelope) error {
	if len(envelope.RoomId) != 0 || len(envelope.ClientId) != 0 || len(envelope.TargetClientId) != 0 {
		return errors.New("list envelope contains routing identity")
	}
	req := new(wire.RelayListRoomsRequest)
	if err := unmarshalPayload(envelope.Payload, req); err != nil {
		return err
	}
	response := &wire.RelayListRoomsResponse{RequestId: req.RequestId}
	s.mu.RLock()
	for _, r := range s.rooms {
		info := proto.Clone(&r.info).(*wire.RelayRoomInfo)
		info.ConnectedPlayers = int32(1 + len(r.clients))
		response.Rooms = append(response.Rooms, info)
	}
	s.mu.RUnlock()
	sort.Slice(response.Rooms, func(i, j int) bool { return bytes.Compare(response.Rooms[i].RoomId, response.Rooms[j].RoomId) < 0 })
	return s.sendControl(c, MessageRelayRoomList, response, nil)
}

func (s *Server) joinRoom(c *connection, envelope *wire.WireEnvelope) error {
	if role, _, _ := c.binding(); role != roleUnbound {
		return errors.New("connection is already bound")
	}
	if len(envelope.RoomId) != 0 || len(envelope.ClientId) != 0 || len(envelope.TargetClientId) != 0 {
		return errors.New("unbound join envelope contains identity")
	}
	req := new(wire.RelayJoinRoomRequest)
	if err := unmarshalPayload(envelope.Payload, req); err != nil {
		return err
	}
	if err := validateID(req.RoomId, "room id"); err != nil {
		return s.controlFailure(c, req.RequestId, err.Error(), nil)
	}
	if len(req.Password) > maxPasswordBytes {
		return s.controlFailure(c, req.RequestId, "password is too long", req.RoomId)
	}
	if !c.allowPasswordAttempt() {
		return s.controlFailure(c, req.RequestId, "password attempts are rate limited", req.RoomId)
	}
	clientID, err := randomID()
	if err != nil {
		return err
	}
	s.mu.RLock()
	r := s.rooms[string(req.RoomId)]
	if r == nil {
		s.mu.RUnlock()
		return s.controlFailure(c, req.RequestId, "room not found", nil)
	}
	status := r.info.Status
	verifier := r.password
	roomID := cloneBytes(r.id)
	s.mu.RUnlock()
	if status != wire.RelayRoomStatus_RELAY_ROOM_WAITING {
		return s.controlFailure(c, req.RequestId, "room is not accepting players", roomID)
	}
	if !verifier.matches(req.Password) {
		return s.controlFailure(c, req.RequestId, "invalid password", roomID)
	}
	s.mu.Lock()
	r = s.rooms[string(req.RoomId)]
	if r == nil || r.info.Status != wire.RelayRoomStatus_RELAY_ROOM_WAITING {
		s.mu.Unlock()
		return s.controlFailure(c, req.RequestId, "room is not accepting players", roomID)
	}
	r.clients[string(clientID)] = c
	r.info.ConnectedPlayers = int32(1 + len(r.clients))
	c.bind(roleClient, clientID, r.id)
	host := r.host
	roomID = cloneBytes(r.id)
	s.mu.Unlock()
	if err := s.controlSuccess(c, req.RequestId, clientID, roomID); err != nil {
		return err
	}
	notice := &wire.RelayPeerNotice{ClientId: cloneBytes(clientID), Reason: "joined"}
	return s.sendControl(host, MessageRelayPeerJoined, notice, roomID)
}

func (s *Server) closeJoining(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayRoomRequest)
	if err := unmarshalPayload(envelope.Payload, req); err != nil {
		return err
	}
	r, err := s.boundHost(c, envelope, req.RoomId)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.rooms[string(r.id)] == r {
		r.info.Status = wire.RelayRoomStatus_RELAY_ROOM_CLOSED
	}
	s.mu.Unlock()
	return s.controlSuccess(c, req.RequestId, nil, r.id)
}

func (s *Server) leaveRoom(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayRoomRequest)
	if err := unmarshalPayload(envelope.Payload, req); err != nil {
		return err
	}
	role, _, roomID := c.binding()
	if role == roleUnbound || !bytes.Equal(roomID, req.RoomId) || !validBoundEnvelope(envelope, roomID, c) {
		return errors.New("leave request does not match binding")
	}
	if role == roleHost {
		return s.closeRoom(c, req.RequestId, "host closed room")
	}
	response, err := makeControlResponse(req.RequestId, true, "", nil, roomID)
	if err != nil {
		return err
	}
	c.sendAndClose(response)
	return nil
}

func (s *Server) closeRoomRequest(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayRoomRequest)
	if err := unmarshalPayload(envelope.Payload, req); err != nil {
		return err
	}
	if _, err := s.boundHost(c, envelope, req.RoomId); err != nil {
		return err
	}
	return s.closeRoom(c, req.RequestId, "host closed room")
}

func (s *Server) closeRoom(host *connection, requestID uint64, reason string) error {
	role, _, roomID := host.binding()
	if role != roleHost {
		return errors.New("connection is not a host")
	}
	s.mu.Lock()
	r := s.rooms[string(roomID)]
	if r == nil || r.host != host {
		s.mu.Unlock()
		return errors.New("host room is missing")
	}
	delete(s.rooms, string(roomID))
	clients := make([]*connection, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	host.unbind()
	s.mu.Unlock()
	if err := s.controlSuccess(host, requestID, nil, roomID); err != nil {
		return err
	}
	for _, client := range clients {
		s.notifyRoomClosed(client, reason, roomID)
	}
	return nil
}

func (s *Server) routeGameMessage(c *connection, envelope *wire.WireEnvelope) error {
	role, id, roomID := c.binding()
	if role == roleUnbound || !bytes.Equal(envelope.RoomId, roomID) || !bytes.Equal(envelope.ClientId, id) {
		return errors.New("routed envelope identity does not match connection")
	}
	s.mu.RLock()
	r := s.rooms[string(roomID)]
	if r == nil {
		s.mu.RUnlock()
		return errors.New("room is not registered")
	}
	switch role {
	case roleClient:
		if envelope.Delivery != wire.Delivery_TO_HOST || len(envelope.TargetClientId) != 0 {
			s.mu.RUnlock()
			return errors.New("client may only send TO_HOST")
		}
		if !c.isGameReady() && envelope.MessageType != MessageHello {
			s.mu.RUnlock()
			return errors.New("client game handshake is incomplete")
		}
		host := r.host
		s.mu.RUnlock()
		if !host.enqueue(envelope) {
			return errors.New("host write queue unavailable")
		}
		return nil
	case roleHost:
		if envelope.Delivery == wire.Delivery_TO_CLIENT {
			if len(envelope.TargetClientId) != 16 {
				s.mu.RUnlock()
				return errors.New("TO_CLIENT requires target client id")
			}
			target := r.clients[string(envelope.TargetClientId)]
			s.mu.RUnlock()
			if target == nil {
				return errors.New("target client is not in room")
			}
			if !target.enqueue(envelope) {
				return errors.New("target write queue unavailable")
			}
			if envelope.MessageType == MessageWelcome {
				target.markGameReady()
			}
			return nil
		}
		if envelope.Delivery == wire.Delivery_BROADCAST {
			if len(envelope.TargetClientId) != 0 {
				s.mu.RUnlock()
				return errors.New("broadcast contains target client id")
			}
			targets := make([]*connection, 0, len(r.clients))
			for _, target := range r.clients {
				if target.isGameReady() {
					targets = append(targets, target)
				}
			}
			s.mu.RUnlock()
			for _, target := range targets {
				target.enqueue(envelope)
			}
			return nil
		}
		s.mu.RUnlock()
		return errors.New("host may only send TO_CLIENT or BROADCAST")
	default:
		s.mu.RUnlock()
		return errors.New("invalid connection role")
	}
}

func (s *Server) boundHost(c *connection, envelope *wire.WireEnvelope, requestedRoom []byte) (*room, error) {
	role, id, roomID := c.binding()
	if role != roleHost || !bytes.Equal(requestedRoom, roomID) || !bytes.Equal(envelope.RoomId, roomID) || !bytes.Equal(envelope.ClientId, id) || len(envelope.TargetClientId) != 0 {
		return nil, errors.New("host control request does not match binding")
	}
	s.mu.RLock()
	r := s.rooms[string(roomID)]
	s.mu.RUnlock()
	if r == nil || r.host != c {
		return nil, errors.New("host room is missing")
	}
	return r, nil
}

func validBoundEnvelope(envelope *wire.WireEnvelope, roomID []byte, c *connection) bool {
	_, id, _ := c.binding()
	return bytes.Equal(envelope.RoomId, roomID) && bytes.Equal(envelope.ClientId, id) && len(envelope.TargetClientId) == 0
}

func (s *Server) onDisconnect(c *connection) {
	role, id, roomID := c.binding()
	s.mu.Lock()
	delete(s.conns, c)
	r := s.rooms[string(roomID)]
	if r == nil {
		s.mu.Unlock()
		return
	}
	if role == roleHost && r.host == c {
		delete(s.rooms, string(roomID))
		clients := make([]*connection, 0, len(r.clients))
		for _, client := range r.clients {
			clients = append(clients, client)
		}
		s.mu.Unlock()
		for _, client := range clients {
			s.notifyRoomClosed(client, "host disconnected", roomID)
		}
		return
	}
	if role == roleClient && r.clients[string(id)] == c {
		delete(r.clients, string(id))
		r.info.ConnectedPlayers = int32(1 + len(r.clients))
		host := r.host
		s.mu.Unlock()
		notice := &wire.RelayPeerNotice{ClientId: cloneBytes(id), Reason: "disconnected"}
		_ = s.sendControl(host, MessageRelayPeerLeft, notice, roomID)
		return
	}
	s.mu.Unlock()
}

func (s *Server) notifyRoomClosed(client *connection, reason string, roomID []byte) {
	notice := &wire.RelayPeerNotice{Reason: reason}
	envelope, err := makeControlEnvelope(MessageRelayCloseRoom, notice, roomID)
	if err != nil {
		client.close()
		return
	}
	client.sendAndClose(envelope)
}

func (s *Server) controlSuccess(c *connection, requestID uint64, clientID, roomID []byte) error {
	return s.controlResponse(c, requestID, true, "", clientID, roomID)
}

func (s *Server) controlFailure(c *connection, requestID uint64, reason string, roomID []byte) error {
	return s.controlResponse(c, requestID, false, reason, nil, roomID)
}

func (s *Server) controlResponse(c *connection, requestID uint64, success bool, reason string, clientID, roomID []byte) error {
	envelope, err := makeControlResponse(requestID, success, reason, clientID, roomID)
	if err != nil {
		return err
	}
	if !c.enqueue(envelope) {
		return errors.New("control response write queue unavailable")
	}
	return nil
}

func makeControlResponse(requestID uint64, success bool, reason string, clientID, roomID []byte) (*wire.WireEnvelope, error) {
	response := &wire.RelayControlResponse{RequestId: requestID, Success: success, Reason: reason, ClientId: cloneBytes(clientID)}
	return makeControlEnvelope(MessageRelayControlResponse, response, roomID)
}

func (s *Server) sendControl(c *connection, messageType uint32, payload proto.Message, roomID []byte) error {
	envelope, err := makeControlEnvelope(messageType, payload, roomID)
	if err != nil {
		return err
	}
	if !c.enqueue(envelope) {
		return errors.New("control write queue unavailable")
	}
	return nil
}

func randomID() ([]byte, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	return id, nil
}

func validateID(id []byte, name string) error {
	if len(id) != 16 {
		return fmt.Errorf("%s must contain 16 bytes", name)
	}
	return nil
}
