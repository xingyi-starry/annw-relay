package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
	"mprelay/internal/wire"
)

const (
	MessageRelayHeartbeat       uint32 = 4
	MessageRelayRegisterRoom    uint32 = 23
	MessageRelayUpdateRoom      uint32 = 24
	MessageRelayListRooms       uint32 = 25
	MessageRelayRoomList        uint32 = 26
	MessageRelayJoinRoom        uint32 = 27
	MessageRelayControlResponse uint32 = 28
	MessageRelayCloseJoining    uint32 = 29
	MessageRelayLeaveRoom       uint32 = 30
	MessageRelayCloseRoom       uint32 = 31
	MessageRelayPeerJoined      uint32 = 32
	MessageRelayPeerLeft        uint32 = 33
	MessageRelayResumeRoom      uint32 = 41
)

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
		MessageType: messageType,
		Payload:     data,
		RoomId:      cloneBytes(roomID),
		Delivery:    wire.Delivery_RELAY_CONTROL,
	}, nil
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
