package ippool

import (
	"cni/pkg/constant"
	"cni/pkg/ip"
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"
	"cni/pkg/logging"
	"cni/pkg/ptr"
	"cni/pkg/types"
	"cni/pkg/utils/convert"
	"context"
	"fmt"
	"sort"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var reconcilerLogger *zap.SugaredLogger

type IPPoolReconciler struct {
	Client client.Client
	Cache  cache.Cache
	Scheme *runtime.Scheme
}

func (reconciler *IPPoolReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
		Namespace:        request.Namespace,
		Name:             request.Name,
	}

	logger := reconcilerLogger.Named("Reconcile").With("Application", appNamespacedName)
	logger.Debug("Start Reconcile")

	fmt.Println("======", request.Name)

	instance := &v1alpha1.IPPool{}
	if err := reconciler.Client.Get(ctx, request.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("resource not found. ignoring since object must be deleted")
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, apierrors.NewInternalError(fmt.Errorf("failed to get IPPool, resource is '%v': %w", appNamespacedName.String(), err))
		}
	}

	if isIPPoolOwner(instance) {
		if err := reconciler.updateStatusWithSpec(logging.IntoContext(ctx, logger), instance); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	if instance.Status.SharedInUnallocatedIPs != nil {
		if err := reconciler.updateStatusWithSharedInUnallocatedIPs(ctx, instance.DeepCopy()); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if sharedIPPool, err := reconciler.getSharedIPPoolWithIPPool(ctx, instance.DeepCopy()); err != nil {
		return ctrl.Result{}, err
	} else if sharedIPPool != nil && instance.Status.SharedInUnallocatedIPs == nil {
		if err := reconciler.updateStatusWithSharedIPPool(ctx, instance.DeepCopy(), sharedIPPool); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if sharedIPPool, err := reconciler.shouldCreateNewSharedIPPool(ctx, instance.DeepCopy()); err != nil {
		return ctrl.Result{}, err
	} else if sharedIPPool != nil {
		if err := reconciler.updateStatusWithNewSharedIPPool(ctx, instance.DeepCopy(), sharedIPPool); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (reconciler *IPPoolReconciler) updateStatusWithSpec(ctx context.Context, ippool *v1alpha1.IPPool) error {
	allIPs, err := ip.ParseIPRanges(constant.IPv4, ippool.Spec.IPs)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to parse IP ranges '%v': %w", ippool.Spec.IPs, err))
	}

	allocatedIPs := v1alpha1.PoolIPAllocations{}
	if ippool.Status.AllocatedIPs != nil && len(ptr.OrEmpty(ippool.Status.AllocatedIPs)) > 0 {
		res, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
		if err != nil {
			return apierrors.NewInternalError(fmt.Errorf("failed to unmarshal allocated IPs '%v': %w", ptr.OrEmpty(ippool.Status.AllocatedIPs), err))
		}
		allocatedIPs = res
	}

	unallocatedIPs, err := reconciler.calculateOwnerUnallocatedIPs(ctx, ippool)
	if err != nil {
		return err
	}
	if len(unallocatedIPs) > 0 {
		rawUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
		if err != nil {
			return apierrors.NewInternalError(fmt.Errorf("failed to marshal unallocated IPs '%v': %w", unallocatedIPs, err))
		}
		ippool.Status.UnallocatedIPs = rawUnallocatedIPs
	}

	ippool.Status.TotalIPCount = ptr.Of(int64(len(allIPs)))
	ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
	ippool.Status.AllocatedIPCount = ptr.Of(int64(len(allocatedIPs)))

	if err := reconciler.Client.Status().Update(ctx, ippool); err != nil {
		if apierrors.IsConflict(err) {
			return err
		}
		return apierrors.NewInternalError(fmt.Errorf("failed to update IPPool status '%v': %w", ippool.Name, err))
	}
	return nil
}

func (reconciler *IPPoolReconciler) calculateOwnerUnallocatedIPs(ctx context.Context, owner *v1alpha1.IPPool) (v1alpha1.PoolIPUnallocation, error) {
	allIPs, err := ip.ParseIPRanges(constant.IPv4, owner.Spec.IPs)
	if err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to parse IP ranges '%v': %w", owner.Spec.IPs, err))
	}

	ippools := v1alpha1.IPPoolList{}
	if err := reconciler.Client.List(ctx, &ippools, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolLabelOwner: owner.Name}),
	}); err != nil {
		return nil, err
	}
	ippools.Items = append(ippools.Items, *owner.DeepCopy())

	totalAllocatedIPs := map[string]struct{}{}
	allSharedInUnallocatedIPs := map[string]struct{}{}
	allSharedOutUnallocatedIPs := map[string]struct{}{}
	totalOwnedUnallocatedIPs := map[string]struct{}{}
	for _, item := range ippools.Items {
		allocatedIPs := item.Status.AllocatedIPs
		if allocatedIPs != nil && len(ptr.OrEmpty(allocatedIPs)) > 0 {
			res, err := convert.UnmarshalIPPoolAllocatedIPs(allocatedIPs)
			if err != nil {
				return nil, apierrors.NewInternalError(fmt.Errorf("failed to unmarshal allocated IPs '%v': %w", ptr.OrEmpty(owner.Status.AllocatedIPs), err))
			}
			for ip := range res {
				totalAllocatedIPs[ip] = struct{}{}
			}
		}

		sharedInUnallocatedIPs := item.Status.SharedInUnallocatedIPs
		if sharedInUnallocatedIPs != nil {
			res, err := convert.UnmarshalIPPoolUnallocatedIPs(&sharedInUnallocatedIPs.UnallocatedIPs)
			if err != nil {
				return nil, apierrors.NewInternalError(fmt.Errorf("failed to unmarshal allocated IPs '%v': %w", sharedInUnallocatedIPs.UnallocatedIPs, err))
			}
			for _, ip := range res {
				allSharedInUnallocatedIPs[ip] = struct{}{}
			}
		}

		sharedOutUnallocatedIPs := item.Status.SharedOutUnallocatedIPs
		if sharedOutUnallocatedIPs != nil {
			res, err := convert.UnmarshalIPPoolUnallocatedIPs(&sharedOutUnallocatedIPs.UnallocatedIPs)
			if err != nil {
				return nil, apierrors.NewInternalError(fmt.Errorf("failed to unmarshal allocated IPs '%v': %w", sharedOutUnallocatedIPs.UnallocatedIPs, err))
			}
			for _, ip := range res {
				allSharedOutUnallocatedIPs[ip] = struct{}{}
			}
		}

		if item.Name != owner.Name {
			res, err := convert.UnmarshalIPPoolUnallocatedIPs(item.Status.UnallocatedIPs)
			if err != nil {
				return nil, apierrors.NewInternalError(fmt.Errorf("failed to unmarshal allocated IPs '%v': %w", item.Status.UnallocatedIPs, err))
			}
			for _, ip := range res {
				totalOwnedUnallocatedIPs[ip] = struct{}{}
			}
		}
	}

	unallocatedIPs := v1alpha1.PoolIPUnallocation{}
	for _, ip := range allIPs {
		if _, ok := totalAllocatedIPs[ip.String()]; ok {
			continue
		}
		if _, ok := allSharedInUnallocatedIPs[ip.String()]; ok {
			continue
		}
		if _, ok := allSharedOutUnallocatedIPs[ip.String()]; ok {
			continue
		}
		if _, ok := totalOwnedUnallocatedIPs[ip.String()]; ok {
			continue
		}
		unallocatedIPs = append(unallocatedIPs, ip.String())
	}

	return unallocatedIPs, nil
}

