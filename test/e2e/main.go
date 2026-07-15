package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"testing"

	"github.com/kubev2v/migration-planner/test/e2e/config"
	"github.com/kubev2v/migration-planner/test/e2e/infra"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"go.uber.org/zap"
)

func main() {
	flag.StringVar(&config.Cfg.Infra.APIImage, "api-image", "quay.io/redhat-user-workloads/assisted-migration-tenant/migration-planner-api:latest", "Image ref (e.g. oma-api:e2e)")
	flag.StringVar(&config.Cfg.Infra.ISOImage, "iso-image", "quay.io/redhat-user-workloads/assisted-migration-tenant/migration-planner-rhcos-iso:latest", "Image ref (e.g. oma-iso:e2e)")
	flag.StringVar(&config.Cfg.Infra.ClusterName, "cluster-name", "kind-e2e", "Kind cluster name")
	flag.StringVar(&config.Cfg.Infra.PrivateKeyPath, "private-key-path", "/etc/planner/e2e", "Path to private key directory")
	flag.BoolVar(&config.Cfg.Infra.KeepEnv, "keep-env", false, "Keep the Kind cluster after tests complete")
	flag.BoolVar(&config.Cfg.Infra.SkipSetup, "skip-setup", false, "Skip infra setup, run tests against existing cluster and deployments")
	flag.Parse()

	ip, err := detectLocalIP()
	if err != nil {
		log.Fatalf("failed to detect host IP: %v", err)
	}
	config.Cfg.Infra.HostIP = ip.String()

	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	zap.ReplaceGlobals(logger)
	defer func() { _ = logger.Sync() }()

	if err := run(); err != nil {
		log.Fatalf("Execution failed: %v", err)
	}
}

func run() error {
	im, err := infra.NewKindInfraManager(config.Cfg.Infra)
	if err != nil {
		return fmt.Errorf("failed to create infra manager: %w", err)
	}

	defer func() {
		_ = im.StopPortForwards()
		if !config.Cfg.Infra.KeepEnv {
			_ = im.DeleteCluster()
		}
	}()

	if !config.Cfg.Infra.SkipSetup {
		if err := setupInfra(im); err != nil {
			return fmt.Errorf("infra setup failed: %w", err)
		}
	}

	if err := im.SetupPortForwards(); err != nil {
		return fmt.Errorf("failed to setup port-forwards: %w", err)
	}

	gomega.RegisterFailHandler(ginkgo.Fail)
	if !ginkgo.RunSpecs(&testing.T{}, "E2E Suite") {
		return fmt.Errorf("tests failed")
	}
	return nil
}

func setupInfra(im infra.InfraManager) error {
	cfg := config.Cfg.Infra

	if err := im.CreateCluster(); err != nil {
		return fmt.Errorf("creating cluster: %w", err)
	}

	apiRepo, apiTag := splitImageRef(cfg.APIImage)
	isoRepo, isoTag := splitImageRef(cfg.ISOImage)
	if apiTag != isoTag {
		return fmt.Errorf("api-image tag %q and iso-image tag %q must match. please provide same tag", apiTag, isoTag)
	}

	if err := im.LoadImages([]string{cfg.APIImage, cfg.ISOImage}); err != nil {
		return fmt.Errorf("loading images: %w", err)
	}

	if err := im.DeploySecrets(config.Cfg.PrivateKeyFilePath()); err != nil {
		return fmt.Errorf("deploying secrets: %w", err)
	}
	if err := im.DeployPostgres(); err != nil {
		return fmt.Errorf("deploying postgres: %w", err)
	}
	if err := im.DeployVcsim(); err != nil {
		return fmt.Errorf("deploying vcsim: %w", err)
	}

	serviceURL := fmt.Sprintf("http://%s:%d%s", cfg.HostIP, cfg.PlannerAgentPort, cfg.ServiceAPIPath)
	serviceParams := map[string]string{
		"SERVICE_API_PATH":                    cfg.ServiceAPIPath,
		"MIGRATION_PLANNER_URL":               serviceURL,
		"MIGRATION_PLANNER_UI_URL":            fmt.Sprintf("http://%s:%d", cfg.HostIP, cfg.UIPort),
		"MIGRATION_PLANNER_IMAGE_URL":         serviceURL,
		"MIGRATION_PLANNER_IMAGE_PULL_POLICY": cfg.ImagePullPolicy,
		"MIGRATION_PLANNER_ISO_IMAGE":         isoRepo,
		"MIGRATION_PLANNER_IMAGE":             apiRepo,
		"IMAGE_TAG":                           apiTag,
		"MIGRATION_PLANNER_REPLICAS":          cfg.ServiceReplicas,
		"PERSISTENT_DISK_DEVICE":              cfg.PersistentDiskDevice,
		"MIGRATION_PLANNER_AUTH":              cfg.AuthMethod,
		"MIGRATION_PLANNER_ADMIN_GROUP_FILE":  cfg.AdminGroupFile,
		"RHCOS_PASSWORD":                      cfg.RHCOSPassword,
	}
	return im.DeployService(serviceParams)
}

// AfterFailed runs body if the current spec failed, useful for collecting logs.
func AfterFailed(body func()) {
	ginkgo.JustAfterEach(func() {
		if ginkgo.CurrentSpecReport().Failed() {
			ginkgo.By("Running AfterFailed function")
			body()
		}
	})
}

func splitImageRef(ref string) (repo, tag string) {
	if i := strings.LastIndex(ref, ":"); i > 0 && !strings.Contains(ref[i:], "/") {
		return ref[:i], ref[i+1:]
	}
	return ref, "latest"
}

func detectLocalIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}
