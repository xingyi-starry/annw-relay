package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/sirupsen/logrus"

	"google.golang.org/protobuf/proto"
	"mprelay/internal/wire"
)

type room struct {
	id           []byte
	hostID       []byte
	host         *connection
	clients      map[string]*connection
	reservations map[string]time.Time
	matchID      []byte
	password     passwordVerifier
	info         wire.RelayRoomInfo
}

type Server struct {
	config     Config
	logger     *logrus.Logger
	nextConnID atomic.Uint64
	ctx        context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
	closedDone chan struct{}
	background sync.WaitGroup
	mu         sync.RWMutex
	rooms      map[string]*room
	conns      map[*connection]struct{}
	listener   net.Listener
	closed     bool
}

const (
	maxRoomNameRunes    = 48
	maxHostNameRunes    = 32
	maxMapTitleRunes    = 128
	maxVersionBytes     = 80
	maxFingerprintBytes = 256
	maxPasswordBytes    = 256
)

func New(config Config, logger *logrus.Logger) *Server {
	config.normalize()
	if logger == nil {
		logger = NewLogger(io.Discard)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{config: config, logger: logger, ctx: ctx, cancel: cancel, closedDone: make(chan struct{}), rooms: make(map[string]*room), conns: make(map[*connection]struct{})}
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
		peer.start()
		s.mu.Unlock()
		s.logFor(peer).WithField("event", "connection_opened").Info("connection accepted")
	}
}

func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		listener := s.listener
		peers := make([]*connection, 0, len(s.conns))
		for peer := range s.conns {
			peers = append(peers, peer)
		}
		s.rooms = make(map[string]*room)
		s.mu.Unlock()
		s.cancel()
		if listener != nil {
			_ = listener.Close()
		}
		for _, peer := range peers {
			peer.abort(disconnectServerShutdown)
		}
		for _, peer := range peers {
			peer.wait()
		}
		s.background.Wait()
		close(s.closedDone)
	})
	<-s.closedDone
	return nil
}

func (s *Server) handleEnvelope(c *connection, envelope *wire.WireEnvelope) error {
	if envelopeType(envelope) == MessageRelayHeartbeat && envelope.Delivery == wire.Delivery_RELAY_CONTROL {
		return nil
	}
	if envelope.Delivery == wire.Delivery_RELAY_CONTROL {
		return s.handleControl(c, envelope)
	}
	return s.routeGameMessage(c, envelope)
}

func (s *Server) handleControl(c *connection, envelope *wire.WireEnvelope) error {
	switch envelopeType(envelope) {
	case MessageRelayRegisterRoom:
		return s.registerRoom(c, envelope)
	case MessageRelayUpdateRoom:
		return s.updateRoom(c, envelope)
	case MessageRelayListRooms:
		return s.listRooms(c, envelope)
	case MessageRelayJoinRoom:
		return s.joinRoom(c, envelope)
	case MessageRelayResumeRoom:
		return s.resumeRoom(c, envelope)
	case MessageRelayCloseJoining:
		return s.closeJoining(c, envelope)
	case MessageRelayLeaveRoom:
		return s.leaveRoom(c, envelope)
	case MessageRelayCloseRoom:
		return s.closeRoomRequest(c, envelope)
	default:
		return fmt.Errorf("control message type %d is not accepted", envelopeType(envelope))
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
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
		return err
	}
	if err := validateID(req.RoomId, "room id"); err != nil {
		return s.controlFailure(c, req.RequestId, err.Error(), nil)
	}
	roomName := strings.TrimSpace(req.RoomName)
	hostName := strings.TrimSpace(req.HostName)
	if roomName == "" || utf8.RuneCountInString(roomName) > maxRoomNameRunes || hostName == "" || utf8.RuneCountInString(hostName) > maxHostNameRunes || len(req.Password) > maxPasswordBytes || len(req.PluginVersion) > maxVersionBytes || len(req.GameFingerprint) > maxFingerprintBytes || len(req.ContentFingerprint) > maxFingerprintBytes || req.MaxParticipants < 0 || req.MaxParticipants > 8 {
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
	r := &room{id: cloneBytes(req.RoomId), hostID: hostID, host: c, clients: make(map[string]*connection), reservations: make(map[string]time.Time), password: verifier}
	r.info = wire.RelayRoomInfo{RoomId: cloneBytes(req.RoomId), RoomName: roomName, HostName: hostName, ConnectedPlayers: 1, HasPassword: verifier.enabled(), Status: wire.RelayRoomStatus_RELAY_ROOM_WAITING, PluginVersion: req.PluginVersion, GameFingerprint: req.GameFingerprint, ContentFingerprint: req.ContentFingerprint, MaxParticipants: req.MaxParticipants}
	s.rooms[key] = r
	c.bind(roleHost, hostID, req.RoomId)
	s.mu.Unlock()
	s.logFor(c).WithFields(logrus.Fields{"event": "room_created", "request_id": req.RequestId}).Info("room registered")
	return s.controlSuccess(c, req.RequestId, hostID, req.RoomId)
}

func (s *Server) updateRoom(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayUpdateRoomRequest)
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
		return err
	}
	r, err := s.boundHost(c, envelope, req.RoomId)
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(req.MapTitle) > maxMapTitleRunes || req.HumanSeats < 0 || req.AvailableSeats < 0 || req.MaxParticipants < 0 || req.MaxParticipants > 8 || len(req.MatchId) != 0 && len(req.MatchId) != 16 {
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
	r.info.AvailableSeats = req.AvailableSeats
	r.info.MaxParticipants = req.MaxParticipants
	r.info.Status = req.Status
	if req.Status == wire.RelayRoomStatus_RELAY_ROOM_WAITING {
		r.matchID = nil
		clear(r.reservations)
	} else if len(req.MatchId) != 0 {
		r.matchID = cloneBytes(req.MatchId)
	}
	r.info.ConnectedPlayers = int32(1 + len(r.clients))
	s.mu.Unlock()
	s.logFor(c).WithFields(logrus.Fields{"event": "room_updated", "request_id": req.RequestId, "match_id": formatID(req.MatchId), "status": req.Status.String(), "available_seats": req.AvailableSeats}).Info("room metadata updated")
	return s.controlSuccess(c, req.RequestId, nil, r.id)
}

