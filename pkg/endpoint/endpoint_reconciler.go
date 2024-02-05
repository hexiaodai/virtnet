package endpoint

import (
	"context"
	"fmt"

	"github.com/hexiaodai/virtnet/pkg/constant"
	cniip "github.com/hexiaodai/virtnet/pkg/ip"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/ptr"
	"github.com/hexiaodai/virtnet/pkg/types"
	"github.com/hexiaodai/virtnet/pkg/utils/convert"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var reconcilerLogger *zap.SugaredLogger

type EndpointReconciler struct {
	Client client.Client
	Cache  cache.Cache
	Scheme *runtime.Scheme
}

func (reconciler *EndpointReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.EndpointGVK,
		Namespace:        request.Namespace,
		Name:             request.Name,
	}

	logger := reconcilerLogger.Named("Reconcile").With("Application", appNamespacedName)
	logger.Debug("Start Reconcile")

	instance := v1alpha1.Endpoint{}
	if err := reconciler.Client.Get(ctx, request.NamespacedName, &instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("Resource not found. ignoring since object must be deleted")
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to get Endpoint: %w", err)
		}
	}

	if instance.DeletionTimestamp == nil {
		logger.Debug("Endpoint is not being deleted. Skipping reconcile")
		return ctrl.Result{}, nil
	}

	logger.Debug("Start release IP")

	logger.Debug("IPs that needs to be released: %+v", instance.Status.Current.IPs)
	poolNameIndex := map[string]struct{}{}
	for _, ip := range instance.Status.Current.IPs {
		poolNameIndex[ptr.OrEmpty(ip.IPv4Pool)] = struct{}{}
	}
	logger.Debug("IPPool to be operated: %+v", poolNameIndex)

	ippools := []v1alpha1.IPPool{}
	for poolName := range poolNameIndex {
		var ippool v1alpha1.IPPool
		if err := reconciler.Cache.Get(ctx, apitypes.NamespacedName{Name: poolName}, &ippool); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Debug("ippool '%v' not found. Skipping release", poolName)
				continue
			}
			return ctrl.Result{}, fmt.Errorf("failed to get ippool 'reconciler.Cache.Get()', owner '%v': %w", poolName, err)
		}
		ippools = append(ippools, ippool)
	}

	for _, ippool := range ippools {
		releasedIPs := []v1alpha1.IPAllocationDetail{}
		ippoolDeepCopy := ippool.DeepCopy()

		// AllocatedIPs
		ippoolDeepCopyAllocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(ippoolDeepCopy.Status.AllocatedIPs)
		recordIPPoolDeepCopyAllocatedIPCount := len(ippoolDeepCopyAllocatedIPs)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal AllocatedIPs with 'convert.UnmarshalIPPoolAllocatedIPs()': %w", err)
		}

		for _, ip := range instance.Status.Current.IPs {
			ipv4 := ptr.OrEmpty(ip.IPv4)
			ipNet, err := cniip.ParseIP(constant.IPv4, ipv4, true)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to parse ip '%v' with 'cniip.ParseIP()': %w", ipv4, err)
			}
			if _, ok := ippoolDeepCopyAllocatedIPs[ipNet.IP.String()]; !ok {
				continue
			}
			releasedIPs = append(releasedIPs, ip)
			delete(ippoolDeepCopyAllocatedIPs, ipNet.IP.String())
		}

		if len(ippoolDeepCopyAllocatedIPs) == recordIPPoolDeepCopyAllocatedIPCount {
			logger.Debug("No change in AllocatedIPs")
			continue
		}

		rawIPPoolDeepCopyAllocatedIPs, err := convert.MarshalIPPoolAllocatedIPs(ippoolDeepCopyAllocatedIPs)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to marshal AllocatedIPs with 'convert.MarshalIPPoolAllocatedIPs()': %w", err)
		}

		// UnallocatedIPs
		ippoolDeepCopyUnallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippoolDeepCopy.Status.UnallocatedIPs)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal UnallocatedIPs with 'convert.UnmarshalIPPoolUnallocatedIPs()': %w", err)
		}
		for _, releasedIP := range releasedIPs {
			ipv4 := ptr.OrEmpty(releasedIP.IPv4)
			ipNet, err := cniip.ParseIP(constant.IPv4, ipv4, true)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to parse ip '%v' with 'cniip.ParseIP()': %w", ipv4, err)
			}
			ippoolDeepCopyUnallocatedIPs = append(ippoolDeepCopyUnallocatedIPs, ipNet.IP.String())
		}
		rawIPPoolDeepCopyUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(ippoolDeepCopyUnallocatedIPs)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to marshal UnallocatedIPs with 'convert.MarshalIPPoolUnallocatedIPs()': %w", err)
		}

		ippool.Status.AllocatedIPs = rawIPPoolDeepCopyAllocatedIPs
		ippool.Status.UnallocatedIPs = rawIPPoolDeepCopyUnallocatedIPs
		ippool.Status.AllocatedIPCount = ptr.Of(int64(len(ippoolDeepCopyAllocatedIPs)))
		ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(ippoolDeepCopyUnallocatedIPs)))

		if err := reconciler.Client.Status().Update(ctx, &ippool); err != nil {
			if apierrors.IsConflict(err) {
				logger.Debug("Conflict occurred while updating ippool, requeuing")
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, fmt.Errorf("failed to update ippool '%v': %w", ippool.Name, err)
		}
		logger.Debugf("Released IPs: %+v", releasedIPs)
	}

	controllerutil.RemoveFinalizer(&instance, v1alpha1.IPPoolFinalizer)
	if err := reconciler.Client.Update(ctx, &instance); err != nil {
		if apierrors.IsConflict(err) {
			logger.Debug("Conflict occurred while removing finalizer, requeuing")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer from ippool '%v': %w", instance.Name, err)
	}
	logger.Debugw("Removed finalizer from endpoint", "Finalizer", v1alpha1.IPPoolFinalizer)

	return ctrl.Result{}, nil
}

func (reconciler *EndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if reconcilerLogger == nil {
		reconcilerLogger = logger.Named("Reconclier")
	}
	reconcilerLogger.Debug("Setting up reconciler with manager")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Endpoint{}).
		// The .metadata.generation value is incremented for all changes, except for changes to .metadata or .status.
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		// WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{},
		// 	predicate.LabelChangedPredicate{},
		// 	predicate.AnnotationChangedPredicate{})).
		Complete(reconciler)
}
