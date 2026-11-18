// appliance-updater runs on the RHCOS host as core, outside the agent container.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/kubev2v/migration-planner/internal/applianceupdater"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configFile := flag.String("config", "/home/core/.migration-planner/upgrade.json", "Host upgrade configuration")
	attemptID := flag.String("attempt-id", "", "Exact attempt for an explicit recovery retry")
	flag.Parse()
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: appliance-updater [-config path] [-attempt-id id] update|recover|gate|retry-recovery")
	}
	file, err := os.Open(*configFile)
	if err != nil {
		return err
	}
	var config applianceupdater.Config
	decoder := json.NewDecoder(io.LimitReader(file, 1024*1024))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&config)
	if err == nil && decoder.Decode(new(any)) != io.EOF {
		err = fmt.Errorf("trailing host configuration")
	}
	_ = file.Close()
	if err != nil {
		return err
	}
	host, err := applianceupdater.NewHost(config)
	if err != nil {
		return err
	}
	coordinator, err := applianceupdater.New(config, host)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch flag.Arg(0) {
	case "update":
		return coordinator.Update(ctx)
	case "recover":
		return coordinator.Recover(ctx)
	case "gate":
		return coordinator.Gate(ctx)
	case "retry-recovery":
		return coordinator.RetryRecovery(ctx, *attemptID)
	default:
		return fmt.Errorf("unknown updater command")
	}
}
