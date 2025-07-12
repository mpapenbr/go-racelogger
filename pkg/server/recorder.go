package server

import (
	"context"

	v1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/racelogger/v1"
	"google.golang.org/grpc"

	"github.com/mpapenbr/go-racelogger/internal/recorder"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
)

type (
	// used when recording is in progress
	recordingContext struct {
		ctx             context.Context
		cancel          context.CancelFunc
		conn            *grpc.ClientConn // connection to the provider service
		recorder        *recorder.Recorder
		l               *log.Logger
		cbRecordingDone func()
	}
)

//nolint:whitespace // editor/linter issue
func newRecordingContext(
	ctx context.Context,
	conn *grpc.ClientConn,
	cbRecordingDone func(),
) *recordingContext {
	myCtx, cancel := context.WithCancel(ctx)
	return &recordingContext{
		ctx:             myCtx,
		cancel:          cancel,
		conn:            conn,
		cbRecordingDone: cbRecordingDone,
		l:               log.GetFromContext(myCtx).Named("recsrv"),
	}
}

func (rc *recordingContext) startRecording(msg *v1.StartRecordingRequest) {
	rc.l.Debug("Start recording")
	rc.recorder = recorder.NewRecorder(
		recorder.WithContext(rc.ctx, rc.cancel),
		recorder.WithCliArgs(config.DefaultCliArgs()),
		recorder.WithConnection(rc.conn),
		recorder.WithEventNames([]string{msg.Name}),
		recorder.WithEventDescriptions(msg.Descriptions),
	)
	rc.recorder.Start()
	go func() {
		// Wait for the context to be done, which means the recording is finished
		<-rc.ctx.Done()
		rc.l.Debug("Received ctx.Done.")
		rc.cbRecordingDone()
		rc.l.Debug("Race recording finished")
	}()
}

func (rc *recordingContext) stopRecording() {
	rc.l.Debug("Stop recording")
	rc.recorder.Stop()
	rc.cancel()
}
