package service

import (
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/sirupsen/logrus"
)

type ServiceHandler struct {
	store store.Store
	log   logrus.FieldLogger
}

// Make sure we conform to servers Service interface
var _ server.Service = (*ServiceHandler)(nil)

func NewServiceHandler(store store.Store, log logrus.FieldLogger) *ServiceHandler {
	return &ServiceHandler{
		store: store,
		log:   log,
	}
}
