//nolint:funlen // keep things together
package internal

//nolint:gosec // md5 is used as hash for racing events
import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	eventv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/event/v1"
	providerv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/provider/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/racestate/v1"
	trackv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/track/v1"
	"github.com/google/uuid"
	"github.com/mpapenbr/goirsdk/irsdk"
	"github.com/mpapenbr/goirsdk/yaml"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	goyaml "gopkg.in/yaml.v3"

	"github.com/mpapenbr/go-racelogger/internal/processor"
	"github.com/mpapenbr/go-racelogger/log"
	grpcDataclient "github.com/mpapenbr/go-racelogger/pkg/grpc"
	"github.com/mpapenbr/go-racelogger/version"
)

type (
	EventKeyFunc func(*yaml.IrsdkYaml) string
	Config       struct {
		ctx                     context.Context
		cancel                  context.CancelFunc
		conn                    *grpc.ClientConn
		eventKeyFunc            EventKeyFunc
		waitForDataTimeout      time.Duration
		speedmapPublishInterval time.Duration
		speedmapSpeedThreshold  float64
		maxSpeed                float64
		recordingMode           providerv1.RecordingMode
		token                   string
	}
)
type ConfigFunc func(cfg *Config)

// Racelogger is the main component to control the connection to iRacing Telemetry API
type Racelogger struct {
	eventKey     string
	api          *irsdk.Irsdk
	dataprovider *grpcDataclient.DataProviderClient
	simIsRunning bool
	config       *Config
	globalData   processor.GlobalProcessingData
}

func defaultConfig() *Config {
	return &Config{
		eventKeyFunc:            defaultEventKeyFunc,
		waitForDataTimeout:      1 * time.Second,
		speedmapPublishInterval: 30 * time.Second,
		speedmapSpeedThreshold:  0.5,
		maxSpeed:                500,
		recordingMode:           providerv1.RecordingMode_RECORDING_MODE_PERSIST,
	}
}

func defaultEventKeyFunc(irYaml *yaml.IrsdkYaml) string {
	out, err := goyaml.Marshal(irYaml.WeekendInfo)
	if err != nil {
		log.Warn("Could not marshal WeekendInfo", log.ErrorField(err))
		out = []byte(uuid.New().String())
	}
	//nolint:gosec //just used as hash
	h := md5.New()
	h.Write(out)
	ret := hex.EncodeToString(h.Sum(nil))
	return ret
}

func WithGrpcConn(conn *grpc.ClientConn) ConfigFunc {
	return func(cfg *Config) { cfg.conn = conn }
}

func WithContext(ctx context.Context, cancelFunc context.CancelFunc) ConfigFunc {
	return func(cfg *Config) { cfg.ctx = ctx; cfg.cancel = cancelFunc }
}

func WithWaitForDataTimeout(t time.Duration) ConfigFunc {
	return func(cfg *Config) { cfg.waitForDataTimeout = t }
}

func WithSpeedmapPublishInterval(t time.Duration) ConfigFunc {
	return func(cfg *Config) { cfg.speedmapPublishInterval = t }
}

func WithSpeedmapSpeedThreshold(f float64) ConfigFunc {
	return func(cfg *Config) { cfg.speedmapSpeedThreshold = f }
}

func WithMaxSpeed(f float64) ConfigFunc {
	return func(cfg *Config) { cfg.maxSpeed = f }
}

func WithRecordingMode(mode providerv1.RecordingMode) ConfigFunc {
	return func(cfg *Config) { cfg.recordingMode = mode }
}

func WithToken(token string) ConfigFunc {
	return func(cfg *Config) { cfg.token = token }
}

func NewRaceLogger(cfg ...ConfigFunc) *Racelogger {
	c := defaultConfig()
	for _, fn := range cfg {
		fn(c)
	}

	ret := &Racelogger{
		simIsRunning: false,
		dataprovider: grpcDataclient.NewDataProviderClient(
			grpcDataclient.WithConnection(c.conn),
			grpcDataclient.WithToken(c.token),
		),
		config: c,
	}

	ret.init()
	return ret
}

func (r *Racelogger) Close() {
	log.Debug("Closing Racelogger")
	r.api.Close()
	r.dataprovider.Close()
}

