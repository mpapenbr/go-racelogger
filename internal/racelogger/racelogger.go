//nolint:funlen // keep things together
package racelogger

//nolint:gosec // md5 is used as hash for racing events
import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	commonv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/common/v1"
	eventv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/event/v1"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	trackv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/track/v1"
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
	EventKeyFunc func(api *irsdk.Irsdk) string
	Config       struct {
		ctx                     context.Context
		cancel                  context.CancelFunc
		conn                    *grpc.ClientConn
		eventKeyFunc            EventKeyFunc
		waitForServicesTimeout  time.Duration
		waitForDataTimeout      time.Duration
		speedmapPublishInterval time.Duration
		speedmapSpeedThreshold  float64
		maxSpeed                float64
		recordingMode           providerv1.RecordingMode
		token                   string
		grpcLogFile             string
		ensureLiveData          bool
		ensureLiveDataInterval  time.Duration
		watchdogInterval        time.Duration
		raceSessionRecordedChan chan int32
	}
)
type ConfigFunc func(cfg *Config)

// Racelogger is the main component to control the connection to iRacing Telemetry API
type Racelogger struct {
	eventKey      string
	api           *irsdk.Irsdk
	dataprovider  *grpcDataclient.DataProviderClient
	simIsRunning  bool
	config        *Config
	globalData    processor.GlobalProcessingData
	msgLogger     *os.File
	log           *log.Logger
	simStatusChan chan bool
	httpClient    *http.Client
}

const (
	RACE = "Race"
)

func defaultConfig() *Config {
	return &Config{
		eventKeyFunc:            uuidBasedEventKeyFunc,
		waitForDataTimeout:      1 * time.Second,
		speedmapPublishInterval: 30 * time.Second,
		speedmapSpeedThreshold:  0.5,
		maxSpeed:                500,
		recordingMode:           providerv1.RecordingMode_RECORDING_MODE_PERSIST,
		ensureLiveData:          true,
		ensureLiveDataInterval:  0,
	}
}

func eventBasedEventKeyFunc(api *irsdk.Irsdk) string {
	irYaml, _ := api.GetYaml()
	out, err := goyaml.Marshal(irYaml.WeekendInfo)
	if err != nil {
		log.Warn("Could not marshal WeekendInfo", log.ErrorField(err))
		out = []byte(uuid.New().String())
	}
	sessionNum, _ := api.GetIntValue("SessionNum")
	//nolint:gosec //just used as hash
	h := md5.New()
	h.Write(out)
	h.Write([]byte(strconv.Itoa(int(sessionNum))))
	ret := hex.EncodeToString(h.Sum(nil))
	return ret
}

func uuidBasedEventKeyFunc(api *irsdk.Irsdk) string {
	return uuid.New().String()
}

func WithGrpcConn(conn *grpc.ClientConn) ConfigFunc {
	return func(cfg *Config) { cfg.conn = conn }
}

func WithContext(ctx context.Context, cancelFunc context.CancelFunc) ConfigFunc {
	return func(cfg *Config) { cfg.ctx = ctx; cfg.cancel = cancelFunc }
}

