package e2e_test

import (
	"fmt"
	"testing"

	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

// AfterFailed is a function that it's called on JustAfterEach to run a
// function if the test fail. For example, retrieving logs.
func AfterFailed(body func()) {
	JustAfterEach(func() {
		if CurrentSpecReport().Failed() {
			By("Running AfterFailed function")
			body()
		}
	})
}

var _ = BeforeSuite(func() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.CallerKey = ""
	config.EncoderConfig.MessageKey = "msg"

	logger, _ := config.Build()
	if logger != nil {
		zap.ReplaceGlobals(logger)
	}
})

var _ = AfterSuite(func() {
	LogExecutionSummary()
	_, _ = RunLocalCommand(fmt.Sprintf("rm -rf %s", DefaultBasePath))
})
