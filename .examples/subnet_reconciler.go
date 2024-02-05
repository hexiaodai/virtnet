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

	if err := reconciler.releaseBarrelUnallocatedIPs(ctx, instance); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to release BarrelUnallocatedIPs: %w", err)
	}

	barrels, err := reconciler.barrelsWithOwner(ctx, instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get barrelsWithOwner: %w", err)
	}

	barrelsUnallocatedIPs, err := reconciler.barrelsUnallocatedIPs(ctx, barrels)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get unallocated IPs: %w", err)
	}

	barrelsAllocatedIPs, err := reconciler.barrelsAllocatedIPs(ctx, barrels)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get allocated IPs: %w", err)
	}

	remainingUnallocatedIPs, err := reconciler.remainingUnallocatedIPs(ctx, instance, barrelsUnallocatedIPs, barrelsAllocatedIPs)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get remainingUnallocated IPs: %w", err)
	}

	barrelNeedsExpansion, err := reconciler.barrelNeedsExpansion(ctx, barrels, barrelsUnallocatedIPs, remainingUnallocatedIPs)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get barrelNeedsExpansion: %w", err)
	}

	if err := reconciler.barrelExpansion(ctx, barrelNeedsExpansion, remainingUnallocatedIPs); err != nil {
		if apierrors.IsConflict(err) {
			logger.Debug("Conflict detected. Requeuing...")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to expand Barrels: %w", err)
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
	barrels := v1alpha1.BarrelList{}
	if err := reconciler.Client.List(ctx, &barrels, &client.ListOptions{
		Namespace:     owner.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.SubnetLabelOwner: owner.Name}),
	}); err != nil {
		return fmt.Errorf("failed to list Barrels: %w", err)
	}

	desiredReplicaDiff := owner.Spec.Replicas - len(barrels.Items)

	if desiredReplicaDiff == 0 {
		return nil
	}

	for i := 0; i < desiredReplicaDiff; i++ {
		bl := v1alpha1.Barrel{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: fmt.Sprintf("%v-", owner.Name),
				Labels: map[string]string{
					constant.SubnetLabelOwner: owner.Name,
				},
			},
		}
		if err := ctrl.SetControllerReference(owner, &bl, reconciler.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
		if err := reconciler.Client.Create(ctx, &bl); err != nil {
			return fmt.Errorf("failed to create Barrel: %w", err)
		}
	}

	for i := 0; i < -desiredReplicaDiff; i++ {
		if err := reconciler.Client.Delete(ctx, &barrels.Items[i]); err != nil {
			return fmt.Errorf("failed to delete Barrel: %w", err)
		}
	}

	return nil
}

func (reconciler *SubnetReconciler) barrelsWithOwner(ctx context.Context, owner *v1alpha1.Subnet) ([]v1alpha1.Barrel, error) {
	logger := logging.FromContext(ctx)
	barrels := v1alpha1.BarrelList{}
	if err := reconciler.Client.List(ctx, &barrels, &client.ListOptions{
		Namespace:     owner.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.SubnetLabelOwner: owner.Name}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list Barrels: %w", err)
	}
	logger.Debugw("Listed Barrels", "count", len(barrels.Items), "barrels", barrels)
	return barrels.Items, nil
}

func (reconciler *SubnetReconciler) barrelsUnallocatedIPs(ctx context.Context, barrels []v1alpha1.Barrel) (map[string]v1alpha1.PoolIPUnallocation, error) {
	logger := logging.FromContext(ctx)

	unallocatedIPs := map[string]v1alpha1.PoolIPUnallocation{}
	for _, barrel := range barrels {
		result, err := convert.UnmarshalSubnetUnallocatedIPs(barrel.Status.UnallocatedIPs)
		if err != nil {
			return nil, err
		}
		unallocatedIPs[barrel.Name] = result
	}
	logger.Debugw("Unallocated IPs", "unallocatedIPs", unallocatedIPs)

	return unallocatedIPs, nil
}

