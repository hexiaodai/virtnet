package cmd

import (
	"context"
	"fmt"

	"github.com/hexiaodai/virtnet/pkg/barrel"
	"github.com/hexiaodai/virtnet/pkg/endpoint"
	"github.com/hexiaodai/virtnet/pkg/ippool"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	clientset "github.com/hexiaodai/virtnet/pkg/k8s/client/clientset/versioned"
	"github.com/hexiaodai/virtnet/pkg/pod"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	uruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func getDaemonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "VirtNet Agent Daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setupDaemon()
		},
	}

	return cmd
}

func setupDaemon() error {
	scheme := runtime.NewScheme()
	uruntime.Must(v1alpha1.AddToScheme(scheme))
	uruntime.Must(corev1.AddToScheme(scheme))

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
		return err
	}

	clientset, err := clientset.NewForConfig(config)
	if err != nil {
		return err
	}
	barrelClient := barrel.NewBarrelClient(mgr.GetAPIReader(), mgr.GetClient(), mgr.GetCache())
	if err := barrelClient.SetupClient(context.TODO(), clientset); err != nil {
		return err
	}
	ippoolClient := ippool.NewIPPoolClientWithManager(mgr)
	podClient := pod.NewPodClient(mgr.GetClient(), mgr.GetCache())
	endpointClient := endpoint.NewEndpointClient(mgr.GetClient(), mgr.GetCache())

	server := NewServer(mgr, ippoolClient, barrelClient, podClient, endpointClient)

	go func() {
		if err := server.Start(); err != nil {
			panic(fmt.Errorf("failed to start server: %v", err))
		}
	}()

	if err := mgr.Start(context.TODO()); err != nil {
		return err
	}

	return nil
}
