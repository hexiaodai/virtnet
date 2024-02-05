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
	"reflect"
	"sort"
	"strconv"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	instance := &v1alpha1.IPPool{}
	if err := reconciler.Client.Get(ctx, request.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("resource not found. ignoring since object must be deleted")
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, apierrors.NewInternalError(fmt.Errorf("failed to get IPPool, resource is '%v': %w", appNamespacedName.String(), err))
		}
	}

	if err := reconciler.updateStatusWithSpec(logging.IntoContext(ctx, logger), instance); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	if err := reconciler.updateStatusWithAnnoSharedIn(logging.IntoContext(ctx, logger), instance); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	if err := reconciler.expansionIPPoolWithOwner(logging.IntoContext(ctx, logger), instance); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// type reconcile int

// const (
// 	reconcileDefault reconcile = iota
// 	reconcileSharedIn
// 	reconcileExpansionIPPool
// 	reconcileShrinkIPPool
// )

// func (reconciler *IPPoolReconciler) reconcile(ctx context.Context, ippool *v1alpha1.IPPool) reconcile {
// 	if ippool.Annotations == nil {
// 		return reconcileDefault
// 	}

// 	if ippool.Annotations != nil && len(ippool.Annotations[constant.IPPoolAnnoSharedIn]) > 0 {
// 		return reconcileSharedIn
// 	}

// 	if isIPPoolOwner(ippool) {
// 		ippools := v1alpha1.IPPoolList{}
// 		reconciler.Cache.List(ctx, &ippools, &client.ListOptions{
// 			Namespace:     ippool.Namespace,
// 			LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolAnnoOwner: ippool.Name}),
// 		})
// 		count, err := strconv.Atoi(ippool.Annotations[constant.IPPoolAnnoCount])
// 		if err != nil {
// 			return reconcileDefault
// 		}
// 		if len(ippools.Items) < count {
// 			return reconcileExpansionIPPool
// 		} else if len(ippools.Items) > count {
// 			return reconcileShrinkIPPool
// 		}
// 	}

// 	return reconcileDefault
// }

func (reconciler *IPPoolReconciler) updateStatusWithSpec(ctx context.Context, ippool *v1alpha1.IPPool) error {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
		Namespace:        ippool.Namespace,
		Name:             ippool.Name,
	}

	if !isIPPoolOwner(ippool) || reflect.DeepEqual(ippool.Spec, v1alpha1.IPPoolSpec{}) {
		return nil
	}

	allIPs, err := ip.ParseIPRanges(constant.IPv4, ippool.Spec.IPs)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to parse IP ranges '%v', resource is '%v': %w", ippool.Spec.IPs, appNamespacedName.String(), err))
	}

	allocatedIPs := v1alpha1.PoolIPAllocations{}
	if ippool.Status.AllocatedIPs != nil && len(ptr.OrEmpty(ippool.Status.AllocatedIPs)) > 0 {
		res, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
		if err != nil {
			return apierrors.NewInternalError(fmt.Errorf("failed to unmarshal allocated IPs '%v', resource is '%v': %w", ptr.OrEmpty(ippool.Status.AllocatedIPs), appNamespacedName.String(), err))
		}
		allocatedIPs = res
	}

	unallocatedIPs := v1alpha1.PoolIPUnallocation{}
	for _, ip := range allIPs {
		if _, ok := allocatedIPs[ip.String()]; ok {
			continue
		}
		unallocatedIPs = append(unallocatedIPs, ip.String())
	}

	if len(unallocatedIPs) > 0 {
		newUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
		if err != nil {
			return apierrors.NewInternalError(fmt.Errorf("failed to marshal unallocated IPs '%v', resource is '%v': %w", unallocatedIPs, appNamespacedName.String(), err))
		}
		ippool.Status.UnallocatedIPs = newUnallocatedIPs
	}

	ippool.Status.TotalIPCount = ptr.Of(int64(len(ippool.Spec.IPs)))
	ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
	ippool.Status.AllocatedIPCount = ptr.Of(int64(len(allocatedIPs)))

	if err := reconciler.Client.Status().Update(ctx, ippool); err != nil {
		if apierrors.IsConflict(err) {
			return err
		}
		return apierrors.NewInternalError(fmt.Errorf("failed to update IPPool status '%v': %w", appNamespacedName.String(), err))
	}
	return nil
}

