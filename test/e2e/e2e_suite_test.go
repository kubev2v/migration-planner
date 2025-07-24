package e2e_test

import (
	"fmt"
	"os"
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

	// Initialize RHCOS ISO before running e2e tests
	By("Initializing RHCOS ISO...")
	isoUrl := os.Getenv("MIGRATION_PLANNER_ISO_URL")
	if isoUrl == "" {
		isoUrl = "https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-4.19.0-x86_64-live-iso.x86_64.iso"
	}

	isoSha256 := os.Getenv("MIGRATION_PLANNER_ISO_SHA256")
	if isoSha256 == "" {
		isoSha256 = "6a9cf9df708e014a2b44f372ab870f873cf2db5685f9ef4518f52caa36160c36"
	}

	initCommand := fmt.Sprintf("MIGRATION_PLANNER_ISO_URL=%s MIGRATION_PLANNER_ISO_SHA256=%s ./bin/planner-api init", isoUrl, isoSha256)
	zap.S().Infof("Running init command: %s", initCommand)

	output, err := RunLocalCommand(initCommand)
	if err != nil {
		zap.S().Errorf("Failed to initialize ISO: %v\nOutput: %s", err, output)
		Fail(fmt.Sprintf("ISO initialization failed: %v", err))
	}
	zap.S().Info("RHCOS ISO initialization completed successfully")
})

var _ = AfterSuite(func() {
	LogExecutionSummary()
	_, _ = RunLocalCommand(fmt.Sprintf("rm -rf %s", DefaultBasePath))
})
