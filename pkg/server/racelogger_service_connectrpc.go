package server

import (
	"context"

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

//nolint:whitespace,funlen // ok here
func (s *raceloggerServiceConnectRPC) GetStatusStream(
	ctx context.Context,
	req *connect.Request[v1.GetStatusStreamRequest],
	stream *connect.ServerStream[v1.GetStatusStreamResponse],
) error {
	log.Debug("GetStatusStream called")

	statusChan := s.serverImpl.SubscribeStatus()
	cleanup := func() {
		log.Debug("GetStatusStream cleanup called")
		s.serverImpl.UnsubscribeStatus(statusChan)
		log.Debug("GetStatusStream cleanup done")
	}
	defer cleanup()
	go func() {
		<-ctx.Done()
		log.Debug("calling cleanup")
		cleanup()
		log.Debug("after cleanup")
	}()
	composeRaceSessions := func(status *myStatus) []*v1.RaceSession {
		raceSessions := make([]*v1.RaceSession, 0, len(status.RaceSessions))
		for _, rs := range status.RaceSessions {
			raceSessions = append(raceSessions, &v1.RaceSession{
				Num:  rs.Num,
				Name: rs.Name,
			})
		}
		return raceSessions
	}
	composeResponse := func(status *myStatus) *v1.GetStatusStreamResponse {
		return &v1.GetStatusStreamResponse{
			BackendAvailable:   status.BackendAvailable,
			BackendCompatible:  status.BackendCompatible,
			ValidCredentials:   status.ValidCredentials,
			SimulationRunning:  status.SimulationRunning,
			TelemetryAvailable: status.TelemetryAvailable,
			RecordingActive:    status.Recording,
			CurrentSessionNum:  status.CurrentSessionNum,
			RaceSessions:       composeRaceSessions(status),
		}
	}
	for status := range statusChan {
		if err := stream.Send(composeResponse(&status)); err != nil {
			log.Error("GetStatusStream error sending status",
				log.ErrorField(err))
			return err
		}
	}
	return nil
}

//nolint:whitespace // editor/linter issue
func (s *raceloggerServiceConnectRPC) StartRecording(
	ctx context.Context,
	req *connect.Request[v1.StartRecordingRequest],
) (*connect.Response[v1.StartRecordingResponse], error) {
	log.Debug("StartRecording called")
	s.serverImpl.StartRecording(req.Msg)
	return connect.NewResponse(&v1.StartRecordingResponse{}), nil
}

//nolint:whitespace // editor/linter issue
func (s *raceloggerServiceConnectRPC) StopRecording(
	ctx context.Context,
	req *connect.Request[v1.StopRecordingRequest],
) (*connect.Response[v1.StopRecordingResponse], error) {
	log.Debug("StoptRecording called")
	s.serverImpl.StopRecording() // stop the recording context
	return connect.NewResponse(&v1.StopRecordingResponse{}), nil
}
