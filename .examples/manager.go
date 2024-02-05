package cmd

import (
	"path"

	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/endpoint"
	"github.com/hexiaodai/virtnet/pkg/ippool"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	uruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	runtimeWebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

type Manager struct{}

func (m *Manager) Run() error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{
		Development: true,
	})))

	scheme := runtime.NewScheme()
	uruntime.Must(v1alpha1.AddToScheme(scheme))
	uruntime.Must(corev1.AddToScheme(scheme))

	config := ctrl.GetConfigOrDie()
	config.Burst = 200
	config.QPS = 100
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                  scheme,
		LeaderElectionID:        "ctrl.virtnest.io",
		LeaderElectionNamespace: "kube-system",
		LeaderElection:          true,
		WebhookServer: runtimeWebhook.NewServer(runtimeWebhook.Options{
			Port:    8081,
			CertDir: path.Dir(constant.DefaultSecretPath),
		}),
	})
	if err != nil {
		return err
	}

	if err := (&ippool.IPPoolWebhook{
		Client: mgr.GetClient(),
	}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}

	if err := (&ippool.IPPoolReconciler{
		Client: mgr.GetClient(),
		Cache:  mgr.GetCache(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&endpoint.EndpointReconciler{
		Client: mgr.GetClient(),
		Cache:  mgr.GetCache(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}
