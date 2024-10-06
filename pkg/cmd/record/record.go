package record

import (
	"context"
	"os"
	"os/signal"
	"time"

	"buf.build/gen/go/mpapenbr/iracelog/grpc/go/iracelog/provider/v1/providerv1grpc"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/mpapenbr/go-racelogger/internal"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/util"
	"github.com/mpapenbr/go-racelogger/version"
)

var (
	eventName        string
	eventDescription string
)

//nolint:funlen // ok here
func NewRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "record an iRacing event",
		RunE: func(cmd *cobra.Command, args []string) error {
			return recordEvent(cmd.Context(), config.DefaultCliArgs())
		},
	}

	cmd.Flags().StringVarP(&eventName,
		"name",
		"n",
		"",
		"Event name")
	cmd.Flags().StringVarP(&eventDescription,
		"description",
		"d",
		"",
		"Event description")

	cmd.Flags().StringVar(&config.DefaultCliArgs().WaitForServices,
		"wait",
		"60s",
		"Wait for running iRacing Sim")
	cmd.Flags().StringVar(&config.DefaultCliArgs().WaitForData,
		"wait-for-data",
		"1s",
		"Timeout to wait for irsdk to signal valid data")

	cmd.Flags().StringVarP(&config.DefaultCliArgs().Token,
		"token",
		"t",
		"",
		"Dataprovider token")

	cmd.Flags().StringVar(&config.DefaultCliArgs().SpeedmapPublishInterval,
		"speedmap-publish-interval",
		"30s",
		"publish speedmap data to server using this interval")
	cmd.Flags().Float64Var(&config.DefaultCliArgs().SpeedmapSpeedThreshold,
		"speedmap-speed-threshold",
		0.5,
		"do not record speeds below this threshold pct (0-1.0) to the avg speed of the chunk")
	cmd.Flags().Float64Var(&config.DefaultCliArgs().MaxSpeed,
		"max-speed",
		500,
		"do not process computed speed above this value in km/h")
	cmd.Flags().BoolVar(&config.DefaultCliArgs().DoNotPersist,
		"do-not-persist",
		false,
		"do not persist the recorded data (used for debugging)")
	cmd.Flags().StringVar(&config.DefaultCliArgs().MsgLogFile,
		"msg-log-file",
		"",
		"write grpc messages to this file")
	cmd.Flags().BoolVar(&config.DefaultCliArgs().EnsureLiveData,
		"ensure-live-data",
		true,
		"set replay to live data on connection")
	cmd.Flags().StringVar(&config.DefaultCliArgs().EnsureLiveDataInterval,
		"ensure-live-data-interval",
		"0s",
		"set replay to live mode with this interval (if > 0s)")
	cmd.Flags().StringVar(&config.DefaultCliArgs().WatchdogInterval,
		"watchdog-interval",
		"5s",
		"how often should we issue the watchdog checks (0s == disabled)")
	return cmd
}

//nolint:funlen,gocritic,cyclop // by design
func recordEvent(cmdCtx context.Context, cfg *config.CliArgs) error {
	log.Debug("Starting...")
	log.Debug("Config", log.Any("cfg", cfg))
	if ok := util.WaitForSimulation(cfg); !ok {
		log.Error("Simulation not running")
		return nil
	}

	var conn *grpc.ClientConn
	var err error
	if conn, err = util.ConnectGrpc(cfg); err != nil {
		log.Error("Could not connect to gRPC server", log.ErrorField(err))
		return nil
	}

	if ok := validateBackendVersion(conn); !ok {
		return nil
	}

	var waitForData,
		waitForServicesTimeout,
		speedmapPublishInterval,
		ensureLiveDataInterval,
		watchdogInterval time.Duration

	waitForData, err = time.ParseDuration(cfg.WaitForData)
	if err != nil {
		waitForData = time.Second
	}
	waitForServicesTimeout, err = time.ParseDuration(cfg.WaitForServices)
	if err != nil {
		waitForServicesTimeout = time.Minute
	}
	speedmapPublishInterval, err = time.ParseDuration(cfg.SpeedmapPublishInterval)
	if err != nil {
		speedmapPublishInterval = 30 * time.Second
	}
	ensureLiveDataInterval, err = time.ParseDuration(cfg.EnsureLiveDataInterval)
	if err != nil {
		ensureLiveDataInterval = 0
	}
	watchdogInterval, err = time.ParseDuration(cfg.WatchdogInterval)
	if err != nil {
		watchdogInterval = 5 * time.Second
	}

	recordingMode := providerv1.RecordingMode_RECORDING_MODE_PERSIST
	if cfg.DoNotPersist {
		recordingMode = providerv1.RecordingMode_RECORDING_MODE_DO_NOT_PERSIST
	}
	ctx, cancel := context.WithCancel(cmdCtx)
	r := internal.NewRaceLogger(
		internal.WithGrpcConn(conn),
		internal.WithContext(ctx, cancel),
		internal.WithWaitForServicesTimeout(waitForServicesTimeout),
		internal.WithWaitForDataTimeout(waitForData),
		internal.WithSpeedmapPublishInterval(speedmapPublishInterval),
		internal.WithSpeedmapSpeedThreshold(cfg.SpeedmapSpeedThreshold),
		internal.WithMaxSpeed(cfg.MaxSpeed),
		internal.WithRecordingMode(recordingMode),
		internal.WithToken(cfg.Token),
		internal.WithGrpcLogFile(cfg.MsgLogFile),
		internal.WithEnsureLiveData(cfg.EnsureLiveData),
		internal.WithEnsureLiveDataInterval(ensureLiveDataInterval),
		internal.WithWatchdogInterval(watchdogInterval),
	)
	if r == nil {
		log.Error("Could not create racelogger")
		return nil
	}
	defer r.Close()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer func() {
		signal.Stop(sigChan)
		cancel()
	}()

	log.Debug("Register event")

	if err := r.RegisterProvider(eventName, eventDescription); err != nil {
		return err
	}

	log.Debug("Waiting for termination")
	select {
	case <-sigChan:
		{
			log.Debug("interrupt signaled. Terminating")
			cancel()
			// log.Debug("Waiting some seconds")
			// time.Sleep(time.Second * 2)
		}
	case <-ctx.Done():
		{
			log.Debug("Received ctx.Done")
		}
	}

	log.Debug("Unregister event")
	r.UnregisterProvider()
	// log.Debug("Got signal ", log.Any("signal", v))
	// wampHandler.shutdown()
	log.Info("Server terminated")
	return nil
}

func validateBackendVersion(conn *grpc.ClientConn) bool {
	c := providerv1grpc.NewProviderServiceClient(conn)
	var res *providerv1.VersionCheckResponse
	var err error
	if res, err = c.VersionCheck(context.Background(), &providerv1.VersionCheckRequest{
		RaceloggerVersion: version.Version,
	}); err != nil {
		log.Error("error checking compatibility", log.ErrorField(err))
		return false
	}
	if !res.RaceloggerCompatible {
		log.Error("Client and server are not compatible",
			log.String("this-racelogger-version", version.Version),
			log.String("server-version", res.ServerVersion),
			log.String("minimum-racelogger-version", res.SupportedRaceloggerVersion),
			log.Bool("compatible", res.RaceloggerCompatible))
	}
	return res.RaceloggerCompatible
}
