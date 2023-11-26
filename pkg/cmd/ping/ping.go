package ping

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/util"
	"github.com/mpapenbr/go-racelogger/pkg/wamp"
)

func NewPingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "check connection to backend server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return pingBackend()
		},
	}

	return cmd
}

func pingBackend() error {
	pc := wamp.NewPublicClient(config.URL, config.Realm)
	defer pc.Close()
	version, err := pc.GetVersion()
	if err != nil {
		log.Error("Could not get remote version", log.ErrorField(err))
	}
	versionOk := util.CheckServerVersion(version)
	fmt.Printf("Server responds with version: %s \n", version)
	fmt.Printf("Compatible: %t\n", versionOk)

	return nil
}
