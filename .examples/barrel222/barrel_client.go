package barrel

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
	"golang.org/x/sync/errgroup"
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

type BarrelClient struct {
	ClientReader client.Reader
	Client       client.Client
	Cache        ctrlcache.Cache

	poolBarrels map[string]*poolBarrels
	mutex       sync.Mutex
}

type poolBarrel struct {
	barrel string
	mutex  sync.Mutex
}

type poolBarrels struct {
	barrels []*poolBarrel
	index   int
}

func NewBarrelClient(clientReader client.Reader, client client.Client, cache ctrlcache.Cache) *BarrelClient {
	if clientLogger == nil {
		clientLogger = logger.Named("BarrelClient")
	}
	return &BarrelClient{
		poolBarrels:  map[string]*poolBarrels{},
		ClientReader: clientReader,
		Client:       client,
		Cache:        cache,
	}
}

func (bc *BarrelClient) SetupClient(ctx context.Context, clientInter clientset.Interface) error {
	clientLogger.Debug("Setting up client with clientset.Interface")

	factory := externalversions.NewSharedInformerFactory(clientInter, 0)
	barrelInformer := factory.Virtnest().V1alpha1().Barrels()

	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			barrel := obj.(*v1alpha1.Barrel)
			if barrel.Labels == nil || len(barrel.Labels[constant.IPPoolLabelOwner]) == 0 {
				clientLogger.Debugf("'%s' has no owner, skipping add", barrel.Name)
				return
			}

			ownerName := barrel.Labels[constant.IPPoolLabelOwner]
			clientLogger.Debugf("'%s' added, owner: '%s'", barrel.Name, ownerName)

			bc.mutex.Lock()
			defer bc.mutex.Unlock()
			result, ok := bc.poolBarrels[ownerName]
			newBarrel := &poolBarrel{
				barrel: barrel.Name,
			}
			if ok {
				result.barrels = append(result.barrels, newBarrel)
				bc.poolBarrels[ownerName] = result
			} else {
				bc.poolBarrels[ownerName] = &poolBarrels{
					barrels: []*poolBarrel{newBarrel},
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {
			delBarrel := obj.(*v1alpha1.Barrel)
			if delBarrel.Labels == nil || len(delBarrel.Labels[constant.IPPoolLabelOwner]) == 0 {
				clientLogger.Debugf("'%s' has no owner, skipping delete", delBarrel.Name)
				return
			}

			ownerName := delBarrel.Labels[constant.IPPoolLabelOwner]
			clientLogger.Debugf("'%s' deleted, owner: '%s'", delBarrel.Name, ownerName)

			bc.mutex.Lock()
			defer bc.mutex.Unlock()
			poolBarrels, ok := bc.poolBarrels[ownerName]
			if ok {
				newBarrelsNames := []*poolBarrel{}
				for _, pb := range poolBarrels.barrels {
					if pb.barrel == delBarrel.Name {
						continue
					}
					newBarrelsNames = append(newBarrelsNames, pb)
					// newBarrelsNames = append(newBarrelsNames, &poolBarrel{
					// 	barrel: pb.barrel,
					// })
				}
				if poolBarrels.index >= len(newBarrelsNames) {
					poolBarrels.index = 0
					// poolBarrels.index = len(newBarrelsNames) - 1
				}
				poolBarrels.barrels = newBarrelsNames
				bc.poolBarrels[ownerName] = poolBarrels
			} else {
				clientLogger.Debugf("'%s' not found in the pool, skipping delete", ownerName)
			}
		},
	}
	if _, err := barrelInformer.Informer().AddEventHandler(eventHandler); err != nil {
		return fmt.Errorf("failed to AddEventHandler, error: %v", err)
	}

	factory.Start(ctx.Done())

	return nil
}

func (bc *BarrelClient) GetBarrelByName(ctx context.Context, barrelName string) (*v1alpha1.Barrel, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.BarrelGVK,
		Name:             barrelName,
	}

	var barrel v1alpha1.Barrel
	if err := bc.ClientReader.Get(ctx, apitypes.NamespacedName{Name: barrelName}, &barrel); err != nil {
		return nil, fmt.Errorf("failed to get barrel using 'client.Client.Get()', resource is '%v': %w", appNamespacedName.String(), err)
	}

	return &barrel, nil
}

func (bc *BarrelClient) ListBarrelsFromCache(ctx context.Context, opts ...client.ListOption) (*v1alpha1.BarrelList, error) {
	var barrelList v1alpha1.BarrelList
	if err := bc.Cache.List(ctx, &barrelList, opts...); err != nil {
		return nil, fmt.Errorf("failed to get barrels 'bc.Cache.List()': %w", err)
	}

	return &barrelList, nil
}

func (bc *BarrelClient) ListBarrelsWithOwnerFromCache(ctx context.Context, owner string, opts ...client.ListOption) (*v1alpha1.BarrelList, error) {
	var barrelList v1alpha1.BarrelList
	if err := bc.Cache.List(ctx, &barrelList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{constant.IPPoolLabelOwner: owner}),
	}); err != nil {
		return nil, fmt.Errorf("failed to get barrels 'bc.Cache.List()', owner '%v': %w", owner, err)
	}

	return &barrelList, nil
}

