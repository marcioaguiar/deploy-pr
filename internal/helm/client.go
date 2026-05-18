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

// kubeUnreachableHint is the actionable message appended to errors that
// indicate client-go could not reach the API server. The substring is also
// used as a guard to avoid double-annotating.
const kubeUnreachableHint = "hint: no usable kubeconfig was found. Set KUBECONFIG, " +
	"or run `aws eks update-kubeconfig --name <cluster> --region <region>` " +
	"to populate ~/.kube/config, then retry."

// annotateUnreachable adds an actionable hint when err indicates the Kubernetes
// API server could not be reached. The default symptom is client-go falling
// back to http://localhost:8080 when no kubeconfig is loaded, which surfaces as
// Helm's "Kubernetes cluster unreachable" wrapper. Other errors pass through.
func annotateUnreachable(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, kubeUnreachableHint) {
		return err
	}
	if !strings.Contains(msg, "Kubernetes cluster unreachable") &&
		!strings.Contains(msg, "127.0.0.1:8080") &&
		!strings.Contains(msg, "localhost:8080") {
		return err
	}
	return fmt.Errorf("%w\n\n%s", err, kubeUnreachableHint)
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
		return nil, annotateUnreachable(fmt.Errorf("helm config init: %w", err))
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
	// CreateNamespace asks Helm to create the target namespace on first
	// install. Requires cluster-scoped "create namespaces" RBAC on the
	// caller's principal; set false when an admin pre-creates pr-<N>.
	CreateNamespace bool
}

// UpgradeOrInstall is the idempotent entry point: if the release does not
// exist, it runs Install (creating the namespace when CreateNamespace is set);
// otherwise it runs Upgrade. Wait+WaitForJobs are on by default so callers
// can trust that a successful return means the rollout actually came up.
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
			return nil, annotateUnreachable(fmt.Errorf("history %s: %w", opts.ReleaseName, err))
		}
		install := action.NewInstall(m.config)
		install.ReleaseName = opts.ReleaseName
		install.Namespace = opts.Namespace
		install.CreateNamespace = opts.CreateNamespace
		install.Wait = true
		install.WaitForJobs = true
		install.Timeout = timeout
		m.log.Info("helm install", "release", opts.ReleaseName, "namespace", opts.Namespace)
		rel, err := install.RunWithContext(ctx, opts.Chart, opts.Values)
		return rel, annotateUnreachable(err)
	}

	upgrade := action.NewUpgrade(m.config)
	upgrade.Namespace = opts.Namespace
	upgrade.Wait = true
	upgrade.WaitForJobs = true
	upgrade.Timeout = timeout
	m.log.Info("helm upgrade", "release", opts.ReleaseName, "namespace", opts.Namespace)
	rel, err := upgrade.RunWithContext(ctx, opts.ReleaseName, opts.Chart, opts.Values)
	return rel, annotateUnreachable(err)
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
		return annotateUnreachable(fmt.Errorf("uninstall %s: %w", releaseName, err))
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
		return nil, annotateUnreachable(fmt.Errorf("helm config init: %w", err))
	}
	list := action.NewList(cfg)
	list.AllNamespaces = true
	list.All = true
	rels, err := list.Run()
	if err != nil {
		return nil, annotateUnreachable(fmt.Errorf("helm list: %w", err))
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
		return nil, annotateUnreachable(fmt.Errorf("rest config: %w", err))
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, annotateUnreachable(fmt.Errorf("kube client: %w", err))
	}
	list, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, annotateUnreachable(fmt.Errorf("list namespaces: %w", err))
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
		return annotateUnreachable(fmt.Errorf("rest config: %w", err))
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return annotateUnreachable(fmt.Errorf("kube client: %w", err))
	}
	if err := cs.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return annotateUnreachable(fmt.Errorf("delete namespace %s: %w", name, err))
	}
	return nil
}
