package subnet

import (
	"context"
	"fmt"

	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/ip"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/logging"
	"github.com/hexiaodai/virtnet/pkg/ptr"
	"github.com/hexiaodai/virtnet/pkg/types"
	"github.com/hexiaodai/virtnet/pkg/utils/convert"
	"github.com/hexiaodai/virtnet/pkg/utils/retry"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var reconcilerLogger *zap.SugaredLogger

type SubnetReconciler struct {
	Client client.Client
	Cache  cache.Cache
	Scheme *runtime.Scheme
}

func (reconciler *SubnetReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.SubnetGVK,
		Namespace:        request.Namespace,
		Name:             request.Name,
	}

	logger := reconcilerLogger.Named("Reconcile").With("Application", appNamespacedName)
	logger.Debug("Start Reconcile")

	instance := &v1alpha1.Subnet{}
	if err := reconciler.Client.Get(ctx, request.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("Resource not found. ignoring since object must be deleted")
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to get Subnet: %w", err)
		}
	}

	if err := reconciler.updateOwnerTotalIPCount(ctx, instance); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to update OwnerTotalIPCount: %w", err)
	}

	if err := reconciler.replicate(ctx, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to replicate Subnet: %w", err)
	}

	if err := reconciler.releaseIPPoolUnallocatedIPs(ctx, instance); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to release IPPoolUnallocatedIPs: %w", err)
	}

	ippools, err := reconciler.ippoolsWithOwner(ctx, instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get ippoolsWithOwner: %w", err)
	}

	ippoolsUnallocatedIPs, err := reconciler.ippoolsUnallocatedIPs(ctx, ippools)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get unallocated IPs: %w", err)
	}

	ippoolsAllocatedIPs, err := reconciler.ippoolsAllocatedIPs(ctx, ippools)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get allocated IPs: %w", err)
	}

	remainingUnallocatedIPs, err := reconciler.remainingUnallocatedIPs(ctx, instance, ippoolsUnallocatedIPs, ippoolsAllocatedIPs)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get remainingUnallocated IPs: %w", err)
	}

	ippoolNeedsExpansion, err := reconciler.ippoolNeedsExpansion(ctx, ippools, ippoolsUnallocatedIPs, remainingUnallocatedIPs)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get ippoolNeedsExpansion: %w", err)
	}

	if err := reconciler.ippoolExpansion(ctx, ippoolNeedsExpansion, remainingUnallocatedIPs); err != nil {
		if apierrors.IsConflict(err) {
			logger.Debug("Conflict detected. Requeuing...")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to expand IPPools: %w", err)
	}

	return ctrl.Result{}, nil
}

func (reconciler *SubnetReconciler) updateOwnerTotalIPCount(ctx context.Context, owner *v1alpha1.Subnet) error {
	totalIPs, err := ip.ParseIPRanges(constant.IPv4, owner.Spec.IPs)
	if err != nil {
		return fmt.Errorf("failed to parse IP ranges '%v': %w", owner.Spec.IPs, err)
	}
	owner.Status.TotalIPCount = ptr.Of(int64(len(totalIPs)))
	return reconciler.Client.Status().Update(ctx, owner)
}

func (reconciler *SubnetReconciler) replicate(ctx context.Context, owner *v1alpha1.Subnet) error {
	// logger := logging.FromContext(ctx)
	ippools := v1alpha1.IPPoolList{}
	if err := reconciler.Client.List(ctx, &ippools, &client.ListOptions{
		Namespace:     owner.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.SubnetLabelOwner: owner.Name}),
	}); err != nil {
		return fmt.Errorf("failed to list IPPools: %w", err)
	}

	desiredReplicaDiff := owner.Spec.Replicas - len(ippools.Items)

	if desiredReplicaDiff == 0 {
		return nil
	}

	for i := 0; i < desiredReplicaDiff; i++ {
		bl := v1alpha1.IPPool{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: fmt.Sprintf("%v-", owner.Name),
				Labels: map[string]string{
					constant.SubnetLabelOwner: owner.Name,
				},
			},
			// FIXME: IPs
			Spec: v1alpha1.IPPoolSpec{
				Subnet:  owner.Spec.Subnet,
				Gateway: owner.Spec.Gateway,
			},
		}
		if err := ctrl.SetControllerReference(owner, &bl, reconciler.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
		if err := reconciler.Client.Create(ctx, &bl); err != nil {
			return fmt.Errorf("failed to create IPPool: %w", err)
		}
	}

	for i := 0; i < -desiredReplicaDiff; i++ {
		if err := reconciler.Client.Delete(ctx, &ippools.Items[i]); err != nil {
			return fmt.Errorf("failed to delete IPPool: %w", err)
		}
	}

	return nil
}

