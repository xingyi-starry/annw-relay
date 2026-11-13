package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"mprelay/internal/wire"
)

const (
	MessageRelayRoomClosed      uint32 = 51
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
	if envelopeBody(envelope) == nil {
		return nil, errors.New("missing message body")
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

func unmarshalPayload(value proto.Message, message proto.Message) error {
	if value == nil || value.ProtoReflect().Descriptor().FullName() != message.ProtoReflect().Descriptor().FullName() {
		return errors.New("unexpected control message")
	}
	proto.Merge(message, value)
	return nil
}

func makeControlEnvelope(messageType uint32, payload proto.Message, roomID []byte) (*wire.WireEnvelope, error) {
	envelope := &wire.WireEnvelope{RoomId: cloneBytes(roomID), Delivery: wire.Delivery_RELAY_CONTROL}
	if err := setEnvelopeBody(envelope, messageType, payload); err != nil {
		return nil, err
	}
	return envelope, nil
}

// The generated oneof descriptor is the source of branch identity. The relay
// does not inspect game-message fields when classifying or forwarding them.
func bodyField(envelope *wire.WireEnvelope) protoreflect.FieldDescriptor {
	if envelope == nil {
		return nil
	}
	message := envelope.ProtoReflect()
	return message.WhichOneof(message.Descriptor().Oneofs().ByName("body"))
}

func envelopeType(envelope *wire.WireEnvelope) uint32 {
	field := bodyField(envelope)
	if field == nil {
		return 0
	}
	return uint32(field.Number()) - 100
}

func envelopeBody(envelope *wire.WireEnvelope) proto.Message {
	field := bodyField(envelope)
	if field == nil {
		return nil
	}
	message := envelope.ProtoReflect().Get(field).Message()
	if !message.IsValid() {
		return nil
	}
	return message.Interface()
}

func setEnvelopeBody(envelope *wire.WireEnvelope, messageType uint32, payload proto.Message) error {
	message := envelope.ProtoReflect()
	field := message.Descriptor().Fields().ByNumber(protoreflect.FieldNumber(100 + messageType))
	if field == nil || field.ContainingOneof() == nil || field.ContainingOneof().Name() != "body" || payload == nil {
		return errors.New("unknown envelope body")
	}
	value := payload.ProtoReflect()
	if !value.IsValid() || field.Message().FullName() != value.Descriptor().FullName() {
		return errors.New("unexpected envelope body")
	}
	message.Set(field, protoreflect.ValueOfMessage(value))
	return nil
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