func (r *Racelogger) RegisterProvider(eventName, eventDescription string) error {
	irYaml, err := r.api.GetYaml()
	if err != nil {
		return err
	}
	event := r.createEventInfo(irYaml)

	track := r.createTrackInfo(irYaml)

	if eventName != "" {
		event.Name = eventName
	} else {
		event.Name = fmt.Sprintf("%s %s",
			track.Name,
			event.EventTime.AsTime().Format("20060102-150405"))
	}
	if eventDescription != "" {
		event.Description = eventDescription
	}

	r.eventKey = r.config.eventKeyFunc(irYaml)
	event.Key = r.eventKey
	r.globalData = processor.GlobalProcessingData{
		TrackInfo:     track,
		EventDataInfo: event,
	}

	err = r.dataprovider.RegisterProvider(event, track, r.config.recordingMode)
	if err != nil {
		return err
	}

	r.setupMainLoop()
	return nil
}

func (r *Racelogger) UnregisterProvider() {
	if err := r.dataprovider.UnregisterProvider(r.eventKey); err != nil {
		log.Warn("Could not unregister event",
			log.String("eventKey", r.eventKey),
			log.ErrorField(err))
	}
}

func (r *Racelogger) init() {
	r.setupWatchdog(time.Second)
	log.Debug("Ensure iRacing simulation is ready to provide data")
	for {
		if r.simIsRunning {
			break
		} else {
			log.Debug("Waiting for initialized simulation")
			time.Sleep(time.Second)
		}
	}
	log.Debug("Telemetry data is available")
}

func (r *Racelogger) createEventInfo(irYaml *yaml.IrsdkYaml) *eventv1.Event {
	pitSpeed, _ := processor.GetMetricUnit(irYaml.WeekendInfo.TrackPitSpeedLimit)
	event := eventv1.Event{
		TrackId:           uint32(irYaml.WeekendInfo.TrackID),
		MultiClass:        irYaml.WeekendInfo.NumCarClasses > 1,
		NumCarTypes:       uint32(irYaml.WeekendInfo.NumCarTypes),
		TeamRacing:        irYaml.WeekendInfo.TeamRacing > 0,
		IrSessionId:       int32(irYaml.WeekendInfo.SessionID),
		RaceloggerVersion: version.Version,
		EventTime:         timestamppb.Now(),
		Sessions:          r.convertSessions(irYaml.SessionInfo.Sessions),
		NumCarClasses:     uint32(irYaml.WeekendInfo.NumCarClasses),
		PitSpeed:          float32(pitSpeed),
	}
	return &event
}

func (r *Racelogger) createTrackInfo(irYaml *yaml.IrsdkYaml) *trackv1.Track {
	trackLength, _ := processor.GetTrackLengthInMeters(irYaml.WeekendInfo.TrackLength)
	pitSpeed, _ := processor.GetMetricUnit(irYaml.WeekendInfo.TrackPitSpeedLimit)
	ret := trackv1.Track{
		Id:        &trackv1.TrackId{Id: uint32(irYaml.WeekendInfo.TrackID)},
		Name:      irYaml.WeekendInfo.TrackDisplayName,
		ShortName: irYaml.WeekendInfo.TrackDisplayShortName,
		Config:    irYaml.WeekendInfo.TrackConfigName,
		Length:    float32(trackLength),
		PitSpeed:  float32(pitSpeed),

		Sectors: r.convertSectors(irYaml.SplitTimeInfo.Sectors),
	}
	return &ret
}

func (r *Racelogger) convertSectors(sectors []yaml.Sectors) []*trackv1.Sector {
	ret := make([]*trackv1.Sector, len(sectors))
	for i, v := range sectors {
		ret[i] = &trackv1.Sector{
			Num:      uint32(v.SectorNum),
			StartPct: float32(v.SectorStartPct),
		}
	}
	return ret
}

//nolint:gocritic // by design
func (r *Racelogger) convertSessions(sectors []yaml.Sessions) []*eventv1.Session {
	ret := make([]*eventv1.Session, len(sectors))
	for i, v := range sectors {
		ret[i] = &eventv1.Session{Num: uint32(v.SessionNum), Name: v.SessionName}
	}
	return ret
}