func WithWaitForServicesTimeout(t time.Duration) ConfigFunc {
	return func(cfg *Config) { cfg.waitForServicesTimeout = t }
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

func WithGrpcLogFile(grpcLogFile string) ConfigFunc {
	return func(cfg *Config) { cfg.grpcLogFile = grpcLogFile }
}

func WithEnsureLiveData(b bool) ConfigFunc {
	return func(cfg *Config) { cfg.ensureLiveData = b }
}

func WithEnsureLiveDataInterval(t time.Duration) ConfigFunc {
	return func(cfg *Config) { cfg.ensureLiveDataInterval = t }
}

func WithWatchdogInterval(t time.Duration) ConfigFunc {
	return func(cfg *Config) { cfg.watchdogInterval = t }
}

func WithRaceSessionRecorded(c chan int32) ConfigFunc {
	return func(cfg *Config) { cfg.raceSessionRecordedChan = c }
}

func WithEventKeyFunc(f EventKeyFunc) ConfigFunc {
	return func(cfg *Config) { cfg.eventKeyFunc = f }
}

func WithUUIDEventKey() ConfigFunc {
	return func(cfg *Config) { cfg.eventKeyFunc = uuidBasedEventKeyFunc }
}

func WithEventBasedEventKey() ConfigFunc {
	return func(cfg *Config) { cfg.eventKeyFunc = eventBasedEventKeyFunc }
}

func NewRaceLogger(cfg ...ConfigFunc) *Racelogger {
	c := defaultConfig()
	for _, fn := range cfg {
		fn(c)
	}
	var grpcMsgLog *os.File = nil
	if c.grpcLogFile != "" {
		f, err := os.Create(c.grpcLogFile)
		if err != nil {
			log.Warn("Could not create grpc log file", log.ErrorField(err))
		} else {
			grpcMsgLog = f
		}
	}
	ret := &Racelogger{
		simIsRunning: false,
		dataprovider: grpcDataclient.NewDataProviderClient(
			grpcDataclient.WithConnection(c.conn),
			grpcDataclient.WithToken(c.token),
			grpcDataclient.WithMsgLogFile(grpcMsgLog),
		),
		config:        c,
		msgLogger:     grpcMsgLog,
		log:           log.GetFromContext(c.ctx).Named("rl"),
		simStatusChan: make(chan bool, 1),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}

	if ret.init() {
		return ret
	} else {
		return nil
	}
}

func (r *Racelogger) Close() {
	r.log.Debug("Closing Racelogger")
	r.api.Close()

	if r.msgLogger != nil {
		r.msgLogger.Close()
	}
}

func (r *Racelogger) GetRaceSessions() (all []int, current int32, err error) {
	irYaml, err := r.api.GetYaml()
	if err != nil {
		return []int{}, 0, err
	}
	for i := range irYaml.SessionInfo.Sessions {
		s := irYaml.SessionInfo.Sessions[i]
		if s.SessionType == RACE {
			all = append(all, s.SessionNum)
		}
	}
	current, _ = r.api.GetIntValue("SessionNum")
	return all, current, nil
}

func (r *Racelogger) GetSessionName(sessionNum int32) string {
	irYaml, err := r.api.GetYaml()
	if err != nil {
		return "n.a."
	}
	return irYaml.SessionInfo.Sessions[sessionNum].SessionName
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

	r.eventKey = r.config.eventKeyFunc(r.api)
	event.Key = r.eventKey

	resp, err := r.dataprovider.RegisterProvider(event, track, r.config.recordingMode)
	if err != nil {
		return err
	}
	r.globalData = processor.GlobalProcessingData{
		TrackInfo:     resp.Track,
		EventDataInfo: event,
	}

	return nil
}

//nolint:whitespace // false positive
func (r *Racelogger) RegisterProviderHeat(
	eventName, eventDescription, sessionName string,
) error {
	irYaml, err := r.api.GetYaml()
	if err != nil {
		return err
	}
	event := r.createEventInfo(irYaml)

	track := r.createTrackInfo(irYaml)

	if eventName != "" {
		event.Name = eventName
	} else {
		event.Name = fmt.Sprintf("%s %s %s",
			track.Name,
			event.EventTime.AsTime().Format("20060102-150405"),
			sessionName)
	}
	if eventDescription != "" {
		event.Description = eventDescription
	}

	r.eventKey = r.config.eventKeyFunc(r.api)
	event.Key = r.eventKey

	resp, err := r.dataprovider.RegisterProvider(event, track, r.config.recordingMode)
	if err != nil {
		return err
	}
	r.globalData = processor.GlobalProcessingData{
		TrackInfo:     resp.Track,
		EventDataInfo: event,
	}

	return nil
}

func (r *Racelogger) UnregisterProvider() {
	if err := r.dataprovider.UnregisterProvider(r.eventKey); err != nil {
		log.Warn("Could not unregister event",
			log.String("eventKey", r.eventKey),
			log.ErrorField(err))
	}
}

// this will start the recording in a goroutine.
// call returns immediately
func (r *Racelogger) StartRecording() {
	log.Debug("Starting recording")
	r.setupMainLoop()
}

func (r *Racelogger) init() bool {
	initSim := make(chan bool, 1)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		r.config.waitForServicesTimeout)
	defer cancel()
	r.log.Debug("Waiting for iRacing simulation to be ready")
	go r.initConnectionToSim(ctx, initSim)
	res := <-initSim
	if res {
		r.log.Debug("Connected to iRacing simulation")
		if r.config.watchdogInterval > 0 {
			r.log.Debug("Setting up watchdog",
				log.Duration("interval", r.config.watchdogInterval))
			go r.setupWatchdog(r.config.ctx, r.config.watchdogInterval)
		}
		r.simIsRunning = true
	} else {
		r.log.Error("Could not connect to iRacing simulation")
	}
	return res
}

