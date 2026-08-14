package main

import (
	"testing"

	"github.com/vmware/govmomi/simulator"
)

func TestConfigureDatastoreIORM(t *testing.T) {
	model := simulator.VPX()
	if err := model.Create(); err != nil {
		t.Fatalf("creating model: %v", err)
	}
	defer model.Remove()

	configureDatastoreIORM(model)

	datastores := model.Map().All("Datastore")
	if len(datastores) == 0 {
		t.Fatal("expected at least one datastore in the VPX model")
	}

	for _, entity := range datastores {
		ds, ok := entity.(*simulator.Datastore)
		if !ok {
			continue
		}
		if ds.IormConfiguration == nil {
			t.Fatalf("datastore %s has no IORM configuration", ds.Name)
		}
		if got := ds.IormConfiguration.CongestionThreshold; got != iormCongestionThreshold {
			t.Errorf("datastore %s congestionThreshold = %d, want %d", ds.Name, got, iormCongestionThreshold)
		}
	}
}

func TestNewConfiguredModelEndpoint(t *testing.T) {
	model, err := newConfiguredModel(":9999", "alice", "s3cret")
	if err != nil {
		t.Fatalf("newConfiguredModel: %v", err)
	}
	defer model.Remove()

	if got := model.Service.Listen.Host; got != ":9999" {
		t.Errorf("listen host = %q, want %q", got, ":9999")
	}
	if got := model.Service.Listen.User.Username(); got != "alice" {
		t.Errorf("username = %q, want %q", got, "alice")
	}
	if got, _ := model.Service.Listen.User.Password(); got != "s3cret" {
		t.Errorf("password = %q, want %q", got, "s3cret")
	}
	if !model.Service.RegisterEndpoints {
		t.Error("expected RegisterEndpoints to be true")
	}
	if model.Service.TLS == nil {
		t.Error("expected TLS to be configured")
	}
}
