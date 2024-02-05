package ippool

import (
	"cni/pkg/constant"
	"cni/pkg/ip"
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"
	"cni/pkg/ptr"
	"cni/pkg/types"
	"cni/pkg/utils/convert"
	"context"
	"fmt"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var reconcilerLogger *zap.SugaredLogger

type IPPoolReconciler struct {
	Client client.Client
	Cache  cache.Cache
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

	allIPs, err := ip.ParseIPRanges(constant.IPv4, instance.Spec.IPs)
	if err != nil {
		return ctrl.Result{}, apierrors.NewInternalError(fmt.Errorf("failed to parse IP ranges '%v', resource is '%v': %w", instance.Spec.IPs, appNamespacedName.String(), err))
	}

	allocatedIPs := v1alpha1.PoolIPAllocations{}
	if instance.Status.AllocatedIPs != nil && len(ptr.OrEmpty(instance.Status.AllocatedIPs)) > 0 {
		res, err := convert.UnmarshalIPPoolAllocatedIPs(instance.Status.AllocatedIPs)
		if err != nil {
			return ctrl.Result{}, apierrors.NewInternalError(fmt.Errorf("failed to unmarshal allocated IPs '%v', resource is '%v': %w", ptr.OrEmpty(instance.Status.AllocatedIPs), appNamespacedName.String(), err))
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
			return ctrl.Result{}, apierrors.NewInternalError(fmt.Errorf("failed to marshal unallocated IPs '%v', resource is '%v': %w", unallocatedIPs, appNamespacedName.String(), err))
		}
		instance.Status.UnallocatedIPs = newUnallocatedIPs
	}

	instance.Status.TotalIPCount = ptr.Of(int64(len(instance.Spec.IPs)))
	instance.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
	instance.Status.AllocatedIPCount = ptr.Of(int64(len(allocatedIPs)))

	if err := reconciler.Client.Status().Update(ctx, instance); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, apierrors.NewInternalError(fmt.Errorf("failed to update IPPool status '%v': %w", appNamespacedName.String(), err))
	}

	return ctrl.Result{}, nil
}

func (reconciler *IPPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if reconcilerLogger == nil {
		reconcilerLogger = logger.Named("Reconclier")
	}
	reconcilerLogger.Debug("Setting up reconciler with manager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.IPPool{}).
		Complete(reconciler)
}
