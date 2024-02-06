package cmd

import (
	"path"

	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/endpoint"
	"github.com/hexiaodai/virtnet/pkg/env"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/subnet"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	uruntime "k8s.io/apimachinery/pkg/util/runtime"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	runtimeWebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

var scheme = runtime.NewScheme()

func init() {
	uruntime.Must(v1alpha1.AddToScheme(scheme))
	uruntime.Must(corev1.AddToScheme(scheme))
	uruntime.Must(appsv1.AddToScheme(scheme))
	uruntime.Must(virtv1.AddToScheme(scheme))
}

func NewCRDManager() (ctrl.Manager, error) {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{
		Development: true,
	})))

	config := ctrl.GetConfigOrDie()
	config.Burst = 200
	config.QPS = 100
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                  scheme,
		LeaderElectionID:        "virtnet.io",
		LeaderElectionNamespace: env.Lookup("NAMESPACE", "kube-system"),
		LeaderElection:          true,
		WebhookServer: runtimeWebhook.NewServer(runtimeWebhook.Options{
			Port:    env.Lookup("WEBHOOK_SERVER_PORT", 8081),
			CertDir: path.Dir(constant.DefaultSecretPath),
		}),
	})
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

func RegisterCtrl(mgr ctrl.Manager) error {
	if err := (&subnet.SubnetWebhook{
		Client: mgr.GetClient(),
	}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}

	if err := (&subnet.SubnetReconciler{
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

	return nil
}
