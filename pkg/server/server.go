package server

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"time"

	"buf.build/gen/go/mpapenbr/iracelog/connectrpc/go/racelogger/v1/raceloggerv1connect"
	"buf.build/gen/go/mpapenbr/iracelog/grpc/go/iracelog/provider/v1/providerv1grpc"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	v1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/racelogger/v1"
	"connectrpc.com/grpcreflect"
	"github.com/mpapenbr/goirsdk/irsdk"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/util"
	"github.com/mpapenbr/go-racelogger/version"
)

type (
	Server interface {
		Start() error
		Close() error
	}
	serverImpl struct {
		cfg         *serverConfig
		ctx         context.Context
		l           *log.Logger
		status      myStatus
		recCtx      *recordingContext
		broadcaster *Broadcaster[myStatus]
	}
	raceSession struct {
		Num  uint32
		Name string
	}
	myStatus struct {
		BackendAvailable   bool
		BackendCompatible  bool
		ValidCredentials   bool
		SimulationRunning  bool
		TelemetryAvailable bool
		Recording          bool
		CurrentSessionNum  int32
		RaceSessions       []raceSession
	}

	serverConfig struct {
		ctx                  context.Context
		logger               *log.Logger
		conn                 *grpc.ClientConn
		addr                 string
		backendCheckInterval time.Duration
	}
	Option interface {
		apply(*serverConfig) *serverConfig
	}
	optFunc func(*serverConfig) *serverConfig
)

var _ Server = (*serverImpl)(nil)

func (f optFunc) apply(cfg *serverConfig) *serverConfig {
	return f(cfg)
}

func WithContext(ctx context.Context) Option {
	return optFunc(func(cfg *serverConfig) *serverConfig {
		cfg.ctx = ctx
		return cfg
	})
}

func WithLogger(logger *log.Logger) Option {
	return optFunc(func(cfg *serverConfig) *serverConfig {
		cfg.logger = logger
		return cfg
	})
}

func WithGrpcConn(conn *grpc.ClientConn) Option {
	return optFunc(func(cfg *serverConfig) *serverConfig {
		cfg.conn = conn
		return cfg
	})
}

func WithAddr(addr string) Option {
	return optFunc(func(cfg *serverConfig) *serverConfig {
		cfg.addr = addr
		return cfg
	})
}

func WithBackendCheckInterval(interval time.Duration) Option {
	return optFunc(func(cfg *serverConfig) *serverConfig {
		cfg.backendCheckInterval = interval
		return cfg
	})
}

func NewServer(opts ...Option) (Server, error) {
	cfg := newServerConfig(opts)

	srv := &serverImpl{
		cfg:         cfg,
		ctx:         cfg.ctx,
		status:      myStatus{},
		broadcaster: NewBroadcaster[myStatus](),
	}
	if cfg.logger != nil {
		srv.l = cfg.logger
	} else {
		if log.GetFromContext(cfg.ctx) != nil {
			srv.l = log.GetFromContext(cfg.ctx)
		} else {
			srv.l = log.Default().Named("server")
		}
	}
	return srv, nil
}

func newServerConfig(opts []Option) *serverConfig {
	c := &serverConfig{
		ctx:  context.Background(),
		addr: "localhost:8135", // Default address
	}
	for _, opt := range opts {
		c = opt.apply(c)
	}
	return c
}

func (s *serverImpl) Start() error {
	s.l.Debug("Starting server", log.String("address", s.cfg.addr))
	go s.checkIRacing()
	go s.checkBackend()
	go s.showStatusUpdate()
	go s.startConnectRPCServer()

	return nil
}

func (s *serverImpl) Close() error {
	s.l.Debug("Closing server")
	return nil
}

func (s *serverImpl) startConnectRPCServer() {
	s.l.Debug("Starting connect-RPC server")
	mux := http.NewServeMux()
	path, handler := raceloggerv1connect.NewRaceloggerServiceHandler(
		NewRaceloggerServiceConnectRPC(s))
	mux.Handle(path, handler)

	// Configure CORS (otherwise browser will not allow requests)
	corsHandler := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			values := []string{
				"Content-Type",
				"Connect-Protocol-Version",
				"Connect-Accept-Encoding",
				"Connect-Content-Encoding",
				"X-Connect-Timeout-Ms",
			}
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(values, ", "))
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h.ServeHTTP(w, r)
		})
	}

	// just to ease checks via grpcurl or similar tools
	reflector := grpcreflect.NewStaticReflector(
		raceloggerv1connect.RaceloggerServiceName)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	server := &http.Server{
		Addr:              s.cfg.addr,
		Handler:           corsHandler(h2c.NewHandler(mux, &http2.Server{})),
		ReadHeaderTimeout: 10 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Error("error starting server", log.ErrorField(err))
		return
	}
}

