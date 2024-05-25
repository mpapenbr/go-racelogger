package check

import (
	"context"

	"buf.build/gen/go/mpapenbr/testrepo/grpc/go/testrepo/provider/v1/providerv1grpc"
	providerv1 "buf.build/gen/go/mpapenbr/testrepo/protocolbuffers/go/testrepo/provider/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/util"
	"github.com/mpapenbr/go-racelogger/version"
)

func NewVersionCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "check if racelogger is compatible with the backend server",
		Run: func(cmd *cobra.Command, args []string) {
			checkCompatibility()
		},
	}

	return cmd
}

func checkCompatibility() {
	log.Debug("Starting...")

	var conn *grpc.ClientConn
	var err error

	if conn, err = util.ConnectGrpc(config.DefaultCliArgs()); err != nil {
		log.Error("Could not connect to gRPC server", log.ErrorField(err))
		return
	}

	c := providerv1grpc.NewProviderServiceClient(conn)
	if res, err := c.VersionCheck(context.Background(), &providerv1.VersionCheckRequest{
		RaceloggerVersion: version.Version,
	}); err != nil {
		log.Error("error checking compatibility", log.ErrorField(err))
	} else {
		log.Info("Compatibility check successful",
			log.String("this-racelogger-version", version.Version),
			log.String("server-version", res.ServerVersion),
			log.String("minimum-racelogger-version", res.SupportedRaceloggerVersion),
			log.Bool("compatible", res.RaceloggerCompatible))
	}
}
