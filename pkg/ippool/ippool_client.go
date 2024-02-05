package ippool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	clientset "github.com/hexiaodai/virtnet/pkg/k8s/client/clientset/versioned"
	"github.com/hexiaodai/virtnet/pkg/k8s/client/informers/externalversions"
	"github.com/hexiaodai/virtnet/pkg/logging"
	"github.com/hexiaodai/virtnet/pkg/ptr"
	"github.com/hexiaodai/virtnet/pkg/types"
	"github.com/hexiaodai/virtnet/pkg/utils/convert"
	"github.com/hexiaodai/virtnet/pkg/utils/retry"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errNoUnallocatedIPs = errors.New("no unallocated IPs in IPPool")
)

var clientLogger *zap.SugaredLogger

type IPPoolClient struct {
	ClientReader client.Reader
	Client       client.Client
	Cache        ctrlcache.Cache

	subnetPools map[string]*subnetPools
	mutex       sync.Mutex
}

type subnetPool struct {
	pool  string
	mutex sync.Mutex
}

type subnetPools struct {
	subnetPools []*subnetPool
	index       int
}

func NewIPPoolClient(clientReader client.Reader, client client.Client, cache ctrlcache.Cache) *IPPoolClient {
	if clientLogger == nil {
		clientLogger = logger.Named("IPPoolClient")
	}
	return &IPPoolClient{
		subnetPools:  map[string]*subnetPools{},
		ClientReader: clientReader,
		Client:       client,
		Cache:        cache,
	}
}

func (ic *IPPoolClient) SetupClient(ctx context.Context, clientInter clientset.Interface) error {
	clientLogger.Debug("Setting up client with clientset.Interface")

	factory := externalversions.NewSharedInformerFactory(clientInter, 0)
	ippoolInformer := factory.Virtnet().V1alpha1().IPPools()

	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ippool := obj.(*v1alpha1.IPPool)
			if ippool.Labels == nil || len(ippool.Labels[constant.IPPoolLabelOwner]) == 0 {
				clientLogger.Debugf("'%s' has no owner, skipping add", ippool.Name)
				return
			}

			ownerName := ippool.Labels[constant.IPPoolLabelOwner]
			clientLogger.Debugf("'%s' added, owner: '%s'", ippool.Name, ownerName)

			ic.mutex.Lock()
			defer ic.mutex.Unlock()
			result, ok := ic.subnetPools[ownerName]
			newSubnetPool := &subnetPool{
				pool: ippool.Name,
			}
			if ok {
				result.subnetPools = append(result.subnetPools, newSubnetPool)
				ic.subnetPools[ownerName] = result
			} else {
				ic.subnetPools[ownerName] = &subnetPools{
					subnetPools: []*subnetPool{newSubnetPool},
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {
			delIPPool := obj.(*v1alpha1.IPPool)
			if delIPPool.Labels == nil || len(delIPPool.Labels[constant.IPPoolLabelOwner]) == 0 {
				clientLogger.Debugf("'%s' has no owner, skipping delete", delIPPool.Name)
				return
			}

			ownerName := delIPPool.Labels[constant.IPPoolLabelOwner]
			clientLogger.Debugf("'%s' deleted, owner: '%s'", delIPPool.Name, ownerName)

			ic.mutex.Lock()
			defer ic.mutex.Unlock()
			subnetPools, ok := ic.subnetPools[ownerName]
			if ok {
				newIPPoolsNames := []*subnetPool{}
				for _, pb := range subnetPools.subnetPools {
					if pb.pool == delIPPool.Name {
						continue
					}
					newIPPoolsNames = append(newIPPoolsNames, pb)
				}
				if subnetPools.index >= len(newIPPoolsNames) {
					subnetPools.index = 0
				}
				subnetPools.subnetPools = newIPPoolsNames
				ic.subnetPools[ownerName] = subnetPools
			} else {
				clientLogger.Debugf("'%s' not found in the pool, skipping delete", ownerName)
			}
		},
	}
	if _, err := ippoolInformer.Informer().AddEventHandler(eventHandler); err != nil {
		return fmt.Errorf("failed to AddEventHandler, error: %v", err)
	}

	factory.Start(ctx.Done())

	return nil
}

func (ic *IPPoolClient) GetIPPoolByName(ctx context.Context, ippoolName string) (*v1alpha1.IPPool, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
		Name:             ippoolName,
	}

	var ippool v1alpha1.IPPool
	if err := ic.ClientReader.Get(ctx, apitypes.NamespacedName{Name: ippoolName}, &ippool); err != nil {
		return nil, fmt.Errorf("failed to get ippool 'client.Client.Get()', resource is '%v': %w", appNamespacedName.String(), err)
	}

	return &ippool, nil
}

func (ic *IPPoolClient) ListIPPoolsFromCache(ctx context.Context, opts ...client.ListOption) (*v1alpha1.IPPoolList, error) {
	var ippoolList v1alpha1.IPPoolList
	if err := ic.Cache.List(ctx, &ippoolList, opts...); err != nil {
		return nil, fmt.Errorf("failed to get IPPools 'Cache.List()': %w", err)
	}

	return &ippoolList, nil
}

