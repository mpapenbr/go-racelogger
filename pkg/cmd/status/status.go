package status

import (
	"errors"
	"fmt"
	"time"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"

	// irsdk "github.com/mpapenbr/goirsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"

	"github.com/spf13/cobra"
)

func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "check iracing status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkIracingStatus()
		},
	}

	cmd.PersistentFlags().StringVar(&config.WaitForServices,
		"wait",
		"60s",
		"Wait for running iRacing Sim")

	return cmd
}

var ErrSimulationNotRunning = errors.New("iRacing Simulation not running")
var ErrVarDataRetrieval = errors.New("could not get variable data from iRacing")

func checkIracingStatus() error {
	if waitForSimulation() {
		log.Error(ErrSimulationNotRunning.Error())
		return ErrSimulationNotRunning
	}

	api := irsdk.NewIrsdk()

	if !api.GetData() {
		log.Error(ErrVarDataRetrieval.Error())
		return nil
	}

	if y, err := api.GetYaml(); err == nil {
		sessionNum, _ := api.GetIntValue("SessionNum")
		sessionTime, _ := api.GetDoubleValue("SessionTime")

		log.Info("iRacing Simulation runnng",
			log.String("track", y.WeekendInfo.TrackDisplayName),
			log.String("Session", y.SessionInfo.Sessions[sessionNum].SessionName),
			log.String("SessionTime", fmt.Sprintf("%.2f", sessionTime)),
		)

	} else {
		return err
	}
	return nil
}

func waitForSimulation() bool {
	timeout, err := time.ParseDuration(config.WaitForServices)
	if err != nil {
		log.Warn("Invalid duration value. Setting default 60s", log.ErrorField(err))
		timeout = 60 * time.Second
	}

	log.Debug("Using timout ", log.String("timeout", timeout.String()))

	log.Info("Waiting for iRacing Simulation")
	deadline := time.Now().Add(timeout)
	goon := time.Now().Before(deadline)
	for goon {
		if irsdk.CheckIfSimIsRunning() {
			return true
		}
		time.Sleep(time.Second * 2)
		goon = time.Now().Before(deadline)
	}
	return false
}
