package logger

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/mpapenbr/go-racelogger/log"
)

type (
	MsgLogger struct {
		w     io.Writer
		r     io.Reader
		m     sync.Mutex
		count int
		debug bool
	}
	Option func(*MsgLogger)
	header struct {
		MsgType byte
		MsgLen  uint16
	}
)

const (
	MsgUnknown byte = iota
	MsgRegister
	MsgUnregister
	MsgState
	MsgDriverData
	MsgSpeedmap
	MsgEventExtraInfo
)

var ErrNoReader = errors.New("no reader")

func NewMsgLogger(opts ...Option) *MsgLogger {
	ret := &MsgLogger{}
	for _, opt := range opts {
		opt(ret)
	}
	ret.m = sync.Mutex{}
	return ret
}

func WithWriter(w io.Writer) Option {
	return func(ml *MsgLogger) {
		ml.w = w
	}
}

func WithReader(r io.Reader) Option {
	return func(ml *MsgLogger) {
		ml.r = r
	}
}

func WithDebug(b bool) Option {
	return func(ml *MsgLogger) {
		ml.debug = b
	}
}

//nolint:govet // false positive
func (m *MsgLogger) Log(msg protoreflect.Message) error {
	if m.w == nil {
		return nil
	}

	t := m.getMsgType(msg.Interface())
	if t == MsgUnknown {
		return nil
	}

	if b, err := proto.Marshal(msg.Interface()); err == nil {
		m.m.Lock()
		defer m.m.Unlock()
		h := header{MsgType: t, MsgLen: uint16(len(b))}

		if err := m.write(m.w, h, b); err != nil {
			return err
		}
		m.count++
	} else {
		return err
	}

	return nil
}

func (m *MsgLogger) write(w io.Writer, h header, b []byte) error {
	if err := binary.Write(w, binary.LittleEndian, h); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return nil
}

//nolint:cyclop // false positive
func (m *MsgLogger) ReadNext() (protoreflect.Message, error) {
	if m.r == nil {
		return nil, ErrNoReader
	}

	h := header{}
	if err := binary.Read(m.r, binary.LittleEndian, &h); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		log.Error("could not read header", log.ErrorField(err))
		return nil, err
	}

	b := make([]byte, h.MsgLen)
	if _, err := io.ReadFull(m.r, b); err != nil {
		return nil, err
	}

	var msg proto.Message
	switch h.MsgType {
	case MsgRegister:
		msg = &providerv1.RegisterEventRequest{}
	case MsgUnregister:
		msg = &providerv1.UnregisterEventRequest{}
	case MsgState:
		msg = &racestatev1.PublishStateRequest{}
	case MsgDriverData:
		msg = &racestatev1.PublishDriverDataRequest{}
	case MsgSpeedmap:
		msg = &racestatev1.PublishSpeedmapRequest{}
	case MsgEventExtraInfo:
		msg = &racestatev1.PublishEventExtraInfoRequest{}
	default:
		return nil, nil
	}

	if err := proto.Unmarshal(b, msg); err != nil {
		return nil, err
	}

	return msg.ProtoReflect(), nil
}

func (m *MsgLogger) getMsgType(msg interface{}) byte {
	switch c := msg.(type) {
	case *providerv1.RegisterEventRequest:
		return MsgRegister
	case *providerv1.UnregisterEventRequest:
		return MsgUnregister
	case *racestatev1.PublishStateRequest:
		return MsgState
	case *racestatev1.PublishDriverDataRequest:
		return MsgDriverData
	case *racestatev1.PublishSpeedmapRequest:
		return MsgSpeedmap
	case *racestatev1.PublishEventExtraInfoRequest:
		return MsgEventExtraInfo
	default:
		fmt.Printf("%T\n", c)
	}
	return MsgUnknown
}
