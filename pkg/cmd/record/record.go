package record

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/mpapenbr/go-racelogger/internal"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/util"
	"github.com/mpapenbr/go-racelogger/pkg/wamp"
	"github.com/spf13/cobra"
)

func NewRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "record an iRacing event",
		RunE: func(cmd *cobra.Command, args []string) error {
			return recordEvent()
		},
	}

	cmd.Flags().StringVar(&config.WaitForServices,
		"wait",
		"60s",
		"Wait for running iRacing Sim")
	cmd.Flags().StringVarP(&config.Password,
		"password",
		"p",
		"",
		"Dataprovider password for backend")
	cmd.Flags().StringVar(&config.LogLevel,
		"logLevel",
		"debug",
		"controls the log level (debug, info, warn, error, fatal)")

	cmd.Flags().StringVar(&config.LogFormat,
		"logFormat",
		"json",
		"controls the log output format")
	return cmd
}

func recordEvent() error {
	if logger := util.SetupLogger(); logger == nil {
		fmt.Printf("Could not setup logger. Strange")
	}

	log.Debug("Starting...")
	if ok, err := validateBackendVersion(); err != nil || !ok {
		return nil
	}
	if ok := util.WaitForSimulation(); !ok {
		log.Error("Simulation not running")
		return nil
	}

	// stdLogger, err := zap.NewStdLogAt(logger.ZapLogger(), log.DebugLevel)
	// if err != nil {
	// 	log.Fatal("Could not create stdLogger", log.ErrorField(err))
	// }
	// stdLogger.Printf("something\n")

	ctx, cancel := context.WithCancel(context.Background())
	r := internal.NewRaceLogger(ctx, cancel, "test")
	defer r.Close()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer func() {
		signal.Stop(sigChan)
		cancel()
	}()

	log.Debug("Waiting for termination")
	select {
	case <-sigChan:
		{
			log.Debug("interrupt signaled. Terminating")
			cancel()
			log.Debug("Waiting some seconds")
			time.Sleep(time.Second * 3)
		}
	case <-ctx.Done():
		{
			log.Debug("Recieved ctx.Done")
		}
	}

	// log.Debug("Got signal ", log.Any("signal", v))
	// wampHandler.shutdown()
	log.Info("Server terminated")
	return nil
}

func validateBackendVersion() (bool, error) {
	pc := wamp.NewPublicClient(config.URL, config.Realm)
	defer pc.Close()
	version, err := pc.GetVersion()
	if err != nil {
		log.Error("Could not get remote version", log.ErrorField(err))
		return false, err
	}
	versionOk := util.CheckServerVersion(version)
	if !versionOk {
		log.Error("Backend version not compatible.", log.String("required", util.RequiredServerVersion), log.String("backend", version))
	}
	return versionOk, nil
}
