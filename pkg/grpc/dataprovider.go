package grpc

import (
	"context"
	"io"

	eventv1grpc "buf.build/gen/go/mpapenbr/iracelog/grpc/go/iracelog/event/v1/eventv1grpc"
	providerv1grpc "buf.build/gen/go/mpapenbr/iracelog/grpc/go/iracelog/provider/v1/providerv1grpc"
	racestatev1grpc "buf.build/gen/go/mpapenbr/iracelog/grpc/go/iracelog/racestate/v1/racestatev1grpc"
	commonv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/common/v1"
	eventv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/event/v1"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	trackv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/track/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/grpc/logger"
)

type DataProviderClient struct {
	conn           *grpc.ClientConn
	providerClient providerv1grpc.ProviderServiceClient
	stateClient    racestatev1grpc.RaceStateServiceClient
	eventClient    eventv1grpc.EventServiceClient
	token          string

	msgLogger *logger.MsgLogger
}

type Option func(*DataProviderClient)

func NewDataProviderClient(opts ...Option) *DataProviderClient {
	ret := &DataProviderClient{
		msgLogger: logger.NewMsgLogger(),
	}
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
		dpc.eventClient = eventv1grpc.NewEventServiceClient(conn)
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

//nolint:whitespace // by design
func (dpc *DataProviderClient) RegisterProvider(
	event *eventv1.Event,
	track *trackv1.Track,
	recordingMode providerv1.RecordingMode,
) (*providerv1.RegisterEventResponse, error) {
	req := providerv1.RegisterEventRequest{
		Event: event, Track: track, Key: event.Key, RecordingMode: recordingMode,
	}
	//nolint:errcheck // by design
	dpc.msgLogger.Log(req.ProtoReflect())
	resp, err := dpc.providerClient.RegisterEvent(
		dpc.prepareContext(context.Background()), &req)
	return resp, err
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

func (dpc *DataProviderClient) DeleteEvent(eventKey string) error {
	req := eventv1.DeleteEventRequest{
		EventSelector: &commonv1.EventSelector{Arg: &commonv1.EventSelector_Key{
			Key: eventKey,
		}},
	}
	_, err := dpc.eventClient.DeleteEvent(
		dpc.prepareContext(context.Background()), &req)
	return err
}

//nolint:whitespace,nestif,gocognit // by design
func (dpc *DataProviderClient) PublishStateFromChannel(
	eventKey string,
	rcv chan *racestatev1.PublishStateRequest,
) {
	go func() {
		errorCounter := 0
		for {
			s, more := <-rcv
			if s != nil {
				err := dpc.PublishState(s)
				if err != nil {
					if errorCounter%30 == 0 {
						log.Error("Error publishing state data",
							log.Int("errorCounter", errorCounter+1),
							log.ErrorField(err))
					}
					errorCounter++
				} else {
					if errorCounter > 0 {
						log.Info("Published state data successful again",
							log.Int("errorCounter", errorCounter))
					}
					errorCounter = 0
				}
			}
			if !more {
				log.Debug("closed channel signaled")
				return
			}
		}
	}()
}

func (dpc *DataProviderClient) PublishState(
	req *racestatev1.PublishStateRequest,
) error {
	//nolint:errcheck // by design
	dpc.msgLogger.Log(req.ProtoReflect())
	_, err := dpc.stateClient.PublishState(
		dpc.prepareContext(context.Background()), req)
	return err
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
				err := dpc.PublishDriverData(s)
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

func (dpc *DataProviderClient) PublishDriverData(
	req *racestatev1.PublishDriverDataRequest,
) error {
	//nolint:errcheck // by design
	dpc.msgLogger.Log(req.ProtoReflect())
	_, err := dpc.stateClient.PublishDriverData(
		dpc.prepareContext(context.Background()), req)
	return err
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
				err := dpc.PublishSpeedmap(s)
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

func (dpc *DataProviderClient) PublishSpeedmap(
	req *racestatev1.PublishSpeedmapRequest,
) error {
	//nolint:errcheck // by design
	dpc.msgLogger.Log(req.ProtoReflect())
	_, err := dpc.stateClient.PublishSpeedmap(
		dpc.prepareContext(context.Background()), req)
	return err
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
				err := dpc.PublishEventExtraInfo(extra)
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

func (dpc *DataProviderClient) PublishEventExtraInfo(
	req *racestatev1.PublishEventExtraInfoRequest,
) error {
	//nolint:errcheck // by design
	dpc.msgLogger.Log(req.ProtoReflect())
	_, err := dpc.stateClient.PublishEventExtraInfo(
		dpc.prepareContext(context.Background()), req)
	return err
}