func (reconciler *IPPoolReconciler) getOwnerWithIPPoll(ctx context.Context, ippool *v1alpha1.IPPool) (*v1alpha1.IPPool, error) {
	if isIPPoolOwner(ippool) {
		return ippool.DeepCopy(), nil
	}

	ownerRef := haveIPPoolOwner(ippool)
	if ownerRef == nil {
		return nil, nil
	}

	owner := v1alpha1.IPPool{}
	if err := reconciler.Client.Get(ctx,
		client.ObjectKey{Name: ownerRef.Name, Namespace: ippool.Namespace},
		&owner); err != nil {
		return nil, err
	}

	return &owner, nil
}

func (reconciler *IPPoolReconciler) getSharedIPPoolWithIPPool(ctx context.Context, ippool *v1alpha1.IPPool) (*v1alpha1.IPPool, error) {
	owner, err := reconciler.getOwnerWithIPPoll(ctx, ippool)
	if err != nil {
		return nil, err
	} else if owner == nil {
		return nil, nil
	}

	ippools := v1alpha1.IPPoolList{}
	reconciler.Client.List(ctx, &ippools, &client.ListOptions{
		Namespace:     ippool.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolLabelOwner: owner.Name}),
	})

	// 查找是否有正在共享 UnallocatedIPs 给我
	for _, item := range ippools.Items {
		copyItem := item
		if copyItem.Status.SharedOutUnallocatedIPs != nil &&
			copyItem.Status.SharedOutUnallocatedIPs.Name == ippool.Name {
			return &copyItem, nil
		}
	}
	return nil, nil
}

