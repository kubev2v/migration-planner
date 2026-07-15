package infra

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kubev2v/migration-planner/test/e2e/config"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"
)

const (
	waitForDeploymentTimeout = 2 * time.Minute
	waitForAllPodsTimeout    = 3 * time.Minute
)

type KindInfraManager struct {
	cfg           config.InfraConfig
	provider      *kindcluster.Provider
	clientset     *kubernetes.Clientset
	dynamicClient dynamic.Interface
	restConfig    *rest.Config
	mapper        meta.RESTMapper

	portForwardsMu sync.Mutex
	portForwards   []*portForwarder

	templateDir string
}

type portForwarder struct {
	stopChan chan struct{}
	name     string
}

func NewKindInfraManager(cfg config.InfraConfig) (*KindInfraManager, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	templateDir := filepath.Join(dir, cfg.RelativeTemplatesDir)
	if _, err := os.Stat(templateDir); err != nil {
		return nil, err
	}

	provider := kindcluster.NewProvider()
	m := &KindInfraManager{
		cfg:         cfg,
		provider:    provider,
		templateDir: templateDir,
	}

	if err := m.initKubeClients(); err != nil {
		zap.S().Infof("Cluster %s not reachable yet, will initialize clients in CreateCluster", cfg.ClusterName)
	}

	return m, nil
}

func (k *KindInfraManager) CreateCluster() error {
	zap.S().Infof("Creating Kind cluster %s", k.cfg.ClusterName)
	if err := k.provider.Create(k.cfg.ClusterName); err != nil {
		if strings.Contains(err.Error(), "already exist") {
			zap.S().Infof("Kind cluster %s already exists, reusing", k.cfg.ClusterName)
		} else {
			return fmt.Errorf("creating Kind cluster: %w", err)
		}
	}

	if err := k.initKubeClients(); err != nil {
		return err
	}

	zap.S().Info("Kind cluster ready, waiting for nodes")
	return k.waitForNodes()
}

func (k *KindInfraManager) DeleteCluster() error {
	zap.S().Infof("Deleting Kind cluster %s", k.cfg.ClusterName)
	return k.provider.Delete(k.cfg.ClusterName, "")
}

func (k *KindInfraManager) DeploySecrets(privateKeyPath string) error {
	zap.S().Info("Deploying secrets")

	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("reading private key: %w", err)
	}
	pkBase64 := base64.StdEncoding.EncodeToString(keyData)

	if err := k.applyTemplate("pk-secret-template.yml", map[string]string{
		"E2E_PRIVATE_KEY_BASE64": pkBase64,
	}); err != nil {
		return fmt.Errorf("deploying private key secret: %w", err)
	}

	if err := k.applyTemplate("s3-secret-template.yml", nil); err != nil {
		return fmt.Errorf("deploying S3 secret: %w", err)
	}

	return k.applyTemplate("admin-group-template.yml", map[string]string{
		"ADMIN_USERNAME": k.cfg.AdminUsername,
		"ADMIN_EMAIL":    k.cfg.AdminEmail,
	})
}

func (k *KindInfraManager) DeployPostgres() error {
	zap.S().Info("Deploying PostgreSQL")
	if err := k.applyTemplate("postgres-template.yml", nil); err != nil {
		return err
	}
	return k.waitForDeployment(k.cfg.PostgresDeployName)
}

func (k *KindInfraManager) DeployVcsim() error {
	zap.S().Info("Deploying vcsim instances")

	for _, v := range k.cfg.Vcsim {
		if err := k.applyTemplate("vcsim-template.yml", map[string]string{
			"APP_NAME": v.Name,
			"PORT":     fmt.Sprintf("%d", v.Port),
			"USERNAME": v.Username,
			"PASSWORD": v.Password,
		}); err != nil {
			return fmt.Errorf("deploying %s: %w", v.Name, err)
		}
	}

	for _, v := range k.cfg.Vcsim {
		if err := k.waitForDeployment(v.Name); err != nil {
			return err
		}
	}
	return nil
}

func (k *KindInfraManager) DeployService(params map[string]string) error {
	zap.S().Info("Deploying migration planner service")
	if err := k.applyTemplate("service-template.yml", params); err != nil {
		return err
	}
	return k.waitForAllDeployments()
}