func roomStatusTransitionAllowed(current, next wire.RelayRoomStatus) bool {
	switch current {
	case wire.RelayRoomStatus_RELAY_ROOM_WAITING:
		return next == wire.RelayRoomStatus_RELAY_ROOM_WAITING || next == wire.RelayRoomStatus_RELAY_ROOM_PLAYING
	case wire.RelayRoomStatus_RELAY_ROOM_CLOSED:
		return next == wire.RelayRoomStatus_RELAY_ROOM_CLOSED || next == wire.RelayRoomStatus_RELAY_ROOM_PLAYING
	case wire.RelayRoomStatus_RELAY_ROOM_PLAYING:
		return next == wire.RelayRoomStatus_RELAY_ROOM_PLAYING || next == wire.RelayRoomStatus_RELAY_ROOM_WAITING
	default:
		return false
	}
}

func (s *Server) listRooms(c *connection, envelope *wire.WireEnvelope) error {
	if len(envelope.RoomId) != 0 || len(envelope.ClientId) != 0 || len(envelope.TargetClientId) != 0 {
		return errors.New("list envelope contains routing identity")
	}
	req := new(wire.RelayListRoomsRequest)
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
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
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
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
	if status != wire.RelayRoomStatus_RELAY_ROOM_WAITING && status != wire.RelayRoomStatus_RELAY_ROOM_PLAYING {
		return s.controlFailure(c, req.RequestId, "room is not accepting players", roomID)
	}
	if !verifier.matches(req.Password) {
		return s.controlFailure(c, req.RequestId, "invalid password", roomID)
	}
	s.mu.Lock()
	r = s.rooms[string(req.RoomId)]
	if r == nil || (r.info.Status != wire.RelayRoomStatus_RELAY_ROOM_WAITING && r.info.Status != wire.RelayRoomStatus_RELAY_ROOM_PLAYING) {
		s.mu.Unlock()
		return s.controlFailure(c, req.RequestId, "room is not accepting players", roomID)
	}
	r.clients[string(clientID)] = c
	r.info.ConnectedPlayers = int32(1 + len(r.clients))
	c.bind(roleClient, clientID, r.id)
	host := r.host
	roomID = cloneBytes(r.id)
	s.mu.Unlock()
	s.logFor(c).WithFields(logrus.Fields{"event": "client_joined", "request_id": req.RequestId}).Info("client joined room")
	if err := s.controlSuccess(c, req.RequestId, clientID, roomID); err != nil {
		return err
	}
	notice := &wire.RelayPeerNotice{ClientId: cloneBytes(clientID), Reason: "joined"}
	return s.sendControl(host, MessageRelayPeerJoined, notice, roomID)
}

