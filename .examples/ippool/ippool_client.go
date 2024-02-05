package ippool

import (
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"
	"cni/pkg/logging"
	"cni/pkg/ptr"
	"cni/pkg/types"
	"cni/pkg/utils/convert"
	"context"
	"errors"
	"fmt"
	"net"

	"cni/pkg/utils/retry"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	corev1 "k8s.io/api/core/v1"

	apitypes "k8s.io/apimachinery/pkg/types"

	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var clientLogger *zap.SugaredLogger

var (
	errNoUnallocatedIPs = errors.New("no unallocated IPs in IPPool")
)

type IPPoolClient struct {
	Client client.Client
	Cache  cache.Cache
}

func NewIPPoolClientWithManager(mgr ctrl.Manager) *IPPoolClient {
	if clientLogger == nil {
		clientLogger = logger.Named("IPPoolClient")
	}
	clientLogger.Debug("Setting up ippoolClient with manager")
	return &IPPoolClient{
		Client: mgr.GetClient(),
		Cache:  mgr.GetCache(),
	}
}

func (client *IPPoolClient) GetIPPoolByName(ctx context.Context, poolName string) (*v1alpha1.IPPool, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
		Name:             poolName,
	}

	var ipPool v1alpha1.IPPool
	if err := client.Cache.Get(ctx, apitypes.NamespacedName{Name: poolName}, &ipPool); err != nil {
		return nil, fmt.Errorf("failed to get ippool using 'client.Cache.Get()', resource is '%v': %w", appNamespacedName.String(), err)
	}

	return &ipPool, nil
}

func (client *IPPoolClient) GetUnallocatedIPPool(ctx context.Context, opts ...client.ListOption) (*v1alpha1.IPPool, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
	}

	var ipPoolList v1alpha1.IPPoolList
	if err := client.Cache.List(ctx, &ipPoolList, opts...); err != nil {
		return nil, fmt.Errorf("failed to get ippools using 'client.Cache.List()', resource is '%v': %w", appNamespacedName.String(), err)
	}

	for _, ipPool := range ipPoolList.Items {
		// FIXME: IPPool reconciler did not update UnallocatedIPCount in time.
		if ptr.OrEmpty(ipPool.Status.UnallocatedIPCount) > 0 {
			return &ipPool, nil
		}
	}

	return nil, fmt.Errorf("no unallocated ippool found")
}

func (client *IPPoolClient) ListIPPools(ctx context.Context, opts ...client.ListOption) (*v1alpha1.IPPoolList, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
	}

	var ipPoolList v1alpha1.IPPoolList
	if err := client.Cache.List(ctx, &ipPoolList, opts...); err != nil {
		return nil, fmt.Errorf("failed to get ippools using 'client.Cache.List()', resource is '%v': %w", appNamespacedName.String(), err)
	}

	return &ipPoolList, nil
}

func (client *IPPoolClient) AllocateIP(ctx context.Context, ippool types.IPAMAnnoIPPoolItem, pod *corev1.Pod) (net.IP, error) {
	var ip net.IP

	currentPoolName := ippool.Next()

	backoff := retry.RetryMaxSteps
	steps := backoff.Steps
	err := retry.RetryOnConflictWithContext(ctx, backoff, func(ctx context.Context) error {
		appNamespacedName := types.AppNamespacedName{
			GroupVersionKind: v1alpha1.IPPoolGVK,
			Name:             currentPoolName,
		}
		logger := clientLogger.Named("AllocateIP").
			With("resource", appNamespacedName).
			With("Times", steps-backoff.Steps+1)

		logger.Debug("Re-get IPPool for IP allocation")
		newIP, newIPPool, err := client.allocateIP(ctx, ippool, currentPoolName, pod)
		if err != nil {
			if errors.Is(err, errNoUnallocatedIPs) {
				logger.Warn("No unallocated IPs in the IPPool")
				currentPoolName = ippool.Next()
				if len(currentPoolName) == 0 {
					return err
				}
				// Retry AllocateIP
				return apierrors.NewConflict(v1alpha1.IPPoolGVR.GroupResource(), currentPoolName, errors.New("no unallocated IPs in the IPPool"))
			}
			return fmt.Errorf("failed to allocate IP from IPPool '%s': %w", currentPoolName, err)
		}
		if err := client.Client.Status().Update(ctx, newIPPool); err != nil {
			logger.With("IPPool-ResourceVersion", newIPPool.ResourceVersion).Warn("An conflict occurred when cleaning the IP allocation records of IPPool")
			return err
		}
		ip = newIP
		return nil
	})
	if err != nil {
		if wait.Interrupted(err) {
			err = fmt.Errorf("exhaust all retries (%d times), failed to allocate IP from IPPool %s", steps, currentPoolName)
		}
		return nil, err
	}

	return ip, nil
}