func (reconciler *SubnetReconciler) ippoolsWithOwner(ctx context.Context, owner *v1alpha1.Subnet) ([]v1alpha1.IPPool, error) {
	logger := logging.FromContext(ctx)
	ippools := v1alpha1.IPPoolList{}
	if err := reconciler.Client.List(ctx, &ippools, &client.ListOptions{
		Namespace:     owner.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.SubnetLabelOwner: owner.Name}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list IPPools: %w", err)
	}
	logger.Debugw("Listed IPPools", "count", len(ippools.Items), "ippools", ippools)
	return ippools.Items, nil
}

func (reconciler *SubnetReconciler) ippoolsUnallocatedIPs(ctx context.Context, ippools []v1alpha1.IPPool) (map[string]v1alpha1.PoolIPUnallocation, error) {
	logger := logging.FromContext(ctx)

	unallocatedIPs := map[string]v1alpha1.PoolIPUnallocation{}
	for _, ippool := range ippools {
		result, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
		if err != nil {
			return nil, err
		}
		unallocatedIPs[ippool.Name] = result
	}
	logger.Debugw("Unallocated IPs", "unallocatedIPs", unallocatedIPs)

	return unallocatedIPs, nil
}

func (reconciler *SubnetReconciler) ippoolsAllocatedIPs(ctx context.Context, ippools []v1alpha1.IPPool) (map[string]v1alpha1.PoolIPAllocations, error) {
	logger := logging.FromContext(ctx)

	allocatedIPs := map[string]v1alpha1.PoolIPAllocations{}
	for _, ippool := range ippools {
		result, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
		if err != nil {
			return nil, err
		}
		allocatedIPs[ippool.Name] = result
	}
	logger.Debugw("AllocatedIPs IPs", "allocatedIPs", allocatedIPs)

	return allocatedIPs, nil
}

func (reconciler *SubnetReconciler) remainingUnallocatedIPs(ctx context.Context, owner *v1alpha1.Subnet, unallocation map[string]v1alpha1.PoolIPUnallocation, allocations map[string]v1alpha1.PoolIPAllocations) (v1alpha1.PoolIPUnallocation, error) {
	logger := logging.FromContext(ctx)

	allIPs, err := ip.ParseIPRanges(constant.IPv4, owner.Spec.IPs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP ranges '%v': %w", owner.Spec.IPs, err)
	}
	logger.Debugw("ip.ParseIPRanges()", "Result", allIPs)

	indexMap := map[string]struct{}{}
	for _, ips := range unallocation {
		for _, ip := range ips {
			indexMap[ip] = struct{}{}
		}
	}
	for _, ips := range allocations {
		for ip := range ips {
			indexMap[ip] = struct{}{}
		}
	}

	remainingUnallocatedIPs := v1alpha1.PoolIPUnallocation{}
	for _, ip := range allIPs {
		if _, ok := indexMap[ip.String()]; ok {
			continue
		}
		remainingUnallocatedIPs = append(remainingUnallocatedIPs, ip.String())
	}

	return remainingUnallocatedIPs, nil
}

