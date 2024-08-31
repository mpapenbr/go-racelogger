package check

import (
	"context"
	"fmt"

	"buf.build/gen/go/mpapenbr/iracelog/grpc/go/iracelog/provider/v1/providerv1grpc"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
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
			checkCompatibility(cmd.Context())
		},
	}

	return cmd
}

func checkCompatibility(ctx context.Context) {
	logger := log.GetFromContext(ctx)

	logger.Debug("Starting...")

	var conn *grpc.ClientConn
	var err error

	if conn, err = util.ConnectGrpc(config.DefaultCliArgs()); err != nil {
		logger.Error("Could not connect to gRPC server", log.ErrorField(err))
		return
	}

	c := providerv1grpc.NewProviderServiceClient(conn)
	if res, err := c.VersionCheck(context.Background(), &providerv1.VersionCheckRequest{
		RaceloggerVersion: version.Version,
	}); err != nil {
		logger.Error("error checking compatibility", log.ErrorField(err))
	} else {
		logger.Debug("Compatibility check successful",
			log.String("this-racelogger-version", version.Version),
			log.String("server-version", res.ServerVersion),
			log.String("minimum-racelogger-version", res.SupportedRaceloggerVersion),
			log.Bool("compatible", res.RaceloggerCompatible))
		fmt.Printf(`
Racelogger version  : v%s
Server version      : v%s
Minimum racelogger  : %s
Compatible          : %t`,
			version.Version,
			res.ServerVersion,
			res.SupportedRaceloggerVersion,
			res.RaceloggerCompatible)
	}
}