func (bc *BarrelClient) AllocateIP(ctx context.Context, ippool types.IPAMAnnoIPPoolItem, pod *corev1.Pod) (string, net.IP, error) {
	var ip net.IP

	currentPoolName := ippool.Next()
	poolBarrel, ok := bc.next(currentPoolName)
	if !ok {
		return "", nil, fmt.Errorf("failed to get next barrel for IPPool '%s'", currentPoolName)
	}
	poolBarrel.mutex.Lock()
	defer poolBarrel.mutex.Unlock()

	backoff := retry.DefaultRetry
	steps := backoff.Steps
	err := retry.RetryOnConflictWithContext(ctx, backoff, func(ctx context.Context) error {
		appNamespacedName := types.AppNamespacedName{
			GroupVersionKind: v1alpha1.IPPoolGVK,
			Name:             currentPoolName,
		}
		logger := clientLogger.Named("AllocateIP").
			With("resource", appNamespacedName).
			With("Times", steps-backoff.Steps+1)

		logger.Debug("Re-get Barrel for IP allocation")
		newIP, newBarrel, err := bc.allocateIP(ctx, ippool, currentPoolName, poolBarrel, pod)
		if err != nil {
			if errors.Is(err, errNoUnallocatedIPs) {
				logger.Warn("No unallocated IPs in the Barrel")
				currentPoolName = ippool.Next()
				if len(currentPoolName) == 0 {
					return err
				}
				// Retry AllocateIP
				return apierrors.NewConflict(v1alpha1.IPPoolGVR.GroupResource(), currentPoolName, errors.New("no unallocated IPs in the Barrel"))
			}
			return fmt.Errorf("failed to allocate IP from IPPool '%s': %w", currentPoolName, err)
		}
		if err := bc.Client.Status().Update(ctx, newBarrel); err != nil {
			logger.With("Barrel-ResourceVersion", newBarrel.ResourceVersion).
				With("IPPool", currentPoolName).
				With("Barrel", newBarrel.Name).
				Warn("An conflict occurred when cleaning the IP allocation records of Barrel")
			return err
		}
		ip = newIP
		return nil
	})
	if err != nil {
		if wait.Interrupted(err) {
			err = fmt.Errorf("exhaust all retries (%d times), failed to allocate IP from IPPool %s", steps, currentPoolName)
		}
		return "", nil, err
	}

	return currentPoolName, ip, nil
}

func (bc *BarrelClient) allocateIP(ctx context.Context, annoIPPool types.IPAMAnnoIPPoolItem, currentPoolName string, poolBarrel *poolBarrel, pod *corev1.Pod) (net.IP, *v1alpha1.Barrel, error) {
	logger := logging.FromContext(ctx)

	barrel, err := bc.GetBarrelByName(ctx, poolBarrel.barrel)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get barrel '%s' using 'GetBarrelByName()': %w", poolBarrel.barrel, err)
	}
	logger.Debugw("GetBarrelByName()", "barrel", barrel)

	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(barrel.Status.UnallocatedIPs)
	if err != nil {
		return nil, nil, errNoUnallocatedIPs
	}
	if len(unallocatedIPs) == 0 {
		return nil, nil, fmt.Errorf("no unallocated IPs in barrel '%s'", poolBarrel.barrel)
	}

	allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(barrel.Status.AllocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal barrel '%s' allocatedIPs: %v", poolBarrel.barrel, err)
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
		return nil, nil, fmt.Errorf("failed to marshal barrel '%s' unallocatedIPs: %w", poolBarrel.barrel, err)
	}
	rawAllocatedIPs, err := convert.MarshalIPPoolAllocatedIPs(allocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal barrel '%s' allocatedIPs: %w", poolBarrel.barrel, err)
	}

	if rawUnallocatedIPs == nil || len(ptr.OrEmpty(rawUnallocatedIPs)) == 0 {
		barrel.Status.UnallocatedIPs = nil
	} else {
		barrel.Status.UnallocatedIPs = rawUnallocatedIPs
	}
	if rawAllocatedIPs == nil || len(ptr.OrEmpty(rawAllocatedIPs)) == 0 {
		barrel.Status.AllocatedIPs = nil
	} else {
		barrel.Status.AllocatedIPs = rawAllocatedIPs
	}

	barrel.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
	barrel.Status.AllocatedIPCount = ptr.Of(int64(len(allocatedIPs)))

	return ip, barrel, nil
}

func (bc *BarrelClient) ReleaseIP(ctx context.Context, podName, podNamespace, podUID string, barrelNames []string) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(32)

	// for _, barrelName := range barrelNames {
	// 	copyBarrelName := barrelName
	// 	eg.Go(func() error {
	// 		backoff := retry.DefaultRetry
	// 		steps := backoff.Steps
	// 		err := retry.RetryOnConflictWithContext(ctx, backoff, func(ctx context.Context) error {
	// 			barrel, err := bc.GetBarrelByName(ctx, copyBarrelName)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(barrel.Status.AllocatedIPs)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			return nil
	// 		})

	// 		return err
	// 	})

	// }

	return nil
}

func (bc *BarrelClient) next(poolName string) (*poolBarrel, bool) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()

	lb, ok := bc.poolBarrels[poolName]
	if !ok || len(lb.barrels) == 0 {
		return nil, false
	}

	if lb.index >= len(lb.barrels) {
		lb.index = 0
	}

	barrel := lb.barrels[lb.index]
	lb.index = (lb.index + 1) % len(lb.barrels)

	bc.poolBarrels[poolName] = lb

	return barrel, true
}