func (s *Server) resumeRoom(c *connection, envelope *wire.WireEnvelope) error {
	if role, _, _ := c.binding(); role != roleUnbound {
		return errors.New("connection is already bound")
	}
	if len(envelope.RoomId) != 0 || len(envelope.ClientId) != 0 || len(envelope.TargetClientId) != 0 {
		return errors.New("unbound resume envelope contains identity")
	}
	req := new(wire.RelayResumeRoomRequest)
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
		return err
	}
	if err := validateID(req.RoomId, "room id"); err != nil {
		return s.controlFailure(c, req.RequestId, err.Error(), nil)
	}
	if err := validateID(req.ClientId, "client id"); err != nil {
		return s.controlFailure(c, req.RequestId, err.Error(), req.RoomId)
	}
	if err := validateID(req.MatchId, "match id"); err != nil {
		return s.controlFailure(c, req.RequestId, err.Error(), req.RoomId)
	}

	now := time.Now()
	s.mu.Lock()
	r := s.rooms[string(req.RoomId)]
	if r == nil || r.info.Status != wire.RelayRoomStatus_RELAY_ROOM_PLAYING || !bytes.Equal(r.matchID, req.MatchId) {
		s.mu.Unlock()
		return s.controlFailure(c, req.RequestId, "room or match is not resumable", req.RoomId)
	}
	key := string(req.ClientId)
	deadline, reserved := r.reservations[key]
	if !reserved || !now.Before(deadline) || r.clients[key] != nil {
		s.mu.Unlock()
		return s.controlFailure(c, req.RequestId, "reconnect reservation is unavailable", req.RoomId)
	}
	delete(r.reservations, key)
	r.clients[key] = c
	r.info.ConnectedPlayers = int32(1 + len(r.clients))
	c.bind(roleClient, req.ClientId, r.id)
	host := r.host
	roomID := cloneBytes(r.id)
	s.mu.Unlock()
	s.logFor(c).WithFields(logrus.Fields{"event": "client_resumed", "request_id": req.RequestId, "match_id": formatID(req.MatchId)}).Info("client resumed room")
	if err := s.controlSuccess(c, req.RequestId, req.ClientId, roomID); err != nil {
		return err
	}
	return s.sendControl(host, MessageRelayPeerJoined, &wire.RelayPeerNotice{ClientId: cloneBytes(req.ClientId), Reason: "resumed"}, roomID)
}

func (s *Server) closeJoining(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayRoomRequest)
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
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
	s.logFor(c).WithFields(logrus.Fields{"event": "joining_closed", "request_id": req.RequestId}).Info("room joining closed")
	return s.controlSuccess(c, req.RequestId, nil, r.id)
}

func (s *Server) leaveRoom(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayRoomRequest)
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
		return err
	}
	role, _, roomID := c.binding()
	if role == roleUnbound || !bytes.Equal(roomID, req.RoomId) || !validBoundEnvelope(envelope, roomID, c) {
		return errors.New("leave request does not match binding")
	}
	if role == roleHost {
		return s.closeRoom(c, req.RequestId, "host closed room")
	}
	s.mu.Lock()
	r := s.rooms[string(roomID)]
	_, id, _ := c.binding()
	if r != nil && r.clients[string(id)] == c {
		delete(r.clients, string(id))
		delete(r.reservations, string(id))
		r.info.ConnectedPlayers = int32(1 + len(r.clients))
	}
	host := (*connection)(nil)
	if r != nil {
		host = r.host
	}
	c.unbind()
	s.mu.Unlock()
	s.logger.WithFields(logrus.Fields{"event": "client_left", "room_id": formatID(roomID), "client_id": formatID(id), "request_id": req.RequestId, "conn_id": c.connID}).Info("client left room")
	if host != nil {
		_ = s.sendControl(host, MessageRelayPeerLeft, &wire.RelayPeerNotice{ClientId: cloneBytes(id), Reason: "left"}, roomID)
	}
	response, err := makeControlResponse(req.RequestId, true, "", nil, roomID)
	if err != nil {
		return err
	}
	c.drainAndClose(response, disconnectExplicitLeave)
	return nil
}