func (reconciler *IPPoolReconciler) updateStatusWithSharedInUnallocatedIPs(ctx context.Context, ippool *v1alpha1.IPPool) error {
	rawSharedInUnallocatedIPs := ippool.Status.SharedInUnallocatedIPs

	sharedInIPPool := v1alpha1.IPPool{}
	if err := reconciler.Cache.Get(ctx,
		client.ObjectKey{Namespace: ippool.Namespace, Name: ippool.Status.SharedInUnallocatedIPs.Name},
		&sharedInIPPool); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil && sharedInIPPool.Status.SharedOutUnallocatedIPs != nil {
		sharedInIPPool.Status.SharedOutUnallocatedIPs = nil
		if err := reconciler.Client.Status().Update(ctx, &sharedInIPPool); err != nil {
			return err
		}
	}

	sharedInUnallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(&rawSharedInUnallocatedIPs.UnallocatedIPs)
	if err != nil {
		return err
	}

	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
	if err != nil {
		return err
	}
	unallocatedIPs = append(unallocatedIPs, sharedInUnallocatedIPs...)

	rawUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
	if err != nil {
		return err
	}

	ippool.Status.SharedInUnallocatedIPs = nil
	ippool.Status.UnallocatedIPs = rawUnallocatedIPs
	ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))

	return reconciler.Client.Status().Update(ctx, ippool)
}

func (reconciler *IPPoolReconciler) updateStatusWithSharedIPPool(ctx context.Context, ippool, sharedOutIPPool *v1alpha1.IPPool) error {
	sharedOutUnallocatedIPs := sharedOutIPPool.Status.SharedOutUnallocatedIPs
	// if sharedOutUnallocatedIPs == nil ||
	// 	sharedOutUnallocatedIPs.Name != ippool.Name ||
	// 	ippool.Status.SharedInUnallocatedIPs != nil {
	// 	return nil
	// }

	ippool.Status.SharedInUnallocatedIPs = &v1alpha1.SharedUnallocatedIPs{
		Name:           sharedOutIPPool.Name,
		UnallocatedIPs: sharedOutUnallocatedIPs.UnallocatedIPs,
	}
	if err := reconciler.Client.Status().Update(ctx, ippool); err != nil {
		return err
	}

	// FIXME: 更新失败怎么办？
	sharedOutIPPool.Status.SharedOutUnallocatedIPs = nil
	if err := reconciler.Client.Status().Update(ctx, sharedOutIPPool); err != nil {
		return err
	}

	rawIPPoolSharedInUnallocatedIPs := ippool.Status.SharedInUnallocatedIPs.UnallocatedIPs
	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(&rawIPPoolSharedInUnallocatedIPs)
	if err != nil {
		return err
	}

	ippool.Status.UnallocatedIPs = &rawIPPoolSharedInUnallocatedIPs
	ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
	ippool.Status.SharedInUnallocatedIPs = nil

	if err := reconciler.Client.Status().Update(ctx, ippool); err != nil {
		return err
	}

	return nil
}

func (reconciler *IPPoolReconciler) shouldCreateNewSharedIPPool(ctx context.Context, ippool *v1alpha1.IPPool) (*v1alpha1.IPPool, error) {
	// UnallocatedIPs 大于 1024，不需要扩容
	// FIXME: >= 1024
	if ptr.OrEmpty(ippool.Status.UnallocatedIPCount) >= 10 {
		return nil, nil
	}

	owner := &v1alpha1.IPPool{}
	if !isIPPoolOwner(ippool) {
		ownerRef := haveIPPoolOwner(ippool)
		if ownerRef == nil {
			return nil, nil
		}
		if err := reconciler.Client.Get(ctx,
			client.ObjectKey{Name: ownerRef.Name, Namespace: ippool.Namespace},
			owner); err != nil {
			return nil, err
		}
	} else {
		owner = ippool.DeepCopy()
	}

	ippools := v1alpha1.IPPoolList{}
	reconciler.Client.List(ctx, &ippools, &client.ListOptions{
		Namespace:     ippool.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolLabelOwner: owner.Name}),
	})
	ippools.Items = append(ippools.Items, *owner)

	var unallocatedIPCount int64
	for _, item := range ippools.Items {
		unallocatedIPCount += ptr.OrEmpty(item.Status.UnallocatedIPCount)
	}

	// UnallocatedIPs 大于 IPPools 的平均值, 不需要扩容
	if ptr.OrEmpty(ippool.Status.UnallocatedIPCount) >= unallocatedIPCount/int64(len(ippools.Items)) {
		return nil, nil
	}

	// 从大到小排序
	sort.Sort(sortByUnallocatedIPsCount(ippools.Items))
	firstIPPoll := ippools.Items[0]

	// 拥有最多 UnallocatedIPs 的 IPPool, 小于 1024 不需要扩容
	// FIXME: <= 1024
	if ptr.OrEmpty(firstIPPoll.Status.UnallocatedIPCount) <= 10 {
		return nil, nil
	}

	return &firstIPPoll, nil
}

