package recorder

import (
	"context"
	"time"

	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	"google.golang.org/grpc"

	"github.com/mpapenbr/go-racelogger/internal/racelogger"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
)

type contextData struct {
	ctx    context.Context
	cancel context.CancelFunc
}
type Recorder struct {
	conn *grpc.ClientConn
	cli  *config.CliArgs
	l    *log.Logger
	waitForData,
	waitForServicesTimeout,
	speedmapPublishInterval,
	ensureLiveDataInterval,
	watchdogInterval time.Duration
	recordingMode           providerv1.RecordingMode
	overallCtx              contextData
	raceSessionRecordedChan chan int32
	raceSessions            []int32
	currentSession          int32
	rl                      *racelogger.Racelogger
	eventNames              []string
	eventDescriptions       []string
}
type Option func(*Recorder)

func WithConnection(conn *grpc.ClientConn) Option {
	return func(r *Recorder) {
		r.conn = conn
	}
}

func WithCliArgs(cfg *config.CliArgs) Option {
	return func(r *Recorder) {
		r.initFromCLI(cfg)
	}
}

func WithContext(ctx context.Context, cancel context.CancelFunc) Option {
	return func(r *Recorder) {
		r.overallCtx = contextData{ctx: ctx, cancel: cancel}
		if log.GetFromContext(ctx) != nil {
			r.l = log.GetFromContext(ctx).Named("rec")
		} else {
			r.l = log.Default().Named("rec")
		}
	}
}

func WithEventNames(arg []string) Option {
	return func(r *Recorder) { r.eventNames = arg }
}

func WithEventDescriptions(arg []string) Option {
	return func(r *Recorder) { r.eventDescriptions = arg }
}

func NewRecorder(opts ...Option) *Recorder {
	ret := &Recorder{
		eventNames:        []string{},
		eventDescriptions: []string{},
	}
	for _, opt := range opts {
		opt(ret)
	}
	ret.raceSessionRecordedChan = make(chan int32)
	return ret
}

func (r *Recorder) initFromCLI(cfg *config.CliArgs) {
	var err error
	r.cli = cfg
	r.waitForData, err = time.ParseDuration(cfg.WaitForData)
	if err != nil {
		r.waitForData = time.Second
	}
	r.waitForServicesTimeout, err = time.ParseDuration(cfg.WaitForServices)
	if err != nil {
		r.waitForServicesTimeout = time.Minute
	}
	r.speedmapPublishInterval, err = time.ParseDuration(cfg.SpeedmapPublishInterval)
	if err != nil {
		r.speedmapPublishInterval = 30 * time.Second
	}
	r.ensureLiveDataInterval, err = time.ParseDuration(cfg.EnsureLiveDataInterval)
	if err != nil {
		r.ensureLiveDataInterval = 0
	}
	r.watchdogInterval, err = time.ParseDuration(cfg.WatchdogInterval)
	if err != nil {
		r.watchdogInterval = 5 * time.Second
	}

	r.recordingMode = providerv1.RecordingMode_RECORDING_MODE_PERSIST
	if cfg.DoNotPersist {
		r.recordingMode = providerv1.RecordingMode_RECORDING_MODE_DO_NOT_PERSIST
	}
}