//nolint:gocognit // by design
func (r *Racelogger) setupWatchdog(interval time.Duration) {
	postData := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				log.Debug("watchdog received ctx.Done")
				return
			default:
				if irsdk.CheckIfSimIsRunning() {
					if r.api == nil {
						log.Debug("Initializing irsdk api")

						r.api = irsdk.NewIrsdk()
						log.Debug("waiting some seconds before start")
						time.Sleep(5 * time.Second)

						r.api.WaitForValidData()
						// as long as there are no entries we have to try again
						for len(r.api.GetValueKeys()) == 0 {
							r.api.Close()
							log.Debug("iRacing not yet ready. Retrying in 5s")
							time.Sleep(5 * time.Second)
							r.api = irsdk.NewIrsdk()
							r.api.WaitForValidData()
						}
						r.api.GetData()
						r.simIsRunning = true
					}
				} else {
					if r.api != nil {
						log.Debug("Resetting irsdk api")
						r.api.Close()
					}
					r.api = nil
					r.simIsRunning = false
				}

				time.Sleep(interval)
			}
		}
	}

	go postData(r.config.ctx)
}

//nolint:gocognit // by design
func (r *Racelogger) setupMainLoop() {
	stateChannel := make(chan *racestatev1.PublishStateRequest, 2)
	speedmapChannel := make(chan *racestatev1.PublishSpeedmapRequest, 1)
	carDataChannel := make(chan *racestatev1.PublishDriverDataRequest, 1)
	extraInfoChannel := make(chan *racestatev1.PublishEventExtraInfoRequest, 1)

	recordingDoneChannel := make(chan struct{}, 1)

	proc := processor.NewProcessor(
		r.api,
		stateChannel,
		speedmapChannel,
		carDataChannel,
		extraInfoChannel,
		processor.WithGlobalProcessingData(&r.globalData),
		processor.WithChunkSize(10),
		processor.WithRecordingDoneChannel(recordingDoneChannel),
		processor.WithSpeedmapPublishInterval(r.config.speedmapPublishInterval),
		processor.WithSpeedmapSpeedThreshold(r.config.speedmapSpeedThreshold),
		processor.WithMaxSpeed(r.config.maxSpeed),
	)

	r.dataprovider.PublishStateFromChannel(r.eventKey, stateChannel)
	r.dataprovider.PublishSpeedmapDataFromChannel(r.eventKey, speedmapChannel)
	r.dataprovider.PublishCarDataFromChannel(r.eventKey, carDataChannel)
	r.dataprovider.SendExtraInfoFromChannel(r.eventKey, extraInfoChannel)

	mainLoop := func(ctx context.Context) {
		procDurations := []time.Duration{}
		getDataDurations := []time.Duration{}
		for {
			select {
			case <-ctx.Done():
				log.Debug("mainLoop received ctx.Done")
				return
			case _, more := <-recordingDoneChannel:
				log.Debug("mainLoop received recordingDoneChannel", log.Bool("more", more))
				if !more {
					log.Info("Recording done.")
					r.config.cancel()
					return
				}
			default:
				if !r.simIsRunning {
					log.Warn("Sim is not running. Should not happen",
						log.String("method", "mainLoop"))
					time.Sleep(time.Second)
					continue
				}

				startGetData := time.Now()
				ok := r.api.GetDataWithDataReadyTimeout(r.config.waitForDataTimeout)
				getDataDurations = append(getDataDurations, time.Since(startGetData))
				if len(getDataDurations) == 120 {
					logDurations("getData", getDataDurations)
					getDataDurations = []time.Duration{}
				}
				if ok {
					startProc := time.Now()
					proc.Process()
					procDurations = append(procDurations, time.Since(startProc))

					if len(procDurations) == 120 {
						logDurations("processedData", procDurations)
						procDurations = []time.Duration{}
					}
				} else {
					log.Warn("no new data available")
				}
			}
		}
	}

	go mainLoop(r.config.ctx)
}

func logDurations(msg string, durations []time.Duration) {
	min := 1 * time.Second
	max := time.Duration(0)
	sum := int64(0)
	avg := time.Duration(0)
	zeroDurations := 0
	validDurations := 0
	for _, v := range durations {
		if v.Nanoseconds() == 0 {
			zeroDurations++
			continue
		}
		validDurations++
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v.Nanoseconds()
	}

	durationsStrs := make([]string, len(durations))
	for i, d := range durations {
		durationsStrs[i] = d.String()
	}
	if validDurations > 0 {
		avg = time.Duration(sum / int64(validDurations))
	}
	log.Debug(msg,
		log.Int("zeroDurations", zeroDurations),
		log.Int("validDurations", validDurations),
		log.Duration("min", min),
		log.Duration("max", max),
		log.Duration("avg", avg),
		log.String("durations", strings.Join(durationsStrs, ",")))
}
