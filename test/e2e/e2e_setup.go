package main

import (
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	"github.com/onsi/ginkgo/v2"
)

var _ = ginkgo.AfterSuite(func() {
	LogExecutionSummary()
})
