package logger

import (
	"bytes"
	"errors"
	"io"
	"testing"

	providerv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/provider/v1"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/reflect/protoreflect"
)

//nolint:gocognit // test
func TestMsgLogger_Log(t *testing.T) {
	type args struct {
		msg []protoreflect.Message
	}
	registerMsg := providerv1.RegisterEventRequest{
		RecordingMode: providerv1.RecordingMode_RECORDING_MODE_PERSIST,
	}
	unregisterMsg := providerv1.UnregisterEventRequest{}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
		{"empty", args{[]protoreflect.Message{
			registerMsg.ProtoReflect(),
			unregisterMsg.ProtoReflect(),
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