func (client *IPPoolClient) allocateIP(ctx context.Context, annoIPPool types.IPAMAnnoIPPoolItem, currentPoolName string, pod *corev1.Pod) (net.IP, *v1alpha1.IPPool, error) {
	logger := logging.FromContext(ctx)

	ippool, err := client.GetIPPoolByName(ctx, currentPoolName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get ippool '%s' using 'client.GetIPPoolByName()': %w", currentPoolName, err)
	}
	logger.Debugw("client.GetIPPoolByName()", "IPPool", ippool)

	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
	if err != nil {
		return nil, nil, errNoUnallocatedIPs
		// return nil, nil, fmt.Errorf("failed to unmarshal ippool '%s' unallocatedIPs: %w", poolName, err)
	}
	if len(unallocatedIPs) == 0 {
		return nil, nil, fmt.Errorf("no unallocated IPs in ippool '%s'", currentPoolName)
	}

	allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal ippool '%s' allocatedIPs: %w", currentPoolName, err)
	}

	var ip net.IP

	// if len(annoIPPool.Address) > 0 {
	// 	for _, uip := range unallocatedIPs {
	// 		if uip == annoIPPool.Address {
	// 			// TODO:
	// 			break
	// 		}
	// 	}
	// } else {
	// 	selectIP := unallocatedIPs[0]
	// 	ip = net.ParseIP(string(selectIP))
	// 	unallocatedIPs = unallocatedIPs[1:]
	// 	allocatedIPs[selectIP] = v1alpha1.PoolIPAllocation{
	// 		NamespacedName: (apitypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}).String(),
	// 		PodUID:         string(pod.UID),
	// 	}
	// }
	selectIP := unallocatedIPs[0]
	ip = net.ParseIP(string(selectIP))
	unallocatedIPs = unallocatedIPs[1:]
	allocatedIPs[selectIP] = v1alpha1.PoolIPAllocation{
		NamespacedName: (apitypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}).String(),
		PodUID:         string(pod.UID),
	}

	rawUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ippool '%s' unallocatedIPs: %w", currentPoolName, err)
	}
	rawAllocatedIPs, err := convert.MarshalIPPoolAllocatedIPs(allocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ippool '%s' allocatedIPs: %w", currentPoolName, err)
	}

	if rawUnallocatedIPs == nil || len(ptr.OrEmpty(rawUnallocatedIPs)) == 0 {
		ippool.Status.UnallocatedIPs = nil
	} else {
		ippool.Status.UnallocatedIPs = rawUnallocatedIPs
	}
	if rawAllocatedIPs == nil || len(ptr.OrEmpty(rawAllocatedIPs)) == 0 {
		ippool.Status.AllocatedIPs = nil
	} else {
		ippool.Status.AllocatedIPs = rawAllocatedIPs
	}

	ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
	ippool.Status.AllocatedIPCount = ptr.Of(int64(len(allocatedIPs)))

	return ip, ippool, nil
}