func (reconciler *IPPoolReconciler) updateStatusWithAnnoSharedIn(ctx context.Context, ippool *v1alpha1.IPPool) error {
	if ippool.Annotations == nil {
		return nil
	}

	annoSharedIn := ippool.Annotations[constant.IPPoolAnnoSharedIn]
	if len(annoSharedIn) == 0 {
		return nil
	}

	sharedInUnallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(&annoSharedIn)
	if err != nil {
		return err
	}
	delete(ippool.Annotations, constant.IPPoolAnnoSharedIn)

	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
	if err != nil {
		return err
	}
	unallocatedIPs = append(unallocatedIPs, sharedInUnallocatedIPs...)

	rawUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
	if err != nil {
		return err
	}
	ippool.Status.UnallocatedIPs = rawUnallocatedIPs

	// FIXME:
	if err := reconciler.Client.Status().Update(ctx, ippool); err != nil {
		return err
	}

	return reconciler.Client.Update(ctx, ippool)
}

func (reconciler *IPPoolReconciler) expansionIPPoolWithOwner(ctx context.Context, owner *v1alpha1.IPPool) error {
	if !isIPPoolOwner(owner) {
		return nil
	}

	if owner.Annotations != nil && len(owner.Annotations[constant.IPPoolAnnoSharedOut]) > 0 {
		return nil
	}

	ippools := v1alpha1.IPPoolList{}
	reconciler.Cache.List(ctx, &ippools, &client.ListOptions{
		Namespace:     owner.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolLabelOwner: owner.Name}),
	})

	labCount := owner.Labels[constant.IPPoolLabelCount]
	if len(labCount) == 0 {
		return nil
	}
	count, err := strconv.Atoi(labCount)
	if err != nil {
		return err
	}

	if len(ippools.Items)+1 >= count {
		return nil
	}

	sharedOutIPPool, err := reconciler.getOrCreateAnnoSharedOutIPPool(ctx, owner)
	if err != nil {
		return err
	}
	ippoolAnnoSharedOut := sharedOutIPPool.Annotations[constant.IPPoolAnnoSharedOut]

	sharedInIPPool := &v1alpha1.IPPool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    owner.Namespace,
			GenerateName: fmt.Sprintf("%v-", owner.Name),
			Labels: map[string]string{
				constant.IPPoolLabelOwner: owner.Name,
			},
			Annotations: map[string]string{
				constant.IPPoolAnnoSharedIn: ippoolAnnoSharedOut,
			},
		},
	}
	if err := ctrl.SetControllerReference(owner, sharedInIPPool, reconciler.Scheme); err != nil {
		return err
	}
	if err := reconciler.Client.Create(ctx, sharedInIPPool); err != nil {
		// TODO: Requeue?
		return err
	}
	return nil
}

func (reconcile *IPPoolReconciler) shrinkIPPoolWithOwner(ctx context.Context, owner *v1alpha1.IPPool) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (reconciler *IPPoolReconciler) getOrCreateAnnoSharedOutIPPool(ctx context.Context, owner *v1alpha1.IPPool) (*v1alpha1.IPPool, error) {
	ippools := v1alpha1.IPPoolList{}
	reconciler.Cache.List(ctx, &ippools, &client.ListOptions{
		Namespace:     owner.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolLabelOwner: owner.Name}),
	})
	ippools.Items = append(ippools.Items, *owner)

	for _, ipPool := range ippools.Items {
		if ipPool.Annotations == nil {
			continue
		}
		if annoValue, ok := ipPool.Annotations[constant.IPPoolAnnoSharedOut]; ok && len(annoValue) > 0 {
			return &ipPool, nil
		}
	}

	// 从大到小排序
	sort.Sort(sortByUnallocatedIPsCount(ippools.Items))

	// 选出 unallocatedIPs 数量最多的 IPPool
	selectIPPool := ippools.Items[0]
	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(selectIPPool.Status.UnallocatedIPs)
	if err != nil {
		return nil, err
	}
	if selectIPPool.Annotations == nil {
		selectIPPool.Annotations = make(map[string]string)
	}

	// 平分 unallocatedIPs, 一半给新的 IPPool, 一半留给自己
	mid := len(unallocatedIPs) / 2
	sharedUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs[mid:])
	if err != nil {
		return nil, err
	}
	selectIPPool.Annotations[constant.IPPoolAnnoSharedOut] = ptr.OrEmpty(sharedUnallocatedIPs)

	newUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs[:mid])
	if err != nil {
		return nil, err
	}
	selectIPPool.Status.UnallocatedIPs = newUnallocatedIPs
	if err := reconciler.Client.Update(ctx, &selectIPPool); err != nil {
		// if err := reconciler.Client.Status().Update(ctx, &selectIPPool); err != nil {
		return nil, err
	}

	return &selectIPPool, nil
}

