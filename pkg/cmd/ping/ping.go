package ping

import (
	"context"
	"time"

	"buf.build/gen/go/mpapenbr/iracelog/grpc/go/iracelog/provider/v1/providerv1grpc"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/util"
)

var (
	numPings     int
	delayArg     string
	ignoreErrors bool
)

//nolint:lll // readability
func NewPingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "check connection to backend server",
		Run: func(cmd *cobra.Command, args []string) {
			pingBackend()
		},
	}
	cmd.Flags().IntVarP(&numPings, "num", "n", 10, "number of pings to send")
	cmd.Flags().StringVarP(&delayArg, "delay", "d", "1s", "time to wait between pings")
	cmd.Flags().BoolVarP(&ignoreErrors, "ignore", "i", false, "Ignore errors and continue pinging")
	return cmd
}

func pingBackend() {
	log.Debug("Starting...")

	var conn *grpc.ClientConn
	var err error
	var delay time.Duration

	if conn, err = util.ConnectGrpc(config.DefaultCliArgs()); err != nil {
		log.Error("Could not connect to gRPC server", log.ErrorField(err))
		return
	}
	delay, err = time.ParseDuration(delayArg)
	if err != nil {
		delay = time.Second
	}
	c := providerv1grpc.NewProviderServiceClient(conn)
	for i := 1; i < numPings; i++ {
		log.Debug("pinging server", log.Int("iteration", i))
		req := providerv1.PingRequest{Num: int32(i)}
		r, err := c.Ping(context.Background(), &req)
		if err != nil {
			log.Error("error pinging server", log.Int("i", i), log.ErrorField(err))
			if !ignoreErrors {
				return
			}
		} else {
			log.Info("Response",
				log.Int32("num", r.Num),
				log.String("time-utc", r.Timestamp.AsTime().Format(time.RFC3339)))
		}

		time.Sleep(delay)
	}
}
