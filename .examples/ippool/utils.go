package ippool

import (
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func haveIPPoolOwner(ipPool *v1alpha1.IPPool) *metav1.OwnerReference {
	owners := ipPool.GetOwnerReferences()
	var ippoolOwner *metav1.OwnerReference
	for _, owner := range owners {
		if v1alpha1.IPPoolGVK.Kind == owner.Kind && v1alpha1.IPPoolGVK.GroupVersion().String() == owner.APIVersion {
			if ippoolOwner != nil {
				return nil
			}
			ippoolOwner = &owner
		}
	}
	return ippoolOwner
}

func isIPPoolOwner(ipPool *v1alpha1.IPPool) bool {
	owners := ipPool.GetOwnerReferences()
	for _, owner := range owners {
		if v1alpha1.IPPoolGVK.Kind == owner.Kind && v1alpha1.IPPoolGVK.GroupVersion().String() == owner.APIVersion {
			return false
		}
	}
	return true
}

func chunkAllocatedIPs(allocations v1alpha1.PoolIPAllocations, n int) []v1alpha1.PoolIPAllocations {
	var dividedAllocations []v1alpha1.PoolIPAllocations

	// 转换 allocations 到切片
	allocationsSlice := make([]v1alpha1.PoolIPAllocation, 0, len(allocations))
	for _, val := range allocations {
		allocationsSlice = append(allocationsSlice, val)
	}

	chunkSize := (len(allocationsSlice) + n - 1) / n // 确保向上取整

	// 分割 allocations 切片
	for i := 0; i < len(allocationsSlice); i += chunkSize {
		end := i + chunkSize
		if end > len(allocationsSlice) {
			end = len(allocationsSlice)
		}

		// 将分割后的切片转换回 map[string]PoolIPAllocation 格式
		divided := make(map[string]v1alpha1.PoolIPAllocation)
		for _, allocation := range allocationsSlice[i:end] {
			divided[allocation.NamespacedName] = allocation
		}

		dividedAllocations = append(dividedAllocations, divided)
	}

	return dividedAllocations
}

// func createOwned(ctx context.Context, owner *v1alpha1.IPPool, scheme *runtime.Scheme) {
// 	ippool := &v1alpha1.IPPool{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Namespace:    owner.Namespace,
// 			GenerateName: fmt.Sprintf("%v-small-pool", owner.Name),
// 		},
// 	}
// 	if err := ctrl.SetControllerReference(owner, ippool, scheme); err != nil {

// 	}
// }

// func assignOwnedWeight(ctx context.Context, apiClient client.Client, namespace string, owned types.IPPoolAnnoOwnedValue) types.IPPoolAnnoOwnedValue {
// 	usedWeight := 0
// 	for _, v := range owned {
// 		usedWeight += v.Weight
// 	}

// 	ret := types.IPPoolAnnoOwnedValue{}

// 	if usedWeight == 0 {
// 		count := len(owned)
// 		if count > 10 {
// 			count = 10
// 		}
// 		for _, v := range owned {
// 			ret = append(ret, types.IPPoolAnnoOwnedItem{
// 				Name:   v.Name,
// 				Weight: 100 / count,
// 			})
// 		}
// 		return ret
// 	}

// 	ippools := map[string]v1alpha1.IPPool{}
// 	ippoolsLock := sync.Mutex{}

// 	eg, ctx := errgroup.WithContext(ctx)
// 	eg.SetLimit(len(owned))
// 	for _, value := range owned {
// 		copyValue := value
// 		eg.Go(func() error {
// 			select {
// 			case <-ctx.Done():
// 				logger.Warnw("context canceled", "IPPool", copyValue.Name)
// 				return nil
// 			default:
// 			}

// 			ippool := v1alpha1.IPPool{}
// 			if err := apiClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: copyValue.Name}, &ippool); err != nil {
// 				return err
// 			}
// 			ippoolsLock.Lock()
// 			ippools[copyValue.Name] = ippool
// 			ippoolsLock.Unlock()
// 			return nil
// 		})
// 	}
// }