// 计算平均值
// var unallocatedIPsCount int
// nameUnallocatedIPs := map[string]v1alpha1.PoolIPUnallocation{}
// for _, ipPool := range ownedIPPools.Items {
// 	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ipPool.Status.UnallocatedIPs)
// 	if err != nil {
// 		return ctrl.Result{}, err
// 	}
// 	nameUnallocatedIPs[ipPool.Name] = unallocatedIPs
// 	unallocatedIPsCount += len(unallocatedIPs)
// }
// average := unallocatedIPsCount / len(ownedIPPools.Items)

// func (reconciler *IPPoolReconciler) reconcileOwnedCount(ctx context.Context, owner *v1alpha1.IPPool) (ctrl.Result, error) {
// 	// 1. 协调 owned 数量
// 	// 1-1. 清理多余的 owned
// 	// 1-2. 创建新的 owned
// 	// 2. 协调 UnallocatedIP、AllocatedIP

// 	if owner.Annotations == nil {
// 		return ctrl.Result{}, nil
// 	}

// 	// count, err := strconv.Atoi(owner.Annotations[constant.IPPoolAnnoCount])
// 	// if err != nil {
// 	// 	return ctrl.Result{}, err
// 	// }

// 	ownedIPPools := v1alpha1.IPPoolList{}
// 	reconciler.Client.List(ctx, &ownedIPPools, &client.ListOptions{
// 		Namespace:     owner.Namespace,
// 		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolAnnoOwner: owner.Name}),
// 	})

// 	var unallocatedIPsCount int
// 	for _, ipPool := range ownedIPPools.Items {
// 		unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ipPool.Status.UnallocatedIPs)
// 		if err != nil {
// 			return ctrl.Result{}, err
// 		}
// 		unallocatedIPsCount += len(unallocatedIPs)
// 	}
// 	// average := unallocatedIPsCount / len(ownedIPPools.Items)

// 	return ctrl.Result{}, nil
// }

// // 有一组数据：[60, 55, 30, 80, 10, 0, 150, 50, 50, 60, 30, 20]，和是 600，平均数是 50。我想新增一条数据，使这条数据的大小是这组数据的平均数，并且保持总数不变（注意，需要尽可能少修改这种数据中的元素）。

// func (reconciler *IPPoolReconciler) reconcileAllocatedIPs(ctx context.Context, owner *v1alpha1.IPPool) (ctrl.Result, error) {
// 	allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(owner.Status.AllocatedIPs)
// 	if err != nil {
// 		return ctrl.Result{}, err
// 	}

// 	ownedIPPools := v1alpha1.IPPoolList{}
// 	reconciler.Client.List(ctx, &ownedIPPools, &client.ListOptions{
// 		Namespace:     owner.Namespace,
// 		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolAnnoOwner: owner.Name}),
// 	})

// 	for _, ippool := range ownedIPPools.Items {
// 		ais, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
// 		if err != nil {
// 			return ctrl.Result{}, err
// 		}
// 		for key, value := range ais {
// 			allocatedIPs[key] = value
// 		}
// 	}

// 	chunkAllocatedIPs := chunkAllocatedIPs(allocatedIPs, len(ownedIPPools.Items)+1)

// 	// FIXME: AllocatedIPs 应该更新到 AllocatedIPs 等待区
// 	if res, err := convert.MarshalIPPoolAllocatedIPs(chunkAllocatedIPs[0]); err != nil {
// 		return ctrl.Result{}, err
// 	} else {
// 		owner.Status.AllocatedIPs = res
// 	}

