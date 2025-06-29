package server

import (
	"context"
	"errors"
	"time"

	pb "buf.build/gen/go/mpapenbr/iracelog/connectrpc/go/racelogger/v1/raceloggerv1connect"
	v1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/racelogger/v1"
	"connectrpc.com/connect"

	"github.com/mpapenbr/go-racelogger/log"
)

type (
	raceloggerServiceConnectRPC struct {
		pb.UnimplementedRaceloggerServiceHandler
		serverImpl *serverImpl
	}
)

//nolint:lll // ok here
func NewRaceloggerServiceConnectRPC(serverImpl *serverImpl) *raceloggerServiceConnectRPC {
	return &raceloggerServiceConnectRPC{
		serverImpl: serverImpl,
	}
}

//nolint:gocognit,whitespace,funlen // ok here
func (s *raceloggerServiceConnectRPC) GetStatusStream(
	ctx context.Context,
	req *connect.Request[v1.GetStatusStreamRequest],
	stream *connect.ServerStream[v1.GetStatusStreamResponse],
) error {
	log.Debug("GetStatusStream (connectRPC) called")
	status := s.serverImpl.GetStatus()
	if status == nil {
		log.Error("GetStatusStream (connectRPC) status is nil")
		return connect.NewError(connect.CodeInternal, errors.New("status is nil"))
	}
	composeRaceSessions := func() []*v1.RaceSession {
		raceSessions := make([]*v1.RaceSession, 0, len(status.RaceSessions))
		for _, rs := range status.RaceSessions {
			raceSessions = append(raceSessions, &v1.RaceSession{
				Num:  rs.Num,
				Name: rs.Name,
			})
		}
		return raceSessions
	}
	composeResponse := func() *v1.GetStatusStreamResponse {
		return &v1.GetStatusStreamResponse{
			BackendAvailable:   status.BackendAvailable,
			BackendCompatible:  status.BackendCompatible,
			SimulationRunning:  status.SimulationRunning,
			TelemetryAvailable: status.TelemetryAvailable,
			RecordingActive:    status.Recording,
			CurrentSessionNum:  status.CurrentSessionNum,
			RaceSessions:       composeRaceSessions(),
		}
	}
	if err := stream.Send(composeResponse()); err != nil {
		log.Error("GetStatusStream (connectRPC) error sending initial status",
			log.ErrorField(err))
		return err
	}
	ticker := time.NewTicker(2 * time.Second)
	lastStatus := *status
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Debug("GetStatusStream (connectRPC) context done")
			return nil
		case <-ticker.C:
			if status = s.serverImpl.GetStatus(); status == nil {
				continue
			}
			if !statusEqual(*status, lastStatus) {
				lastStatus = *status
				log.Debug("GetStatusStream (connectRPC) sending status update")
				if err := stream.Send(composeResponse()); err != nil {
					log.Error("GetStatusStream (connectRPC) error sending status update",
						log.ErrorField(err))
					return err
				}
			}
		}
	}
}

//nolint:whitespace // editor/linter issue
func (s *raceloggerServiceConnectRPC) StartRecording(
	ctx context.Context,
	req *connect.Request[v1.StartRecordingRequest],
) (*connect.Response[v1.StartRecordingResponse], error) {
	log.Debug("StartRecording (connectRPC) called")
	s.serverImpl.StartRecording(req.Msg)
	return connect.NewResponse(&v1.StartRecordingResponse{}), nil
}

//nolint:whitespace // editor/linter issue
func (s *raceloggerServiceConnectRPC) StopRecording(
	ctx context.Context,
	req *connect.Request[v1.StopRecordingRequest],
) (*connect.Response[v1.StopRecordingResponse], error) {
	log.Debug("StoptRecording (connectRPC) called")
	s.serverImpl.StopRecording() // stop the recording context
	return connect.NewResponse(&v1.StopRecordingResponse{}), nil
}