func (k *KindInfraManager) LoadImages(images []string) error {
	for _, img := range images {
		if err := ensureImageExists(img); err != nil {
			return err
		}

		zap.S().Infof("Loading image %s into Kind cluster", img)
		cmd := exec.Command("kind", "load", "docker-image", img, "--name", k.cfg.ClusterName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("loading image %s: %w", img, err)
		}
	}
	return nil
}

func (k *KindInfraManager) SetupPortForwards() error {
	zap.S().Info("Setting up port forwards")

	if err := k.startPortForward("planner-agent", k.cfg.PlannerAgentService, k.cfg.PlannerAgentPort, k.cfg.PlannerAgentPort); err != nil {
		return fmt.Errorf("port-forwarding planner-agent: %w", err)
	}

	for _, v := range k.cfg.Vcsim {
		resource := fmt.Sprintf("deploy/%s", v.Name)
		if err := k.startPortForward(v.Name, resource, v.Port, v.Port); err != nil {
			return fmt.Errorf("port-forwarding %s: %w", v.Name, err)
		}
	}

	time.Sleep(5 * time.Second)
	return nil
}

func (k *KindInfraManager) StopPortForwards() error {
	k.portForwardsMu.Lock()
	defer k.portForwardsMu.Unlock()

	for _, pf := range k.portForwards {
		zap.S().Infof("Stopping port-forward %s", pf.name)
		close(pf.stopChan)
	}
	k.portForwards = nil
	return nil
}

func (k *KindInfraManager) applyTemplate(templateName string, params map[string]string) error {
	templatePath := filepath.Join(k.templateDir, templateName)
	objects, err := ProcessTemplate(templatePath, params)
	if err != nil {
		return fmt.Errorf("processing template %s: %w", templateName, err)
	}

	for _, obj := range objects {
		if err := k.applyObject(obj); err != nil {
			return fmt.Errorf("applying %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

func (k *KindInfraManager) applyObject(obj unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	mapping, err := k.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}
	gvr := mapping.Resource

	ns := obj.GetNamespace()
	if ns == "" {
		ns = k.cfg.Namespace
	}

	ctx := context.Background()
	_, err = k.dynamicClient.Resource(gvr).Namespace(ns).Create(ctx, &obj, metav1.CreateOptions{})
	if err == nil {
		zap.S().Debugf("Created %s/%s", obj.GetKind(), obj.GetName())
		return nil
	}

	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}

	existing, err := k.dynamicClient.Resource(gvr).Namespace(ns).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting existing %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = k.dynamicClient.Resource(gvr).Namespace(ns).Update(ctx, &obj, metav1.UpdateOptions{})
	if err != nil {
		zap.S().Warnf("Update failed for %s/%s (may be immutable), skipping: %v", obj.GetKind(), obj.GetName(), err)
		return nil
	}
	zap.S().Debugf("Updated %s/%s", obj.GetKind(), obj.GetName())
	return nil
}

func ensureImageExists(image string) error {
	if exec.Command("docker", "image", "inspect", image).Run() == nil {
		zap.S().Infof("Image %s found locally", image)
		return nil
	}

	zap.S().Infof("Image %s not found locally, pulling", image)
	cmd := exec.Command("docker", "pull", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pulling image %s: %w", image, err)
	}
	return nil
}

func (k *KindInfraManager) initKubeClients() error {
	kubeconfig, err := k.provider.KubeConfig(k.cfg.ClusterName, false)
	if err != nil {
		return err
	}

	k.restConfig, err = clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return fmt.Errorf("building REST config: %w", err)
	}

	k.clientset, err = kubernetes.NewForConfig(k.restConfig)
	if err != nil {
		return fmt.Errorf("creating Kubernetes clientset: %w", err)
	}

	k.dynamicClient, err = dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	dc, err := discovery.NewDiscoveryClientForConfig(k.restConfig)
	if err != nil {
		return fmt.Errorf("creating discovery client: %w", err)
	}

	groups, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return fmt.Errorf("fetching API group resources: %w", err)
	}

	k.mapper = restmapper.NewDiscoveryRESTMapper(groups)

	return nil
}

func (k *KindInfraManager) getPodForService(serviceName string) (string, error) {
	ctx := context.Background()
	svc, err := k.clientset.CoreV1().Services(k.cfg.Namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting service %s: %w", serviceName, err)
	}

	var labelParts []string
	for key, val := range svc.Spec.Selector {
		labelParts = append(labelParts, fmt.Sprintf("%s=%s", key, val))
	}

	pods, err := k.clientset.CoreV1().Pods(k.cfg.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: strings.Join(labelParts, ","),
	})
	if err != nil || len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for service %s", serviceName)
	}
	return pods.Items[0].Name, nil
}

