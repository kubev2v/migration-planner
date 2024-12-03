package service

import (
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/store"
)

type ServiceHandler struct {
	store       store.Store
	eventWriter *events.EventProducer
}

// Make sure we conform to servers Service interface
var _ server.Service = (*ServiceHandler)(nil)

func NewServiceHandler(store store.Store, ew *events.EventProducer) *ServiceHandler {
	return &ServiceHandler{
		store:       store,
		eventWriter: ew,
	}
}
