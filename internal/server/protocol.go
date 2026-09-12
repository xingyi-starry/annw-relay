package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
	"mprelay/internal/wire"
)

const ProtocolVersion uint32 = 11

const (
	MessageHello uint32 = iota + 1
	MessageWelcome
	MessageReject
	MessageHeartbeat
	MessageRoomState
	MessageClaimSeat
	MessageSetReady
	MessageStartMatch
	MessageCommandRequest
	MessageCommandAccepted
	MessageCommandRejected
	MessageAuthorityFrame
	MessageHistoryRequest
	MessageHistoryComplete
	MessageSnapshotRequest
	MessageSnapshotManifest
	MessageSnapshotChunk
	MessageSnapshotComplete
	MessageCatchUpComplete
	MessageSessionEnded
	MessageLobbyDraftChange
	MessageParticipantNotice
	MessageRelayRegisterRoom
	MessageRelayUpdateRoom
	MessageRelayListRooms
	MessageRelayRoomList
	MessageRelayJoinRoom
	MessageRelayControlResponse
	MessageRelayCloseJoining
	MessageRelayLeaveRoom
	MessageRelayCloseRoom
	MessageRelayPeerJoined
	MessageRelayPeerLeft
)

func isRelayMessage(messageType uint32) bool {
	return messageType >= MessageRelayRegisterRoom && messageType <= MessageRelayPeerLeft
}

func isGameMessage(messageType uint32) bool {
	return messageType >= MessageHello && messageType <= MessageParticipantNotice
}

func readEnvelope(r io.Reader, maxBytes int) (*wire.WireEnvelope, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(header[:])
	if length == 0 || uint64(length) > uint64(maxBytes) {
		return nil, fmt.Errorf("invalid packet length %d", length)
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	envelope := new(wire.WireEnvelope)
	if err := proto.Unmarshal(data, envelope); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	return envelope, nil
}

func marshalEnvelope(envelope *wire.WireEnvelope, maxBytes int) ([]byte, error) {
	data, err := proto.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maxBytes {
		return nil, errors.New("encoded packet exceeds limit")
	}
	framed := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint32(framed[:4], uint32(len(data)))
	copy(framed[4:], data)
	return framed, nil
}

func unmarshalPayload(data []byte, message proto.Message) error {
	if err := proto.Unmarshal(data, message); err != nil {
		return fmt.Errorf("decode control payload: %w", err)
	}
	return nil
}

func makeControlEnvelope(messageType uint32, payload proto.Message, roomID []byte) (*wire.WireEnvelope, error) {
	data, err := proto.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &wire.WireEnvelope{
		ProtocolVersion: ProtocolVersion,
		MessageType:     messageType,
		Payload:         data,
		RoomId:          cloneBytes(roomID),
		Delivery:        wire.Delivery_RELAY_CONTROL,
	}, nil
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
