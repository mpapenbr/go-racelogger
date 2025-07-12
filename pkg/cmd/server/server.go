package server

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/server"
	"github.com/mpapenbr/go-racelogger/pkg/util"
)

func NewServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "run racelogger in server mode",
		Run: func(cmd *cobra.Command, args []string) {
			runServer(cmd.Context())
		},
	}
	cmd.PersistentFlags().StringVar(&config.DefaultCliArgs().ServerServiceAddr,
		"service-addr", "localhost:8135", "gRPC listen address for the frontend service")
	cmd.Flags().StringVarP(&config.DefaultCliArgs().Token,
		"token",
		"t",
		"",
		"Dataprovider token")
	cmd.Flags().DurationVar(&config.DefaultCliArgs().BackendCheckInterval,
		"backend-check-interval",
		time.Second*2,
		"Interval to check backend compatibility")
	return cmd
}

//nolint:funlen // ok here
func runServer(cmdCtx context.Context) {
	var conn *grpc.ClientConn
	var err error
	if conn, err = util.ConnectGrpc(config.DefaultCliArgs()); err != nil {
		log.Error("Could not create grpc.ClientConn", log.ErrorField(err))
		return
	}
	defer conn.Close()

	myCtx, cancel := context.WithCancel(cmdCtx)
	defer cancel()
	var srv server.Server

	srv, err = server.NewServer(
		server.WithContext(myCtx),
		server.WithGrpcConn(conn),
		server.WithAddr(config.DefaultCliArgs().ServerServiceAddr),
		server.WithBackendCheckInterval(config.DefaultCliArgs().BackendCheckInterval),
		server.WithLogger(log.GetFromContext(myCtx).Named("server")))
	if err != nil {
		log.Error("Could not create server", log.ErrorField(err))
		return
	}
	defer srv.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer func() {
		signal.Stop(sigChan)
		cancel()
	}()

	if srv.Start() != nil {
		log.Error("Could not start server", log.ErrorField(err))
		return
	}

	log.Debug("Waiting for termination")
	select {
	case <-sigChan:
		log.Debug("interrupt signaled. Terminating")
		cancel()

	case <-myCtx.Done():
		log.Debug("Received ctx.Done")
	}
	log.Info("Server finished")
}
