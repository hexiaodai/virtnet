package ippool

import (
	"cni/pkg/constant"
	cniip "cni/pkg/ip"
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"
	"cni/pkg/logging"
	"cni/pkg/utils/convert"
	"cni/pkg/utils/retry"
	"context"
	"fmt"

	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/wait"

	// "k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (webhook *IPPoolWebhook) mutateIPPool(ctx context.Context, ipPool *v1alpha1.IPPool) *field.Error {
	logger := logging.FromContext(ctx)
	logger.Named("mutateIPPool")
	logger.Debug("Start to mutate IPPool")

	if ipPool.DeletionTimestamp != nil {
		logger.Debug("Terminating IPPool, noting to mutate")
		return nil
	}

	if !controllerutil.ContainsFinalizer(ipPool, v1alpha1.IPPoolFinalizer) {
		// TODO: Add finalizer
		// finalizersUpdated := controllerutil.AddFinalizer(ipPool, v1alpha1.IPPoolFinalizer)
		// if finalizersUpdated {
		// 	logger.Debugf("Add finalizer %s to %s IPPool", ipPool.Name, v1alpha1.IPPoolFinalizer)
		// }
	}

	if ownerRef := haveIPPoolOwner(ipPool); ownerRef != nil {
		// TODO: updateOwnerIPPool
		// if err := webhook.updateOwnerIPPool(logging.IntoContext(ctx, logger), ipPool, ownerRef); err != nil {
		// 	return field.Invalid(
		// 		ownerReferencesField,
		// 		ownerRef,
		// 		fmt.Sprintf("failed to updateOwnerIPPool: %v", err.Error()),
		// 	)
		// }
		logger.Debugf("IPPool %s has owner %s, 'spec' set to nil", ipPool.Name, ownerRef.Name)
		if ipPool.Labels == nil {
			ipPool.Labels = make(map[string]string)
		}
		ipPool.Labels[constant.IPPoolLabelOwner] = ownerRef.Name
		ipPool.Spec = v1alpha1.IPPoolSpec{}
		// fmt.Println("==========", ipPool.Annotations)
		return nil
	}

	if len(ipPool.Spec.IPs) > 0 {
		mergedIPs, err := cniip.MergeIPRanges(constant.IPv4, ipPool.Spec.IPs)
		if err != nil {
			return field.Invalid(
				ipsField,
				ipPool.Spec.IPs,
				fmt.Sprintf("failed to merge '%v': %v", ipsField.String(), err),
			)
		}
		ips := ipPool.Spec.IPs
		ipPool.Spec.IPs = mergedIPs
		logger.Debugf("Merge '%v' %v to %v", ipsField.String(), ips, mergedIPs)
	}

	if len(ipPool.Spec.ExcludeIPs) > 0 {
		mergedExcludeIPs, err := cniip.MergeIPRanges(constant.IPv4, ipPool.Spec.ExcludeIPs)
		if err != nil {
			return field.Invalid(
				excludeIPsField,
				ipPool.Spec.ExcludeIPs,
				fmt.Sprintf("failed to merge '%v': %v", excludeIPsField.String(), err),
			)
		}
		excludeIPs := ipPool.Spec.ExcludeIPs
		ipPool.Spec.ExcludeIPs = mergedExcludeIPs
		logger.Debugf("Merge '%v' %v to %v", excludeIPsField.String(), excludeIPs, mergedExcludeIPs)
	}

	return nil
}

func (webhook *IPPoolWebhook) updateOwnerIPPool(ctx context.Context, ipPool *v1alpha1.IPPool, ownerRef *metav1.OwnerReference) error {
	logger := logging.FromContext(ctx)
	logger.Named("updateOwnerIPPool")

	owner := v1alpha1.IPPool{}
	err := retry.RetryOnConflictWithContext(ctx, retry.DefaultRetry, func(ctx context.Context) error {
		if err := webhook.Client.Get(ctx, client.ObjectKey{Name: ownerRef.Name, Namespace: ipPool.Namespace}, &owner); err != nil {
			return fmt.Errorf("failed to get OwnerIPPool: %v", err.Error())
		}
		if owner.Labels == nil {
			owner.Labels = make(map[string]string)
		}

		owned, err := convert.UnmarshalIPPoolAnnoOwned(owner.Labels[constant.IPPoolLabelOwned])
		if err != nil {
			return fmt.Errorf("failed to unmarshal IPPoolAnnoOwned: %v", err.Error())
		}

		idx := slices.IndexFunc(owned, func(name string) bool {
			return name == ipPool.Name
		})
		// idx := slices.IndexFunc(owned, func(item types.IPPoolAnnoOwnedItem) bool {
		// 	return item.Name == ipPool.Name
		// })

		if idx == -1 {
			owned = append(owned, ipPool.Name)
			// owned = append(owned, types.IPPoolAnnoOwnedItem{Name: ipPool.Name})
		} else {
			logger.Debug("IPPool already owned by this IPPool '%v'", ownerRef.Name)
			return nil
		}

		rawOwned, err := convert.MarshalIPPoolAnnoOwned(owned)
		if err != nil {
			return fmt.Errorf("failed to marshal IPPoolAnnoOwned: %v", err.Error())
		}

		owner.Labels[constant.IPPoolLabelOwned] = rawOwned

		return webhook.Client.Update(ctx, &owner)
	})
	if err != nil && wait.Interrupted(err) {
		err = fmt.Errorf("exhaust all retries, failed to allocate IP from IPPool %s", ownerRef.Name)
	}
	if err != nil {
		return fmt.Errorf("failed to update OwnerIPPool: %v", err.Error())
	}

	return nil
}
