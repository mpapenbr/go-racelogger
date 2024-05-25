package util

import (
	"crypto/tls"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/mpapenbr/go-racelogger/pkg/config"
)

func ConnectGrpc(cfg *config.CliArgs) (*grpc.ClientConn, error) {
	if cfg.Insecure {
		return grpc.NewClient(cfg.Addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS13, // Set the minimum TLS version to TLS 1.3
		}
		return grpc.NewClient(cfg.Addr,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	}
}
