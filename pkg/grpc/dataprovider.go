package grpc

import (
	"context"
	"io"

	providerv1grpc "buf.build/gen/go/mpapenbr/testrepo/grpc/go/testrepo/provider/v1/providerv1grpc"
	racestatev1grpc "buf.build/gen/go/mpapenbr/testrepo/grpc/go/testrepo/racestate/v1/racestatev1grpc"
	commonv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/common/v1"
	eventv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/event/v1"
	providerv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/provider/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/racestate/v1"
	trackv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/track/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/grpc/logger"
)

type DataProviderClient struct {
	conn           *grpc.ClientConn
	providerClient providerv1grpc.ProviderServiceClient
	stateClient    racestatev1grpc.RaceStateServiceClient
	token          string

	msgLogger *logger.MsgLogger
}

type Option func(*DataProviderClient)

func NewDataProviderClient(opts ...Option) *DataProviderClient {
	ret := &DataProviderClient{}
	for _, opt := range opts {
		opt(ret)
	}
	return ret
}

func WithConnection(conn *grpc.ClientConn) Option {
	return func(dpc *DataProviderClient) {
		dpc.conn = conn
		dpc.providerClient = providerv1grpc.NewProviderServiceClient(conn)
		dpc.stateClient = racestatev1grpc.NewRaceStateServiceClient(conn)
	}
}

func WithToken(token string) Option {
	return func(dpc *DataProviderClient) {
		dpc.token = token
	}
}

func WithMsgLogFile(writer io.Writer) Option {
	return func(dpc *DataProviderClient) {
		dpc.msgLogger = logger.NewMsgLogger(logger.WithWriter(writer))
	}
}

func (dpc *DataProviderClient) Close() {
	dpc.conn.Close()
}

//nolint:whitespace // by design
func (dpc *DataProviderClient) RegisterProvider(
	event *eventv1.Event,
	track *trackv1.Track,
	recordingMode providerv1.RecordingMode,
) error {
	req := providerv1.RegisterEventRequest{
		Event: event, Track: track, Key: event.Key, RecordingMode: recordingMode,
	}
	//nolint:errcheck // by design
	dpc.msgLogger.Log(req.ProtoReflect())
	_, err := dpc.providerClient.RegisterEvent(
		dpc.prepareContext(context.Background()), &req)
	return err
}

func (dpc *DataProviderClient) prepareContext(ctx context.Context) context.Context {
	md := metadata.Pairs("api-token", dpc.token)
	return metadata.NewOutgoingContext(ctx, md)
}

func (dpc *DataProviderClient) UnregisterProvider(eventKey string) error {
	req := providerv1.UnregisterEventRequest{
		EventSelector: &commonv1.EventSelector{Arg: &commonv1.EventSelector_Key{
			Key: eventKey,
		}},
	}
	//nolint:errcheck // by design
	dpc.msgLogger.Log(req.ProtoReflect())
	_, err := dpc.providerClient.UnregisterEvent(
		dpc.prepareContext(context.Background()), &req)
	return err
}

//nolint:whitespace // by design
func (dpc *DataProviderClient) PublishStateFromChannel(
	eventKey string,
	rcv chan *racestatev1.PublishStateRequest,
) {
	go func() {
		for {
			s, more := <-rcv
			if s != nil {
				//nolint:errcheck // by design
				dpc.msgLogger.Log(s.ProtoReflect())
				_, err := dpc.stateClient.PublishState(
					dpc.prepareContext(context.Background()), s)
				if err != nil {
					log.Error("Error publishing state data", log.ErrorField(err))
				}
			}

			if !more {
				log.Debug("closed channel signaled")
				return
			}
		}
	}()
}

//nolint:whitespace // by design
func (dpc *DataProviderClient) PublishCarDataFromChannel(
	eventKey string,
	rcv chan *racestatev1.PublishDriverDataRequest,
) {
	go func() {
		for {
			s, more := <-rcv

			if s != nil {
				//nolint:errcheck // by design
				dpc.msgLogger.Log(s.ProtoReflect())
				_, err := dpc.stateClient.PublishDriverData(
					dpc.prepareContext(context.Background()), s)
				if err != nil {
					log.Error("Error publishing driver data", log.ErrorField(err))
				}
			}

			if !more {
				log.Debug("closed channel signaled")
				return
			}
		}
	}()
}

//nolint:whitespace // by design
func (dpc *DataProviderClient) PublishSpeedmapDataFromChannel(
	eventKey string,
	rcv chan *racestatev1.PublishSpeedmapRequest,
) {
	go func() {
		for {
			s, more := <-rcv

			if s != nil {
				//nolint:errcheck // by design
				dpc.msgLogger.Log(s.ProtoReflect())
				_, err := dpc.stateClient.PublishSpeedmap(
					dpc.prepareContext(context.Background()), s)
				if err != nil {
					log.Error("Error publishing speedmap data", log.ErrorField(err))
				}
			}

			if !more {
				log.Debug("closed channel signaled")
				return
			}
		}
	}()
}

//nolint:whitespace // by design
func (dpc *DataProviderClient) SendExtraInfoFromChannel(
	eventKey string,
	rcv chan *racestatev1.PublishEventExtraInfoRequest,
) {
	go func() {
		for {
			extra, more := <-rcv
			if extra != nil {
				//nolint:errcheck // by design
				dpc.msgLogger.Log(extra.ProtoReflect())
				_, err := dpc.stateClient.PublishEventExtraInfo(
					dpc.prepareContext(context.Background()), extra)
				if err != nil {
					log.Error("Error publishing extra info", log.ErrorField(err))
				}
			}

			if !more {
				log.Debug("closed channel signaled")
				return
			}
		}
	}()
}