// 	for idx, item := range ownedIPPools.Items {
// 		if res, err := convert.MarshalIPPoolAllocatedIPs(chunkAllocatedIPs[idx+1]); err != nil {
// 			return ctrl.Result{}, err
// 		} else {
// 			item.Status.AllocatedIPs = res
// 		}
// 	}

// 	eg, ctx := errgroup.WithContext(context.Background())
// 	eg.SetLimit(100)
// 	eg.Go(func() error {
// 		return retry.RetryOnConflictWithContext(ctx, retry.RetryMaxSteps, func(ctx context.Context) error {
// 			return reconciler.Client.Status().Update(ctx, owner)
// 		})
// 	})
// 	for _, ippool := range ownedIPPools.Items {
// 		copyIPPool := ippool
// 		eg.Go(func() error {
// 			return retry.RetryOnConflictWithContext(ctx, retry.RetryMaxSteps, func(ctx context.Context) error {
// 				return reconciler.Client.Status().Update(ctx, &copyIPPool)
// 			})
// 		})
// 	}

// 	return ctrl.Result{}, nil
// }

// func (reconciler *IPPoolReconciler) reconcileUnallocatedIPs(ctx context.Context, owner *v1alpha1.IPPool) (ctrl.Result, error) {
// 	return ctrl.Result{}, nil
// }

// func (reconciler *IPPoolReconciler) createDefaultOwned(ctx context.Context, owner *v1alpha1.IPPool) (ctrl.Result, types.IPPoolAnnoOwnedValue, error) {
// 	owned, err := convert.UnmarshalIPPoolAnnoOwned(owner.Annotations[constant.IPPoolAnnoOwned])
// 	if err != nil {
// 		return ctrl.Result{}, nil, fmt.Errorf("failed to unmarshal IPPoolAnnoOwned: %v", err.Error())
// 	}

// 	ownedIndex := map[string]struct{}{}
// 	for _, name := range owned {
// 		ownedIndex[name] = struct{}{}
// 	}

// 	accidentalIPPools := v1alpha1.IPPoolList{}
// 	reconciler.Client.List(ctx, &accidentalIPPools, &client.ListOptions{
// 		Namespace:     owner.Namespace,
// 		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolAnnoOwner: owner.Name}),
// 	})
// 	accidentalOwned := []v1alpha1.IPPool{}
// 	for _, item := range accidentalIPPools.Items {
// 		if _, ok := ownedIndex[item.Name]; !ok {
// 			accidentalOwned = append(accidentalOwned, item)
// 		}
// 	}
// 	ownedCount := 111
// 	if len(owned) >= ownedCount {
// 		for _, item := range accidentalOwned {
// 			if err := reconciler.Client.Delete(ctx, &item); err != nil && !apierrors.IsNotFound(err) {
// 				return ctrl.Result{Requeue: true}, nil, fmt.Errorf("failed to delete UnknownIPPool '%v': %w", item.Name, err)
// 			}
// 		}
// 		return ctrl.Result{}, owned, nil
// 	}

// 	for _, item := range accidentalOwned {
// 		// FIXME: SetControllerReference
// 		owned = append(owned, item.Name)
// 	}

// 	count := ownedCount - len(owned)
// 	for i := 0; i < count; i++ {
// 		ippool := &v1alpha1.IPPool{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Namespace:    owner.Namespace,
// 				GenerateName: fmt.Sprintf("%v-small-pool", owner.Name),
// 				Annotations: map[string]string{
// 					constant.IPPoolAnnoOwner: owner.Name,
// 				},
// 			},
// 		}
// 		if err := ctrl.SetControllerReference(owner, ippool, reconciler.Scheme); err != nil {
// 			return ctrl.Result{}, nil, err
// 		}
// 		if err := reconciler.Client.Create(ctx, ippool); err != nil {
// 			return ctrl.Result{Requeue: true}, nil, err
// 		}

// 		if owner.Annotations == nil {
// 			owner.Annotations = map[string]string{}
// 		}
// 		owned = append(owned, ippool.Name)
// 	}
// 	return ctrl.Result{}, owned, nil
// }