func (ic *IPPoolClient) ListIPPoolsWithOwnerFromCache(ctx context.Context, owner string, opts ...client.ListOption) (*v1alpha1.IPPoolList, error) {
	var ippoolList v1alpha1.IPPoolList
	if err := ic.Cache.List(ctx, &ippoolList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{constant.IPPoolLabelOwner: owner}),
	}); err != nil {
		return nil, fmt.Errorf("failed to get IPPools 'Cache.List()', owner '%v': %w", owner, err)
	}

	return &ippoolList, nil
}

func (ic *IPPoolClient) AllocateIP(ctx context.Context, subnet types.IPAMAnnoSubnet, pod *corev1.Pod) (*v1alpha1.IPPool, net.IP, error) {
	var ip net.IP
	var currentPool *v1alpha1.IPPool
	noUnallocatedIPs := true

	backoff := retry.DefaultRetry
	steps := backoff.Steps
	err := retry.RetryOnConflictWithContext(ctx, backoff, func(ctx context.Context) error {
		var subnetPool *subnetPool
		if noUnallocatedIPs {
			res, ok := ic.next(subnet.IPv4Subnet)
			if !ok {
				return fmt.Errorf("failed to get next IPPool for Subnet '%s'", subnet.IPv4Subnet)
			}
			noUnallocatedIPs = false
			subnetPool = res
		}
		subnetPool.mutex.Lock()
		defer subnetPool.mutex.Unlock()

		appNamespacedName := types.AppNamespacedName{
			GroupVersionKind: v1alpha1.IPPoolGVK,
			Name:             subnet.IPv4Subnet,
		}
		logger := clientLogger.Named("AllocateIP").
			With(zap.Any("resource", appNamespacedName), zap.Any("Times", steps-backoff.Steps+1))

		logger.Debug("Re-get IPPool for IP allocation")
		newIP, newIPPool, err := ic.allocateIP(ctx, subnetPool, pod)
		if err != nil {
			if errors.Is(err, errNoUnallocatedIPs) {
				logger.Warn("No unallocated IPs in the IPPool '%s'", subnetPool.pool)
				noUnallocatedIPs = true
				// Retry AllocateIP
				return apierrors.NewConflict(v1alpha1.IPPoolGVR.GroupResource(), subnetPool.pool, errors.New("no unallocated IPs in the IPPool"))
			}
			return fmt.Errorf("failed to allocate IP from IPPool '%s': %w", subnetPool.pool, err)
		}
		currentPool = newIPPool
		if err := ic.Client.Status().Update(ctx, newIPPool); err != nil {
			logger.With(zap.String("IPPool-ResourceVersion", newIPPool.ResourceVersion),
				zap.String("Subnet", subnet.IPv4Subnet),
				zap.String("IPPool", newIPPool.Name)).
				Warn("An conflict occurred when cleaning the IP allocation records of IPPool")
			return err
		}
		ip = newIP
		return nil
	})
	if err != nil {
		if wait.Interrupted(err) {
			err = fmt.Errorf("exhaust all retries (%d times), failed to allocate IP from Subnet %s", steps, subnet.IPv4Subnet)
		}
		return nil, nil, err
	}

	return currentPool, ip, nil
}

func (ic *IPPoolClient) allocateIP(ctx context.Context, subnetPool *subnetPool, pod *corev1.Pod) (net.IP, *v1alpha1.IPPool, error) {
	logger := logging.FromContext(ctx)

	ippool, err := ic.GetIPPoolByName(ctx, subnetPool.pool)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get IPPool '%s' 'GetIPPoolByName()': %w", subnetPool.pool, err)
	}
	logger.Debugw("GetsubnetPoolByName()", "ippool", ippool)

	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
	if err != nil {
		return nil, nil, err
	}
	if len(unallocatedIPs) == 0 {
		return nil, nil, errNoUnallocatedIPs
	}

	allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal ippool '%s' allocatedIPs: %v", subnetPool.pool, err)
	}

	var ip net.IP

	selectIP := unallocatedIPs[0]
	ip = net.ParseIP(string(selectIP))
	unallocatedIPs = unallocatedIPs[1:]
	allocatedIPs[selectIP] = v1alpha1.PoolIPAllocation{
		NamespacedName: (apitypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}).String(),
		PodUID:         string(pod.UID),
	}

	rawUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ippool '%s' unallocatedIPs: %w", subnetPool.pool, err)
	}
	rawAllocatedIPs, err := convert.MarshalIPPoolAllocatedIPs(allocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ippool '%s' allocatedIPs: %w", subnetPool.pool, err)
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

func (ic *IPPoolClient) next(subnet string) (*subnetPool, bool) {
	ic.mutex.Lock()
	defer ic.mutex.Unlock()

	lb, ok := ic.subnetPools[subnet]
	if !ok || len(lb.subnetPools) == 0 {
		return nil, false
	}

	if lb.index >= len(lb.subnetPools) {
		lb.index = 0
	}

	ippool := lb.subnetPools[lb.index]
	lb.index = (lb.index + 1) % len(lb.subnetPools)

	ic.subnetPools[subnet] = lb

	return ippool, true
}