func (r *Racelogger) setupWatchdog(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)

	for {
		select {
		case <-ctx.Done():
			r.log.Debug("setupWatchdog received ctx.Done")
			ticker.Stop()
			return
		case <-ticker.C:
			simAvail, err := irsdk.IsSimRunning(ctx, r.httpClient)
			if err != nil {
				r.log.Debug("Error checking if sim is running", log.ErrorField(err))
			} else {
				r.log.Debug("Sim status", log.Bool("simRunning", simAvail))
				r.simIsRunning = simAvail
				r.simStatusChan <- simAvail
			}
		}
	}
}

//nolint:gocognit // by design
func (r *Racelogger) initConnectionToSim(ctx context.Context, result chan<- bool) {
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			result <- false
			return
		case <-ticker.C:
			simAvail, err := irsdk.IsSimRunning(ctx, r.httpClient)
			if err != nil {
				r.log.Debug("Error checking if sim is running", log.ErrorField(err))
				break
			}
			//nolint:nestif // by design
			if simAvail {
				r.log.Debug("Sim is running")
				api := irsdk.NewIrsdk()
				api.WaitForValidData()
				if !r.hasValidAPIData(api) {
					api.Close()
					r.log.Debug("iRacing telemetry data not yet ready. Need retry")
				} else {
					ticker.Stop()
					if r.config.ensureLiveData {
						//nolint:errcheck // by design
						api.ReplaySearch(irsdk.ReplaySearchModeEnd)
					}
					api.GetData()
					r.api = api
					result <- true
					return
				}
			}
		}
	}
}

func (r *Racelogger) hasValidAPIData(api *irsdk.Irsdk) bool {
	api.GetData()
	return len(api.GetValueKeys()) > 0 && r.hasPlausibleYaml(api)
}

// the yaml data is considered valid if certain plausible values are present.
// for example: the track length must be > 0, track sectors are present
func (r *Racelogger) hasPlausibleYaml(api *irsdk.Irsdk) bool {
	ret := true
	y, err := api.GetYaml()
	if err != nil {
		return false
	}
	if y.WeekendInfo.NumCarTypes == 0 {
		ret = false
	}
	if y.WeekendInfo.TrackID == 0 {
		ret = false
	}
	if len(y.SplitTimeInfo.Sectors) == 0 {
		ret = false
	}
	if len(y.SessionInfo.Sessions) == 0 {
		ret = false
	}
	return ret
}