// func (reconciler *IPPoolReconciler) createDefaultOwned(ctx context.Context, owner *v1alpha1.IPPool) (types.IPPoolAnnoOwnedValue, error) {
// 	owned, err := convert.UnmarshalIPPoolAnnoOwned(owner.Annotations[constant.IPPoolAnnoOwned])
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal IPPoolAnnoOwned: %v", err.Error())
// 	}

// 	ownedIndex := map[string]struct{}{}
// 	for _, name := range owned {
// 		ownedIndex[name] = struct{}{}
// 	}

// 	ownedIPPools := v1alpha1.IPPoolList{}
// 	reconciler.Client.List(ctx, &ownedIPPools, &client.ListOptions{
// 		Namespace:     owner.Namespace,
// 		LabelSelector: labels.SelectorFromSet(map[string]string{constant.IPPoolAnnoOwner: owner.Name}),
// 	})
// 	ownedUnknown := []v1alpha1.IPPool{}
// 	for _, item := range ownedIPPools.Items {
// 		if _, ok := ownedIndex[item.Name]; !ok {
// 			ownedUnknown = append(ownedUnknown, item)
// 		}
// 	}

// 	// allocatedIPs := v1alpha1.PoolIPAllocations{}
// 	// if owner.Status.AllocatedIPs != nil && len(ptr.OrEmpty(owner.Status.AllocatedIPs)) > 0 {
// 	// 	res, err := convert.UnmarshalIPPoolAllocatedIPs(owner.Status.AllocatedIPs)
// 	// 	if err != nil {
// 	// 		return nil, fmt.Errorf("failed to unmarshal IPPoolAllocatedIPs: %v", err.Error())
// 	// 	}
// 	// 	allocatedIPs = res
// 	// }

// 	allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(owner.Status.AllocatedIPs)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal IPPoolAllocatedIPs: %v", err.Error())
// 	}
// 	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(owner.Status.UnallocatedIPs)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if len(owned) >= ownedCount {
// 		for _, item := range ownedUnknown {
// 			if err := reconciler.Client.Delete(ctx, &item); err != nil && !apierrors.IsNotFound(err) {
// 				return nil, fmt.Errorf("failed to delete UnknownIPPool '%v': %w", item.Name, err)
// 			}
// 			// FIXME:
// 			deleteAllocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(item.Status.AllocatedIPs)
// 			if err != nil {
// 				return nil, fmt.Errorf("failed to unmarshal IPPoolAllocatedIPs: %v", err.Error())
// 			}
// 			for ip, puid := range deleteAllocatedIPs {
// 				allocatedIPs[ip] = puid
// 			}
// 			deleteUnallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(item.Status.UnallocatedIPs)
// 			if err != nil {
// 				return nil, fmt.Errorf("failed to unmarshal IPPoolUnallocatedIPs: %v", err.Error())
// 			}
// 			unallocatedIPs = append(unallocatedIPs, deleteUnallocatedIPs...)
// 		}
// 		return owned, nil
// 	}

// 	for _, item := range ownedUnknown {
// 		// FIXME: SetControllerReference
// 		owned = append(owned, item.Name)
// 	}

// 	count := ownedCount - len(owned)
// 	for i := 0; i < count; i++ {
// 		ippool := &v1alpha1.IPPool{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Namespace:    owner.Namespace,
// 				GenerateName: fmt.Sprintf("%v-small-pool", owner.Name),
// 				Annotations: map[string]string{
// 					constant.IPPoolAnnoOwner: owner.Name,
// 				},
// 			},
// 		}
// 		if err := ctrl.SetControllerReference(owner, ippool, reconciler.Scheme); err != nil {
// 			return nil, err
// 		}
// 		if err := reconciler.Client.Create(ctx, ippool); err != nil {
// 			return nil, err
// 		}

// 		if owner.Annotations == nil {
// 			owner.Annotations = map[string]string{}
// 		}
// 		owned = append(owned, ippool.Name)
// 	}
// 	return owned, nil
// }

func (reconciler *IPPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if reconcilerLogger == nil {
		reconcilerLogger = logger.Named("Reconclier")
	}
	reconcilerLogger.Debug("Setting up reconciler with manager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.IPPool{}).
		Complete(reconciler)
}