func (k *KindInfraManager) getPodForDeployment(deployName string) (string, error) {
	ctx := context.Background()
	deploy, err := k.clientset.AppsV1().Deployments(k.cfg.Namespace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting deployment %s: %w", deployName, err)
	}

	var labelParts []string
	for key, val := range deploy.Spec.Selector.MatchLabels {
		labelParts = append(labelParts, fmt.Sprintf("%s=%s", key, val))
	}

	pods, err := k.clientset.CoreV1().Pods(k.cfg.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: strings.Join(labelParts, ","),
	})
	if err != nil || len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for deployment %s", deployName)
	}
	return pods.Items[0].Name, nil
}

func (k *KindInfraManager) waitForNodes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for {
		nodes, err := k.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil && len(nodes.Items) > 0 {
			allReady := true
			for _, node := range nodes.Items {
				ready := false
				for _, c := range node.Status.Conditions {
					if c.Type == "Ready" && c.Status == "True" {
						ready = true
						break
					}
				}
				if !ready {
					allReady = false
					break
				}
			}
			if allReady {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for nodes to be ready")
		case <-time.After(5 * time.Second):
		}
	}
}

func (k *KindInfraManager) waitForDeployment(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), waitForDeploymentTimeout)
	defer cancel()

	zap.S().Infof("Waiting for deployment %s", name)
	for {
		deploy, err := k.clientset.AppsV1().Deployments(k.cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil && deploy.Status.ReadyReplicas > 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for deployment %s", name)
		case <-time.After(5 * time.Second):
		}
	}
}

func (k *KindInfraManager) waitForAllDeployments() error {
	ctx, cancel := context.WithTimeout(context.Background(), waitForAllPodsTimeout)
	defer cancel()

	zap.S().Info("Waiting for all deployments to be ready")
	for {
		deploys, err := k.clientset.AppsV1().Deployments(k.cfg.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			select {
			case <-ctx.Done():
				return fmt.Errorf("timed out listing deployments: %w", err)
			case <-time.After(5 * time.Second):
				continue
			}
		}

		allReady := true
		for _, d := range deploys.Items {
			desired := int32(1)
			if d.Spec.Replicas != nil {
				desired = *d.Spec.Replicas
			}
			if d.Status.ReadyReplicas < desired {
				allReady = false
				break
			}
		}

		if allReady {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for all deployments to be ready")
		case <-time.After(5 * time.Second):
		}
	}
}

func (k *KindInfraManager) startPortForward(name, resource string, localPort, remotePort int) error {
	parts := strings.SplitN(resource, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid resource format %q, expected type/name", resource)
	}
	resourceType, resourceName := parts[0], parts[1]

	var podName string
	var err error

	switch resourceType {
	case "service":
		podName, err = k.getPodForService(resourceName)
	case "deploy", "deployment":
		podName, err = k.getPodForDeployment(resourceName)
	default:
		return fmt.Errorf("unsupported resource type %q for port-forward", resourceType)
	}
	if err != nil {
		return err
	}

	reqURL := k.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(k.cfg.Namespace).
		Name(podName).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(k.restConfig)
	if err != nil {
		return fmt.Errorf("creating SPDY round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, reqURL)

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}

	listenAddresses := []string{"0.0.0.0"}
	pf, err := portforward.NewOnAddresses(dialer, listenAddresses, ports, stopChan, readyChan, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("creating port forwarder: %w", err)
	}

	go func() {
		if err := pf.ForwardPorts(); err != nil {
			zap.S().Errorf("Port-forward %s failed: %v", name, err)
		}
	}()

	select {
	case <-readyChan:
		zap.S().Infof("Port-forward %s ready: 0.0.0.0:%d -> %d", name, localPort, remotePort)
	case <-time.After(30 * time.Second):
		close(stopChan)
		return fmt.Errorf("timed out waiting for port-forward %s", name)
	}

	k.portForwardsMu.Lock()
	k.portForwards = append(k.portForwards, &portForwarder{stopChan: stopChan, name: name})
	k.portForwardsMu.Unlock()

	return nil
}
