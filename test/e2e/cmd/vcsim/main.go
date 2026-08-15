// Custom vcsim wrapper that creates a stock VPX model and patches datastores
// with StorageIORMInfo so the forklift collector produces valid congestionThreshold
// values (the stock vcsim omits iormConfiguration entirely, resulting in 0).
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"

	// Register the optional SOAP/REST endpoints via their init() functions, the
	// same way the stock vmware/vcsim binary does. Without these blank imports
	// the base simulator serves only /sdk, so the vAPI REST endpoint is absent
	// and the forklift collector's REST login gets a 404 and retries
	// forever, leaving collection stuck in "collecting".
	_ "github.com/vmware/govmomi/cns/simulator"
	_ "github.com/vmware/govmomi/eam/simulator"
	_ "github.com/vmware/govmomi/lookup/simulator"
	_ "github.com/vmware/govmomi/pbm/simulator"
	_ "github.com/vmware/govmomi/sts/simulator"
	_ "github.com/vmware/govmomi/vapi/simulator"
	_ "github.com/vmware/govmomi/vsan/simulator"
	_ "github.com/vmware/govmomi/vslm/simulator"
)

const (
	defaultListen   = ":8989"
	defaultUsername = "user"
	defaultPassword = "pass"

	// iormCongestionThreshold must satisfy the migration-planner OpenAPI schema
	// (StorageIoConfiguration.congestionThreshold has minimum 5). The stock
	// vcsim omits iormConfiguration entirely, which the forklift collector
	// reports as 0 and the server rejects with HTTP 400.
	iormCongestionThreshold = 30
)

func main() {
	listen := flag.String("l", defaultListen, "listen address")
	username := flag.String("username", defaultUsername, "username")
	password := flag.String("password", defaultPassword, "password")
	flag.Parse()

	model, err := newConfiguredModel(*listen, *username, *password)
	if err != nil {
		log.Fatalf("configuring vcsim model: %v", err)
	}
	defer model.Remove()

	server := model.Service.NewServer()
	defer server.Close()

	fmt.Fprintf(os.Stderr, "vcsim running at %s\n", server.URL)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

// newConfiguredModel builds a stock VPX model and patches it so it is usable by
// the forklift collector against the migration-planner service: every datastore
// gets a valid StorageIORMInfo, and the service listens on `listen` with
// basic-auth credentials over TLS with all optional endpoints registered. The
// caller owns model.Remove() and starting the server.
func newConfiguredModel(listen, username, password string) (*simulator.Model, error) {
	model := simulator.VPX()
	if err := model.Create(); err != nil {
		return nil, fmt.Errorf("creating model: %w", err)
	}

	configureDatastoreIORM(model)

	model.Service.Listen = &url.URL{
		Host: listen,
		User: url.UserPassword(username, password),
	}
	model.Service.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	model.Service.RegisterEndpoints = true

	return model, nil
}

// configureDatastoreIORM patches every datastore in the model with a valid
// StorageIORMInfo so the collector reports a congestionThreshold that passes
// the server's schema validation.
func configureDatastoreIORM(model *simulator.Model) {
	for _, entity := range model.Map().All("Datastore") {
		ds, ok := entity.(*simulator.Datastore)
		if !ok {
			continue
		}
		ds.IormConfiguration = &types.StorageIORMInfo{
			Enabled:                  false,
			CongestionThreshold:      iormCongestionThreshold,
			CongestionThresholdMode:  "automatic",
			StatsCollectionEnabled:   types.NewBool(false),
			ReservationEnabled:       types.NewBool(false),
			StatsAggregationDisabled: types.NewBool(true),
			PercentOfPeakThroughput:  90,
		}
	}
}