func (reconciler *IPPoolReconciler) updateStatusWithNewSharedIPPool(ctx context.Context, ippool, sharedIPPool *v1alpha1.IPPool) error {
	// 接下来，开始扩容
	sharedIPPoolUnallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(sharedIPPool.Status.UnallocatedIPs)
	if err != nil {
		return err
	}

	// 平分 sharedIPPool.Status.UnallocatedIPs. 一半给新的 IPPool, 一半留给自己
	mid := len(sharedIPPoolUnallocatedIPs) / 2
	sharedIPPoolUnallocatedIPsLeft, sharedIPPoolUnallocatedIPsRight := sharedIPPoolUnallocatedIPs[mid:], sharedIPPoolUnallocatedIPs[:mid]

	rawSharedIPPoolUnallocatedIPsRight, err := convert.MarshalIPPoolUnallocatedIPs(sharedIPPoolUnallocatedIPsRight)
	if err != nil {
		return err
	}
	sharedIPPool.Status.SharedOutUnallocatedIPs = &v1alpha1.SharedUnallocatedIPs{
		Name:           ippool.Name,
		UnallocatedIPs: *rawSharedIPPoolUnallocatedIPsRight,
	}

	rawSharedIPPoolUnallocatedIPsLeft, err := convert.MarshalIPPoolUnallocatedIPs(sharedIPPoolUnallocatedIPsLeft)
	if err != nil {
		return err
	}
	sharedIPPool.Status.UnallocatedIPs = rawSharedIPPoolUnallocatedIPsLeft
	sharedIPPool.Status.UnallocatedIPCount = ptr.Of(int64(len(sharedIPPoolUnallocatedIPsLeft)))
	if err := reconciler.Client.Status().Update(ctx, sharedIPPool); err != nil {
		return err
	}

	rawSharedIPPoolSharedOutUnallocatedIPs := sharedIPPool.Status.SharedOutUnallocatedIPs
	ippool.Status.SharedInUnallocatedIPs = &v1alpha1.SharedUnallocatedIPs{
		Name:           sharedIPPool.Name,
		UnallocatedIPs: rawSharedIPPoolSharedOutUnallocatedIPs.UnallocatedIPs,
	}

	if err := reconciler.Client.Status().Update(ctx, ippool); err != nil {
		return err
	}

	sharedIPPool.Status.SharedOutUnallocatedIPs = nil
	if err := reconciler.Client.Status().Update(ctx, sharedIPPool); err != nil {
		return err
	}

	rawIPPoolSharedInUnallocatedIPs := ippool.Status.SharedInUnallocatedIPs

	ippoolSharedInUnallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(&rawIPPoolSharedInUnallocatedIPs.UnallocatedIPs)
	if err != nil {
		return err
	}

	totalIPPoolUnallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
	if err != nil {
		return err
	}
	totalIPPoolUnallocatedIPs = append(totalIPPoolUnallocatedIPs, ippoolSharedInUnallocatedIPs...)

	rawTotalUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(totalIPPoolUnallocatedIPs)
	if err != nil {
		return err
	}
	ippool.Status.UnallocatedIPs = rawTotalUnallocatedIPs
	ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(totalIPPoolUnallocatedIPs)))
	ippool.Status.SharedInUnallocatedIPs = nil

	return reconciler.Client.Status().Update(ctx, ippool)
}

func (reconciler *IPPoolReconciler) reclaimUnallocatedIPsOnIPPoolDeletion(ctx context.Context, ippool *v1alpha1.IPPool) error {
	if ptr.OrEmpty(ippool.Status.UnallocatedIPCount) == 0 &&
		ptr.OrEmpty(ippool.Status.AllocatedIPCount) == 0 &&
		ippool.Status.SharedInUnallocatedIPs == nil &&
		ippool.Status.SharedOutUnallocatedIPs == nil {
		controllerutil.RemoveFinalizer(ippool, v1alpha1.IPPoolFinalizer)
		return nil
	}

	allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
	if err != nil {
		return err
	}

	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
	if err != nil {
		return err
	}

	return nil
}

func (reconcile *IPPoolReconciler) shrinkIPPoolWithOwner(ctx context.Context, owner *v1alpha1.IPPool) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (reconciler *IPPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if reconcilerLogger == nil {
		reconcilerLogger = logger.Named("Reconclier")
	}
	reconcilerLogger.Debug("Setting up reconciler with manager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.IPPool{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{},
			predicate.LabelChangedPredicate{},
			predicate.AnnotationChangedPredicate{})).
		Complete(reconciler)
}
