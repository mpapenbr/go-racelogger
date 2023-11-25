package status

import (
	"errors"
	"fmt"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/util"

	"github.com/mpapenbr/goirsdk/irsdk"

	"github.com/spf13/cobra"
)

var (
	ErrSimulationNotRunning = errors.New("iRacing Simulation not running")
	ErrVarDataRetrieval     = errors.New("could not get variable data from iRacing")
)

func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "check iracing status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkIracingStatus()
		},
	}

	cmd.Flags().StringVar(&config.WaitForServices,
		"wait",
		"60s",
		"Wait for running iRacing Sim")

	return cmd
}

func checkIracingStatus() error {
	if util.WaitForSimulation() {
		log.Error(ErrSimulationNotRunning.Error())
		return nil
	}

	api := irsdk.NewIrsdk()
	defer api.Close()

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
