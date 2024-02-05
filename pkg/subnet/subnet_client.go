package subnet

import (
	"context"
	"fmt"

	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/types"

	apitypes "k8s.io/apimachinery/pkg/types"

	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var clientLogger *zap.SugaredLogger

var (
// errNoUnallocatedIPs = errors.New("no unallocated IPs in Subnet")
)

type SubnetClient struct {
	Client client.Client
	Cache  cache.Cache
}

func NewSubnetClientWithManager(mgr ctrl.Manager) *SubnetClient {
	if clientLogger == nil {
		clientLogger = logger.Named("SubnetClient")
	}
	clientLogger.Debug("Setting up SubnetClient with manager")
	return &SubnetClient{
		Client: mgr.GetClient(),
		Cache:  mgr.GetCache(),
	}
}

func (client *SubnetClient) GetSubnetByNameFromCache(ctx context.Context, poolName string) (*v1alpha1.Subnet, error) {
	var ipPool v1alpha1.Subnet
	if err := client.Cache.Get(ctx, apitypes.NamespacedName{Name: poolName}, &ipPool); err != nil {
		return nil, fmt.Errorf("failed to get subnet %v, 'client.Cache.Get()': %w", poolName, err)
	}

	return &ipPool, nil
}

func (client *SubnetClient) GetUnallocatedSubnet(ctx context.Context, opts ...client.ListOption) (*v1alpha1.Subnet, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.SubnetGVK,
	}

	var ipPoolList v1alpha1.SubnetList
	if err := client.Cache.List(ctx, &ipPoolList, opts...); err != nil {
		return nil, fmt.Errorf("failed to get subnets, resource is '%v': %w", appNamespacedName.String(), err)
	}

	// for _, ipPool := range ipPoolList.Items {
	// 	// FIXME: Subnet reconciler did not update UnallocatedIPCount in time.
	// 	if ptr.OrEmpty(ipPool.Status.UnallocatedIPCount) > 0 {
	// 		return &ipPool, nil
	// 	}
	// }

	return nil, fmt.Errorf("no unallocated subnet found")
}

func (client *SubnetClient) ListSubnetsFromCache(ctx context.Context, opts ...client.ListOption) (*v1alpha1.SubnetList, error) {
	var ipPoolList v1alpha1.SubnetList
	if err := client.Cache.List(ctx, &ipPoolList, opts...); err != nil {
		return nil, fmt.Errorf("failed to get subnets 'client.Cache.List()': %w", err)
	}

	return &ipPoolList, nil
}
