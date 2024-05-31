//nolint:gocognit,funlen // test
package logger

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	commonv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/common/v1"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var eventSel = &commonv1.EventSelector{
	Arg: &commonv1.EventSelector_Key{Key: "myKey"},
}

var pubState = racestatev1.PublishStateRequest{
	Event:     eventSel,
	Timestamp: timestamppb.New(time.Now()),
	Cars:      []*racestatev1.Car{},
	Session: &racestatev1.Session{
		SessionNum: 1,
	},
	Messages: []*racestatev1.Message{},
}

func TestMsgLogger_Log(t *testing.T) {
	type args struct {
		msg []protoreflect.Message
	}
	registerMsg := providerv1.RegisterEventRequest{
		RecordingMode: providerv1.RecordingMode_RECORDING_MODE_PERSIST,
	}
	unregisterMsg := providerv1.UnregisterEventRequest{
		EventSelector: eventSel,
	}
	tests := []struct {
		name string
		args args
	}{
		{"reg_unreg", args{[]protoreflect.Message{
			registerMsg.ProtoReflect(),
			unregisterMsg.ProtoReflect(),
		}}},
		{"state", args{[]protoreflect.Message{
			pubState.ProtoReflect(),
			pubState.ProtoReflect(),
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new bytes.Buffer with a preallocated 100-byte slice
			buf := bytes.NewBuffer(make([]byte, 0, 100))
			writeLogger := &MsgLogger{w: buf}
			// write all messages to the buffer
			for _, m := range tt.args.msg {
				if err := writeLogger.Log(m); err != nil {
					t.Error(err)
				}
			}
			// read all messages from the buffer
			readLogger := &MsgLogger{r: bytes.NewBuffer(buf.Bytes())}
			data := make([]protoreflect.Message, 0)
			for {
				msg, err := readLogger.ReadNext()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Error(err)
				}
				data = append(data, msg)
			}
			// assert that length of messages written is equal to length of messages read
			assert.Equal(t, len(tt.args.msg), len(data))
		})
	}
}
