package helm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Manager wraps Helm's action package + a kubernetes.Interface derived from
// the same RESTClientGetter so install/upgrade/uninstall and namespace
// deletion all hit the same kube context the user (or CI) configured.
type Manager struct {
	settings  *cli.EnvSettings
	config    *action.Configuration
	namespace string
	log       *slog.Logger
}

// NewManager builds a Manager scoped to namespace using the user's standard
// kubeconfig precedence (KUBECONFIG env, then ~/.kube/config). The same
// settings are used for any subsequent cluster calls.
func NewManager(namespace string, logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}
	settings := cli.New()
	if namespace != "" {
		settings.SetNamespace(namespace)
	}

	debug := func(format string, args ...any) {
		logger.Debug("helm", "msg", fmt.Sprintf(format, args...))
	}

	cfg := new(action.Configuration)
	if err := cfg.Init(settings.RESTClientGetter(), settings.Namespace(), "secret", debug); err != nil {
		return nil, fmt.Errorf("helm config init: %w", err)
	}
	return &Manager{
		settings:  settings,
		config:    cfg,
		namespace: settings.Namespace(),
		log:       logger,
	}, nil
}

// UpgradeOptions parameterises a single upgrade-or-install.
type UpgradeOptions struct {
	ReleaseName string
	Namespace   string
	Chart       *chart.Chart
	Values      map[string]any
	Timeout     time.Duration
}

// UpgradeOrInstall is the idempotent entry point: if the release does not
// exist, it runs Install with --create-namespace; otherwise it runs Upgrade.
// Wait+WaitForJobs are on by default so callers can trust that a successful
// return means the rollout actually came up.
func (m *Manager) UpgradeOrInstall(ctx context.Context, opts UpgradeOptions) (*release.Release, error) {
	if opts.ReleaseName == "" || opts.Namespace == "" || opts.Chart == nil {
		return nil, errors.New("ReleaseName, Namespace, and Chart are required")
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	hist := action.NewHistory(m.config)
	hist.Max = 1
	if _, err := hist.Run(opts.ReleaseName); err != nil {
		if !errors.Is(err, driver.ErrReleaseNotFound) {
			return nil, fmt.Errorf("history %s: %w", opts.ReleaseName, err)
		}
		install := action.NewInstall(m.config)
		install.ReleaseName = opts.ReleaseName
		install.Namespace = opts.Namespace
		install.CreateNamespace = true
		install.Wait = true
		install.WaitForJobs = true
		install.Timeout = timeout
		m.log.Info("helm install", "release", opts.ReleaseName, "namespace", opts.Namespace)
		return install.RunWithContext(ctx, opts.Chart, opts.Values)
	}

	upgrade := action.NewUpgrade(m.config)
	upgrade.Namespace = opts.Namespace
	upgrade.Wait = true
	upgrade.WaitForJobs = true
	upgrade.Timeout = timeout
	m.log.Info("helm upgrade", "release", opts.ReleaseName, "namespace", opts.Namespace)
	return upgrade.RunWithContext(ctx, opts.ReleaseName, opts.Chart, opts.Values)
}

// Uninstall removes the named release. A missing release is treated as a no-op
// success so `down` is idempotent.
func (m *Manager) Uninstall(releaseName string) error {
	uninstall := action.NewUninstall(m.config)
	uninstall.Wait = true
	if _, err := uninstall.Run(releaseName); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			m.log.Info("release not found, nothing to uninstall", "release", releaseName)
			return nil
		}
		return fmt.Errorf("uninstall %s: %w", releaseName, err)
	}
	return nil
}

// ListReleasesByNamespacePrefix returns every Helm release whose namespace
// begins with prefix, across every namespace the caller can see. Used by
// `deploy-pr list` to enumerate active previews.
func ListReleasesByNamespacePrefix(prefix string, logger *slog.Logger) ([]*release.Release, error) {
	if logger == nil {
		logger = slog.Default()
	}
	settings := cli.New()
	debug := func(format string, args ...any) {
		logger.Debug("helm", "msg", fmt.Sprintf(format, args...))
	}
	cfg := new(action.Configuration)
	if err := cfg.Init(settings.RESTClientGetter(), "", "secret", debug); err != nil {
		return nil, fmt.Errorf("helm config init: %w", err)
	}
	list := action.NewList(cfg)
	list.AllNamespaces = true
	list.All = true
	rels, err := list.Run()
	if err != nil {
		return nil, fmt.Errorf("helm list: %w", err)
	}
	out := make([]*release.Release, 0, len(rels))
	for _, r := range rels {
		if strings.HasPrefix(r.Namespace, prefix) {
			out = append(out, r)
		}
	}
	return out, nil
}

// ListNamespacesWithPrefix returns the names of every namespace beginning
// with prefix. Used to detect orphans (namespace without Helm release) in
// `deploy-pr list`.
func ListNamespacesWithPrefix(ctx context.Context, prefix string) ([]string, error) {
	settings := cli.New()
	restCfg, err := settings.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("rest config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kube client: %w", err)
	}
	list, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	out := []string{}
	for _, ns := range list.Items {
		if strings.HasPrefix(ns.Name, prefix) {
			out = append(out, ns.Name)
		}
	}
	return out, nil
}

// DeleteNamespace removes a namespace via the kube API. A NotFound error is
// treated as success so callers can chain Uninstall + DeleteNamespace without
// worrying about partial state.
func (m *Manager) DeleteNamespace(ctx context.Context, name string) error {
	restCfg, err := m.settings.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return fmt.Errorf("rest config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("kube client: %w", err)
	}
	if err := cs.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete namespace %s: %w", name, err)
	}
	return nil
}
