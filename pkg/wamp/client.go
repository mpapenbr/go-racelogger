package wamp

import (
	"context"
	"errors"
	"log"

	"github.com/gammazero/nexus/v3/client"
)

var ErrNoResults = errors.New("no results")

func GetClientWithConfigNew(url string, cfg *client.Config) *client.Client {

	// Connect wampClient session.
	wampClient, err := client.ConnectNet(context.Background(), url, *cfg)
	if err != nil {
		log.Fatal(err)
	}

	return wampClient
}