func (s *serverImpl) showStatusUpdate() {
	s.l.Debug("Starting status update collector")
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastStatus := s.status
	for {
		select {
		case <-ticker.C:
			if !statusEqual(s.status, lastStatus) {
				s.l.Info("Status update",
					log.Any("status", s.status),
				)
				lastStatus = s.status
				s.broadcaster.Broadcast(s.status)
			}
		case <-s.ctx.Done():
			s.l.Debug("Stopping status update collector")
			return
		}
	}
}

// statusEqual compares two myStatus structs for equality.
func statusEqual(a, b myStatus) bool {
	return reflect.DeepEqual(a, b)
}

func (s *serverImpl) GetStatus() *myStatus {
	return &s.status
}

func (s *serverImpl) SubscribeStatus() <-chan myStatus {
	return s.broadcaster.Subscribe()
}

func (s *serverImpl) UnsubscribeStatus(c <-chan myStatus) {
	s.broadcaster.Unsubscribe(c)
}

func (s *serverImpl) StartRecording(msg *v1.StartRecordingRequest) *myStatus {
	rc := newRecordingContext(s.ctx, s.cfg.conn, func() {
		s.l.Debug("Callback recordingDone called. Marking recording as stopped")
		s.status.Recording = false
		s.recCtx = nil
	})
	rc.startRecording(msg)
	s.status.Recording = true
	s.recCtx = rc
	s.l.Debug("Recording started")
	return &s.status
}

func (s *serverImpl) StopRecording() *myStatus {
	s.status.Recording = false
	if s.recCtx != nil {
		s.recCtx.stopRecording()
		s.recCtx = nil
	}
	s.l.Debug("Recording stopped")
	return &s.status
}

//nolint:gocognit,nestif // ok here
func (s *serverImpl) checkIRacing() {
	ticker := time.NewTicker(1 * time.Second)
	var ir *irsdk.Irsdk
	for {
		select {
		case <-s.ctx.Done():
			log.Debug("left checkIRacing loop")
			ticker.Stop()
			return
		case <-ticker.C:
			s.status.SimulationRunning, _ = irsdk.IsSimRunning(s.ctx, http.DefaultClient)
			if s.status.SimulationRunning {
				if ir == nil {
					ir = irsdk.NewIrsdk()
				}
				s.status.TelemetryAvailable = util.HasValidAPIData(ir)
				if s.status.TelemetryAvailable {
					s.collectIracingData(ir)
				} else {
					ir.Close()
					ir = nil
					s.resetIracingData()
				}
			} else {
				if ir != nil {
					ir.Close()
					ir = nil
				}
				s.status.TelemetryAvailable = false
				s.resetIracingData()
			}
		}
	}
}

func (s *serverImpl) resetIracingData() {
	s.status.CurrentSessionNum = -1
	s.status.RaceSessions = make([]raceSession, 0)
	if s.status.Recording {
		s.l.Debug("Stopping recording due to iRacing telemetry not available")
		s.StopRecording()
	}
}

func (s *serverImpl) collectIracingData(ir *irsdk.Irsdk) {
	irYaml, err := ir.GetYaml()
	if err != nil {
		s.l.Error("Could not get iRacing YAML data", log.ErrorField(err))
		return
	}
	raceSessions := make([]raceSession, 0)
	for i := range irYaml.SessionInfo.Sessions {
		s := irYaml.SessionInfo.Sessions[i]
		if s.SessionType == "Race" {
			raceSessions = append(raceSessions, raceSession{
				Num:  uint32(s.SessionNum),
				Name: s.SessionName,
			})
		}
	}
	s.status.CurrentSessionNum, _ = ir.GetIntValue("SessionNum")
	s.status.RaceSessions = raceSessions
}

func (s *serverImpl) checkBackend() {
	ticker := time.NewTicker(s.cfg.backendCheckInterval)
	c := providerv1grpc.NewProviderServiceClient(s.cfg.conn)
	for {
		select {
		case <-s.ctx.Done():
			log.Debug("left checkBackend loop")
			ticker.Stop()
			return
		case <-ticker.C:
			var err error
			var res *providerv1.VersionCheckResponse
			md := metadata.Pairs("api-token", config.DefaultCliArgs().Token)
			ctx := metadata.NewOutgoingContext(s.ctx, md)
			if res, err = c.VersionCheck(ctx,
				&providerv1.VersionCheckRequest{
					RaceloggerVersion: version.Version,
				}); err != nil {
				s.status.BackendAvailable = false
				s.status.BackendCompatible = false
				s.status.ValidCredentials = false
			} else {
				s.status.BackendAvailable = true
				s.status.BackendCompatible = res.RaceloggerCompatible
				s.status.ValidCredentials = res.ValidCredentials
			}
		}
	}
}