func (reconciler *SubnetReconciler) barrelsAllocatedIPs(ctx context.Context, barrels []v1alpha1.Barrel) (map[string]v1alpha1.PoolIPAllocations, error) {
	logger := logging.FromContext(ctx)

	allocatedIPs := map[string]v1alpha1.PoolIPAllocations{}
	for _, barrel := range barrels {
		result, err := convert.UnmarshalSubnetAllocatedIPs(barrel.Status.AllocatedIPs)
		if err != nil {
			return nil, err
		}
		allocatedIPs[barrel.Name] = result
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

func (reconciler *SubnetReconciler) releaseBarrelUnallocatedIPs(ctx context.Context, owner *v1alpha1.Subnet) error {
	barrels, err := reconciler.barrelsWithOwner(ctx, owner)
	if err != nil {
		return err
	}

	barrelUnallocation, err := reconciler.barrelsUnallocatedIPs(ctx, barrels)
	if err != nil {
		return err
	}

	barrelUnallocatedIPs := v1alpha1.PoolIPUnallocation{}
	for _, ips := range barrelUnallocation {
		barrelUnallocatedIPs = append(barrelUnallocatedIPs, ips...)
	}

	for _, barrel := range barrels {
		copyBarrel := barrel
		avg := int64(len(barrelUnallocatedIPs) / len(barrels))
		if ptr.OrEmpty(copyBarrel.Status.UnallocatedIPCount) > avg {
			ips, err := convert.UnmarshalSubnetUnallocatedIPs(copyBarrel.Status.UnallocatedIPs)
			if err != nil {
				return err
			}
			newIPs := ips[:avg]
			rawNewIPs, err := convert.MarshalSubnetUnallocatedIPs(newIPs)
			if err != nil {
				return err
			}
			allocatedIPs, err := convert.UnmarshalSubnetAllocatedIPs(copyBarrel.Status.AllocatedIPs)
			if err != nil {
				return err
			}
			copyBarrel.Status.UnallocatedIPs = rawNewIPs
			copyBarrel.Status.UnallocatedIPCount = ptr.Of(int64(len(newIPs)))
			copyBarrel.Status.TotalIPCount = ptr.Of(int64(len(allocatedIPs) + len(newIPs)))
			if err := retry.RetryOnConflictWithContext(ctx, retry.DefaultRetry, func(ctx context.Context) error {
				return reconciler.Client.Status().Update(ctx, &copyBarrel)
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (reconciler *SubnetReconciler) barrelNeedsExpansion(ctx context.Context, barrels []v1alpha1.Barrel, barrelUnallocation map[string]v1alpha1.PoolIPUnallocation, remainingUnallocatedIPs v1alpha1.PoolIPUnallocation) ([]v1alpha1.Barrel, error) {
	barrelUnallocatedIPs := v1alpha1.PoolIPUnallocation{}
	for _, ips := range barrelUnallocation {
		barrelUnallocatedIPs = append(barrelUnallocatedIPs, ips...)
	}

	totalUnallocatedIPsCount := len(barrelUnallocatedIPs) + len(remainingUnallocatedIPs)

	barrelNeedsExpansion := []v1alpha1.Barrel{}
	for _, barrel := range barrels {
		if ptr.OrEmpty(barrel.Status.UnallocatedIPCount) < int64((totalUnallocatedIPsCount)/len(barrels)) {
			barrelNeedsExpansion = append(barrelNeedsExpansion, barrel)
		}
	}

	if len(barrelNeedsExpansion) == 0 && len(remainingUnallocatedIPs) > 0 {
		return barrels, nil
	}

	return barrelNeedsExpansion, nil
}

func (reconciler *SubnetReconciler) barrelExpansion(ctx context.Context, barrelNeedsExpansion []v1alpha1.Barrel, remainingUnallocatedIPs v1alpha1.PoolIPUnallocation) error {
	// logger := logging.FromContext(ctx)
	if len(barrelNeedsExpansion) == 0 || len(remainingUnallocatedIPs) == 0 {
		return nil
	}
	ips := splitUnallocatedIPs(remainingUnallocatedIPs, len(barrelNeedsExpansion))
	for idx, barrel := range barrelNeedsExpansion {
		copyBarrel := barrel
		if idx >= len(ips) {
			return nil
		}
		unallocatedIPs, err := convert.UnmarshalSubnetUnallocatedIPs(barrel.Status.UnallocatedIPs)
		if err != nil {
			return err
		}
		unallocatedIPs = append(unallocatedIPs, ips[idx]...)
		rawUnallocatedIPs, err := convert.MarshalSubnetUnallocatedIPs(unallocatedIPs)
		if err != nil {
			return err
		}
		allocatedIPs, err := convert.UnmarshalSubnetAllocatedIPs(copyBarrel.Status.AllocatedIPs)
		if err != nil {
			return err
		}
		copyBarrel.Status.UnallocatedIPs = rawUnallocatedIPs
		copyBarrel.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
		copyBarrel.Status.TotalIPCount = ptr.Of(int64(len(allocatedIPs) + len(unallocatedIPs)))
		if err := reconciler.Client.Status().Update(ctx, &copyBarrel); err != nil {
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
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{},
			predicate.LabelChangedPredicate{},
			predicate.AnnotationChangedPredicate{})).
		Complete(reconciler)
}