func (s *Server) closeRoomRequest(c *connection, envelope *wire.WireEnvelope) error {
	req := new(wire.RelayRoomRequest)
	if err := unmarshalPayload(envelopeBody(envelope), req); err != nil {
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
	s.logger.WithFields(logrus.Fields{"event": "room_closed", "room_id": formatID(roomID), "client_id": formatID(r.hostID), "match_id": formatID(r.matchID), "request_id": requestID, "reason": reason}).Info("room closed")
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
		host := r.host
		s.mu.RUnlock()
		host.enqueue(envelope)
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
			target.enqueue(envelope)
			return nil
		}
		if envelope.Delivery == wire.Delivery_BROADCAST {
			if len(envelope.TargetClientId) != 0 {
				s.mu.RUnlock()
				return errors.New("broadcast contains target client id")
			}
			targets := make([]*connection, 0, len(r.clients))
			for _, target := range r.clients {
				targets = append(targets, target)
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

func (s *Server) onConnectionClosed(c *connection, cause disconnectCause) {
	role, id, roomID := c.binding()
	s.mu.Lock()
	delete(s.conns, c)
	if s.closed || cause == disconnectServerShutdown || cause == disconnectExplicitLeave || cause == disconnectRoomClosed {
		s.mu.Unlock()
		return
	}
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
		s.logger.WithFields(logrus.Fields{"event": "host_disconnected", "room_id": formatID(roomID), "client_id": formatID(id), "match_id": formatID(r.matchID), "conn_id": c.connID}).Info("host disconnected; room closed")
		for _, client := range clients {
			s.notifyRoomClosed(client, "host disconnected", roomID)
		}
		return
	}
	if role == roleClient && r.clients[string(id)] == c {
		delete(r.clients, string(id))
		r.info.ConnectedPlayers = int32(1 + len(r.clients))
		if reconnectableDisconnect(cause) && r.info.Status == wire.RelayRoomStatus_RELAY_ROOM_PLAYING && len(r.matchID) == 16 {
			deadline := time.Now().Add(s.config.ReconnectGrace)
			r.reservations[string(id)] = deadline
			host := r.host
			s.mu.Unlock()
			s.logger.WithFields(logrus.Fields{"event": "reconnect_reserved", "room_id": formatID(roomID), "client_id": formatID(id), "match_id": formatID(r.matchID), "conn_id": c.connID, "grace_seconds": s.config.ReconnectGrace.Seconds()}).Info("client connection reserved")
			_ = s.sendControl(host, MessageRelayPeerLeft, &wire.RelayPeerNotice{ClientId: cloneBytes(id), Reason: "reconnecting"}, roomID)
			s.scheduleReservationExpiry(roomID, id, deadline)
			return
		}
		host := r.host
		s.mu.Unlock()
		s.logger.WithFields(logrus.Fields{"event": "client_disconnected", "room_id": formatID(roomID), "client_id": formatID(id), "conn_id": c.connID}).Info("client disconnected")
		_ = s.sendControl(host, MessageRelayPeerLeft, &wire.RelayPeerNotice{ClientId: cloneBytes(id), Reason: "disconnected"}, roomID)
		return
	}
	s.mu.Unlock()
}

func reconnectableDisconnect(cause disconnectCause) bool {
	return cause == disconnectRemote || cause == disconnectTimeout || cause == disconnectWriteFailure
}

func (s *Server) scheduleReservationExpiry(roomID, clientID []byte, deadline time.Time) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.background.Add(1)
	s.mu.Unlock()
	timer := time.NewTimer(time.Until(deadline))
	go func() {
		defer s.background.Done()
		defer timer.Stop()
		select {
		case <-timer.C:
			s.expireReservation(roomID, clientID, deadline)
		case <-s.ctx.Done():
		}
	}()
}

func (s *Server) expireReservation(roomID, clientID []byte, deadline time.Time) {
	s.mu.Lock()
	r := s.rooms[string(roomID)]
	if s.closed || r == nil || !r.reservations[string(clientID)].Equal(deadline) {
		s.mu.Unlock()
		return
	}
	delete(r.reservations, string(clientID))
	host := r.host
	s.mu.Unlock()
	s.logger.WithFields(logrus.Fields{"event": "reconnect_expired", "room_id": formatID(roomID), "client_id": formatID(clientID), "match_id": formatID(r.matchID)}).Info("client reconnect reservation expired")
	_ = s.sendControl(host, MessageRelayPeerLeft, &wire.RelayPeerNotice{ClientId: cloneBytes(clientID), Reason: "reconnect timeout"}, roomID)
}

func (s *Server) notifyRoomClosed(client *connection, reason string, roomID []byte) {
	notice := &wire.RelayPeerNotice{Reason: reason}
	envelope, err := makeControlEnvelope(MessageRelayRoomClosed, notice, roomID)
	if err != nil {
		client.abort(disconnectRoomClosed)
		return
	}
	client.drainAndClose(envelope, disconnectRoomClosed)
}

func (s *Server) controlSuccess(c *connection, requestID uint64, clientID, roomID []byte) error {
	return s.controlResponse(c, requestID, true, "", clientID, roomID)
}

func (s *Server) controlFailure(c *connection, requestID uint64, reason string, roomID []byte) error {
	fields := logrus.Fields{"event": "control_rejected", "request_id": requestID, "reason": reason}
	if value := formatID(roomID); value != "" {
		fields["room_id"] = value
	}
	s.logFor(c).WithFields(fields).Warn("control request rejected")
	return s.controlResponse(c, requestID, false, reason, nil, roomID)
}

func (s *Server) controlResponse(c *connection, requestID uint64, success bool, reason string, clientID, roomID []byte) error {
	envelope, err := makeControlResponse(requestID, success, reason, clientID, roomID)
	if err != nil {
		return err
	}
	c.enqueue(envelope)
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
	c.enqueue(envelope)
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