func (r *Racelogger) createEventInfo(irYaml *yaml.IrsdkYaml) *eventv1.Event {
	pitSpeed, _ := processor.GetMetricUnit(irYaml.WeekendInfo.TrackPitSpeedLimit)
	event := eventv1.Event{
		TrackId:           uint32(irYaml.WeekendInfo.TrackID),
		MultiClass:        irYaml.WeekendInfo.NumCarClasses > 1,
		NumCarTypes:       uint32(irYaml.WeekendInfo.NumCarTypes),
		TeamRacing:        irYaml.WeekendInfo.TeamRacing > 0,
		IrSessionId:       int32(irYaml.WeekendInfo.SessionID),
		IrSubSessionId:    int32(irYaml.WeekendInfo.SubSessionID),
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
		Id:        uint32(irYaml.WeekendInfo.TrackID),
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
func (r *Racelogger) convertSessions(sessions []yaml.Sessions) []*eventv1.Session {
	ret := make([]*eventv1.Session, len(sessions))
	for i, v := range sessions {
		// value is "xxx.0000 sec", so we can use our conversion function
		// (even though it is not a metric depending value)
		time, _ := processor.GetMetricUnit(v.SessionTime)

		laps := 0
		if v.SessionLaps != "unlimited" {
			laps, _ = strconv.Atoi(v.SessionLaps)
		}

		ret[i] = &eventv1.Session{
			Num:         uint32(v.SessionNum),
			Name:        v.SessionName,
			SessionTime: int32(time),
			Laps:        int32(laps), //nolint:gosec // by design
			Type:        convertSessionType(v.SessionType),
			SubType:     convertSessionSubType(v.SessionSubType),
		}
	}
	return ret
}

//nolint:gocognit,cyclop // by design
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
		processor.WithContext(r.config.ctx),
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
				r.log.Debug("mainLoop received ctx.Done")
				return
			case _, more := <-recordingDoneChannel:
				r.log.Debug("mainLoop received recordingDoneChannel", log.Bool("more", more))
				if !more {
					r.log.Info("Recording done.")
					current, _ := r.api.GetIntValue("SessionNum")
					r.config.raceSessionRecordedChan <- current
					return
				}
			case simStatus := <-r.simStatusChan:
				if !simStatus {
					r.log.Warn("Sim is not running. Stopping")
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
					r.logDurations("getData", getDataDurations)
					getDataDurations = []time.Duration{}
				}
				if ok {
					startProc := time.Now()
					proc.Process()
					procDurations = append(procDurations, time.Since(startProc))

					if len(procDurations) == 120 {
						r.logDurations("processedData", procDurations)
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

//nolint:gocognit // by design
func (r *Racelogger) WaitForNextRaceSession(lastRaceSessionNum int32) int32 {
	ticker := time.NewTicker(2 * time.Second)
	nextSessionChan := make(chan int32, 1)
	waitLoop := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				r.log.Debug("mainLoop received ctx.Done")
				ticker.Stop()
				return

			case simStatus := <-r.simStatusChan:
				if !simStatus {
					r.log.Warn("Sim is not running. Stopping")
					ticker.Stop()
					r.config.cancel()
					return
				}
			case <-ticker.C:
				ok := r.api.GetDataWithDataReadyTimeout(r.config.waitForDataTimeout)
				if ok {
					sessionNum, _ := r.api.GetIntValue("SessionNum")
					r.log.Debug("Waiting for session to start",
						log.Int32("lastRaceSessionNum", lastRaceSessionNum),
						log.Int32("current", sessionNum),
					)
					y := r.api.GetLatestYaml()
					if sessionNum != lastRaceSessionNum &&
						y.SessionInfo.Sessions[sessionNum].SessionType == RACE {

						r.log.Info("Next race session  started")
						ticker.Stop()
						nextSessionChan <- sessionNum
						return
					}
				}
			}
		}
	}

	go waitLoop(r.config.ctx)
	r.log.Debug("Waiting for next race session to start")
	ret := <-nextSessionChan
	r.log.Debug("next session started", log.Int32("sessionNum", ret))
	return ret
}

func (r *Racelogger) logDurations(msg string, durations []time.Duration) {
	myLog := r.log.Named("durations")
	minTime := 1 * time.Second
	maxTime := time.Duration(0)
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
		if v < minTime {
			minTime = v
		}
		if v > maxTime {
			maxTime = v
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
	myLog.Debug(msg,
		log.Int("zeroDurations", zeroDurations),
		log.Int("validDurations", validDurations),
		log.Duration("min", minTime),
		log.Duration("max", maxTime),
		log.Duration("avg", avg),
		log.String("durations", strings.Join(durationsStrs, ",")))
}

func convertSessionType(apiValue string) commonv1.SessionType {
	switch apiValue {
	case "Practice":
		return commonv1.SessionType_SESSION_TYPE_PRACTICE
	case "Open Qualify":
		return commonv1.SessionType_SESSION_TYPE_OPEN_QUALIFY
	case "Lone Qualify":
		return commonv1.SessionType_SESSION_TYPE_LONE_QUALIFY
	case "Warmup":
		return commonv1.SessionType_SESSION_TYPE_WARMUP
	case RACE:
		return commonv1.SessionType_SESSION_TYPE_RACE
	default:
		return commonv1.SessionType_SESSION_TYPE_UNSPECIFIED
	}
}

func convertSessionSubType(apiValue string) commonv1.SessionSubType {
	switch apiValue {
	case "Heat":
		return commonv1.SessionSubType_SESSION_SUB_TYPE_HEAT
	case "Consolation":
		return commonv1.SessionSubType_SESSION_SUB_TYPE_CONSOLATION
	case "Feature":
		return commonv1.SessionSubType_SESSION_SUB_TYPE_FEATURE
	default:
		return commonv1.SessionSubType_SESSION_SUB_TYPE_UNSPECIFIED
	}
}
