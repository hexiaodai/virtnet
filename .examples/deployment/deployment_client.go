package pod

import (
	"context"
	"fmt"

	"github.com/hexiaodai/virtnet/pkg/types"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var clientLogger *zap.SugaredLogger

type DeploymentClient struct {
	Client client.Client
	Cache  cache.Cache
}

func NewDeploymentClientWithManager(mgr ctrl.Manager) *DeploymentClient {
	if clientLogger == nil {
		clientLogger = logger.Named("DeploymentClient")
	}
	clientLogger.Debug("Setting up DeploymentClient with manager")
	return &DeploymentClient{
		Client: mgr.GetClient(),
		Cache:  mgr.GetCache(),
	}
}

func (client *DeploymentClient) GetServiceClusterIPRangeFromCache(ctx context.Context) ([]string, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: corev1.SchemeGroupVersion.WithKind("Pod"),
		Namespace:        "kube-system",
		Name:             "",
	}

	var pod corev1.Pod
	if err := client.Cache.Get(ctx, namespacedName, &pod); err != nil {
		return nil, fmt.Errorf("failed to get pod using 'client.Cache.Get()', resource is '%v': %w", appNamespacedName.String(), err)
	}

	return &pod, nil
}
