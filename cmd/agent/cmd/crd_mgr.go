package cmd

import (
	"context"

	"github.com/hexiaodai/virtnet/pkg/endpoint"
	"github.com/hexiaodai/virtnet/pkg/ippool"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	clientset "github.com/hexiaodai/virtnet/pkg/k8s/client/clientset/versioned"
	"github.com/hexiaodai/virtnet/pkg/pod"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	uruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	uruntime.Must(v1alpha1.AddToScheme(scheme))
	uruntime.Must(corev1.AddToScheme(scheme))
	uruntime.Must(appsv1.AddToScheme(scheme))
}

type CRDManager struct {
	Mgr ctrl.Manager

	IPPoolClient   *ippool.IPPoolClient
	PodClient      *pod.PodClient
	EndpointClient *endpoint.EndpointClient
}

func NewCRDManager(ctx context.Context) (*CRDManager, error) {
	config := ctrl.GetConfigOrDie()
	config.Burst = 100
	config.QPS = 50
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		return nil, err
	}

	clientset, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	ippoolClient := ippool.NewIPPoolClient(mgr.GetAPIReader(), mgr.GetClient(), mgr.GetCache())
	ippoolClient.SetupClient(ctx, clientset)

	podClient := pod.NewPodClient(mgr.GetClient(), mgr.GetCache(), mgr.GetAPIReader())
	endpointClient := endpoint.NewEndpointClient(mgr.GetClient(), mgr.GetCache())

	return &CRDManager{
		Mgr:            mgr,
		IPPoolClient:   ippoolClient,
		PodClient:      podClient,
		EndpointClient: endpointClient,
	}, nil
}