func (reconciler *SubnetReconciler) releaseIPPoolUnallocatedIPs(ctx context.Context, owner *v1alpha1.Subnet) error {
	ippools, err := reconciler.ippoolsWithOwner(ctx, owner)
	if err != nil {
		return err
	}

	ippoolUnallocation, err := reconciler.ippoolsUnallocatedIPs(ctx, ippools)
	if err != nil {
		return err
	}

	ippoolUnallocatedIPs := v1alpha1.PoolIPUnallocation{}
	for _, ips := range ippoolUnallocation {
		ippoolUnallocatedIPs = append(ippoolUnallocatedIPs, ips...)
	}

	for _, ippool := range ippools {
		copyIPPool := ippool
		avg := int64(len(ippoolUnallocatedIPs) / len(ippools))
		if ptr.OrEmpty(copyIPPool.Status.UnallocatedIPCount) > avg {
			ips, err := convert.UnmarshalIPPoolUnallocatedIPs(copyIPPool.Status.UnallocatedIPs)
			if err != nil {
				return err
			}
			newIPs := ips[:avg]
			rawNewIPs, err := convert.MarshalIPPoolUnallocatedIPs(newIPs)
			if err != nil {
				return err
			}
			allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(copyIPPool.Status.AllocatedIPs)
			if err != nil {
				return err
			}
			copyIPPool.Status.UnallocatedIPs = rawNewIPs
			copyIPPool.Status.UnallocatedIPCount = ptr.Of(int64(len(newIPs)))
			copyIPPool.Status.TotalIPCount = ptr.Of(int64(len(allocatedIPs) + len(newIPs)))
			if err := retry.RetryOnConflictWithContext(ctx, retry.DefaultRetry, func(ctx context.Context) error {
				return reconciler.Client.Status().Update(ctx, &copyIPPool)
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (reconciler *SubnetReconciler) ippoolNeedsExpansion(ctx context.Context, ippools []v1alpha1.IPPool, ippoolUnallocation map[string]v1alpha1.PoolIPUnallocation, remainingUnallocatedIPs v1alpha1.PoolIPUnallocation) ([]v1alpha1.IPPool, error) {
	ippoolUnallocatedIPs := v1alpha1.PoolIPUnallocation{}
	for _, ips := range ippoolUnallocation {
		ippoolUnallocatedIPs = append(ippoolUnallocatedIPs, ips...)
	}

	totalUnallocatedIPsCount := len(ippoolUnallocatedIPs) + len(remainingUnallocatedIPs)

	ippoolNeedsExpansion := []v1alpha1.IPPool{}
	for _, ippool := range ippools {
		if ptr.OrEmpty(ippool.Status.UnallocatedIPCount) < int64((totalUnallocatedIPsCount)/len(ippools)) {
			ippoolNeedsExpansion = append(ippoolNeedsExpansion, ippool)
		}
	}

	if len(ippoolNeedsExpansion) == 0 && len(remainingUnallocatedIPs) > 0 {
		return ippools, nil
	}

	return ippoolNeedsExpansion, nil
}

func (reconciler *SubnetReconciler) ippoolExpansion(ctx context.Context, ippoolNeedsExpansion []v1alpha1.IPPool, remainingUnallocatedIPs v1alpha1.PoolIPUnallocation) error {
	// logger := logging.FromContext(ctx)
	if len(ippoolNeedsExpansion) == 0 || len(remainingUnallocatedIPs) == 0 {
		return nil
	}
	ips := splitUnallocatedIPs(remainingUnallocatedIPs, len(ippoolNeedsExpansion))
	for idx, ippool := range ippoolNeedsExpansion {
		copyIPPool := ippool
		if idx >= len(ips) {
			return nil
		}
		unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
		if err != nil {
			return err
		}
		unallocatedIPs = append(unallocatedIPs, ips[idx]...)
		rawUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
		if err != nil {
			return err
		}
		allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(copyIPPool.Status.AllocatedIPs)
		if err != nil {
			return err
		}
		copyIPPool.Status.UnallocatedIPs = rawUnallocatedIPs
		copyIPPool.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
		copyIPPool.Status.TotalIPCount = ptr.Of(int64(len(allocatedIPs) + len(unallocatedIPs)))
		if err := reconciler.Client.Status().Update(ctx, &copyIPPool); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler *SubnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if reconcilerLogger == nil {
		reconcilerLogger = logger.Named("Reconclier")
	}
	reconcilerLogger.Debug("Setting up reconciler with manager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Subnet{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		// WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{},
		// 	predicate.LabelChangedPredicate{},
		// 	predicate.AnnotationChangedPredicate{})).
		Complete(reconciler)
}