//nolint:funlen,nestif,gocognit // by design
func (r *Recorder) Start() {
	// loop until all race sessions are recorded
	r.collectRaceSessions()
	raceIndex := 0 // used to get name/description from cli args
	recorderLoop := func() {
		for {
			select {
			case <-r.overallCtx.ctx.Done():
				r.l.Debug("Overall context done")
				return
			case raceSessionDone := <-r.raceSessionRecordedChan:
				r.l.Debug("Race session done", log.Int32("session", raceSessionDone))
				r.rl.UnregisterProvider()
				if raceSessionDone == r.raceSessions[len(r.raceSessions)-1] {
					r.l.Debug("last race session done")
					r.rl.Close()
					r.overallCtx.cancel()
					return
				} else {
					// we keep the current rl until the next race session will start
					nextSessionNum := r.rl.WaitForNextRaceSession(raceSessionDone)
					r.rl.Close()
					r.rl = r.createRacelogger()
					r.l.Info("waiting before registering next session",
						log.Int32("next", nextSessionNum))
					time.Sleep(2 * time.Second)
					r.l.Debug("about to register heat session",
						log.String("name", r.rl.GetSessionName(nextSessionNum)))
					raceIndex++ // increment our own race index
					name, descr := computeNameAndDescription(
						r.eventNames, r.eventDescriptions, raceIndex)
					if regErr := r.rl.RegisterProviderHeat(
						name,
						descr,
						r.rl.GetSessionName(nextSessionNum)); regErr == nil {
						r.rl.StartRecording()
					} else {
						r.l.Error("Error registering heat session", log.ErrorField(regErr))
					}
				}
			}
		}
	}
	go recorderLoop()

	name, descr := computeNameAndDescription(
		r.eventNames, r.eventDescriptions, raceIndex)
	if len(r.raceSessions) == 1 {
		// we only have one race session. standard procedure
		r.rl = r.createRacelogger()
		if regErr := r.rl.RegisterProvider(
			name,
			descr); regErr == nil {
			r.rl.StartRecording()
		} else {
			r.l.Error("Error registering session", log.ErrorField(regErr))
		}
	} else {
		r.rl = r.createRacelogger()
		if regErr := r.rl.RegisterProviderHeat(
			name,
			descr,
			r.rl.GetSessionName(r.currentSession)); regErr == nil {
			r.rl.StartRecording()
		} else {
			r.l.Error("Error registering heat session", log.ErrorField(regErr))
		}
	}
}

func (r *Recorder) Stop() {
	r.l.Debug("Stop recording requested. Unregistering provider")
	r.rl.UnregisterProvider()
}

func (r *Recorder) Close() {
	// cleanup
}

func (r *Recorder) collectRaceSessions() {
	check := racelogger.NewRaceLogger(
		racelogger.WithContext(r.overallCtx.ctx, r.overallCtx.cancel),
		racelogger.WithWaitForServicesTimeout(r.waitForServicesTimeout),
	)

	sessions, cur, _ := check.GetRaceSessions()
	r.l.Debug("Race sessions", log.Any("sessions", sessions), log.Int32("current", cur))
	r.raceSessions = make([]int32, len(sessions))
	for i, s := range sessions {
		r.raceSessions[i] = int32(s)
	}
	r.currentSession = cur
	check.Close()
}

func (r *Recorder) createRacelogger() *racelogger.Racelogger {
	loggerCtx, cancel := context.WithCancel(r.overallCtx.ctx)
	rl := racelogger.NewRaceLogger(
		racelogger.WithGrpcConn(r.conn),
		racelogger.WithContext(loggerCtx, cancel),
		racelogger.WithWaitForServicesTimeout(r.waitForServicesTimeout),
		racelogger.WithWaitForDataTimeout(r.waitForData),
		racelogger.WithSpeedmapPublishInterval(r.speedmapPublishInterval),
		racelogger.WithSpeedmapSpeedThreshold(r.cli.SpeedmapSpeedThreshold),
		racelogger.WithMaxSpeed(r.cli.MaxSpeed),
		racelogger.WithRecordingMode(r.recordingMode),
		racelogger.WithToken(r.cli.Token),
		racelogger.WithGrpcLogFile(r.cli.MsgLogFile),
		racelogger.WithEnsureLiveData(r.cli.EnsureLiveData),
		racelogger.WithEnsureLiveDataInterval(r.ensureLiveDataInterval),
		racelogger.WithWatchdogInterval(r.watchdogInterval),
		racelogger.WithRaceSessionRecorded(r.raceSessionRecordedChan),
		racelogger.WithUUIDEventKey(),
	)
	if rl == nil {
		log.Error("Could not create racelogger")
		return nil
	}
	return rl
}
