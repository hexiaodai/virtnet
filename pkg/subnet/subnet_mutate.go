package subnet

import (
	"context"
	"fmt"

	"github.com/hexiaodai/virtnet/pkg/constant"
	cniip "github.com/hexiaodai/virtnet/pkg/ip"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/logging"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (webhook *SubnetWebhook) mutateSubnet(ctx context.Context, subnet *v1alpha1.Subnet) *field.Error {
	logger := logging.FromContext(ctx)
	logger.Named("mutateSubnet")
	logger.Debug("Start to mutate Subnet")

	if subnet.DeletionTimestamp != nil {
		logger.Debug("Terminating Subnet, noting to mutate")
		return nil
	}

	if !controllerutil.ContainsFinalizer(subnet, v1alpha1.SubnetFinalizer) {
		// TODO: Add finalizer
		// finalizersUpdated := controllerutil.AddFinalizer(ipPool, v1alpha1.SubnetFinalizer)
		// if finalizersUpdated {
		// 	logger.Debugf("Add finalizer %s to %s Subnet", ipPool.Name, v1alpha1.SubnetFinalizer)
		// }
	}

	// if ownerRef := haveSubnetOwner(ipPool); ownerRef != nil {
	// 	// TODO: updateOwnerSubnet
	// 	// if err := webhook.updateOwnerSubnet(logging.IntoContext(ctx, logger), ipPool, ownerRef); err != nil {
	// 	// 	return field.Invalid(
	// 	// 		ownerReferencesField,
	// 	// 		ownerRef,
	// 	// 		fmt.Sprintf("failed to updateOwnerSubnet: %v", err.Error()),
	// 	// 	)
	// 	// }
	// 	logger.Debugf("Subnet %s has owner %s, 'spec' set to nil", ipPool.Name, ownerRef.Name)
	// 	if ipPool.Labels == nil {
	// 		ipPool.Labels = make(map[string]string)
	// 	}
	// 	ipPool.Labels[constant.SubnetLabelOwner] = ownerRef.Name
	// 	ipPool.Spec = v1alpha1.SubnetSpec{}
	// 	// fmt.Println("==========", ipPool.Annotations)
	// 	return nil
	// }

	if len(subnet.Spec.IPs) > 0 {
		mergedIPs, err := cniip.MergeIPRanges(constant.IPv4, subnet.Spec.IPs)
		if err != nil {
			return field.Invalid(
				ipsField,
				subnet.Spec.IPs,
				fmt.Sprintf("failed to merge '%v': %v", ipsField.String(), err),
			)
		}
		ips := subnet.Spec.IPs
		subnet.Spec.IPs = mergedIPs
		logger.Debugf("Merge '%v' %v to %v", ipsField.String(), ips, mergedIPs)
	}

	if len(subnet.Spec.ExcludeIPs) > 0 {
		mergedExcludeIPs, err := cniip.MergeIPRanges(constant.IPv4, subnet.Spec.ExcludeIPs)
		if err != nil {
			return field.Invalid(
				excludeIPsField,
				subnet.Spec.ExcludeIPs,
				fmt.Sprintf("failed to merge '%v': %v", excludeIPsField.String(), err),
			)
		}
		excludeIPs := subnet.Spec.ExcludeIPs
		subnet.Spec.ExcludeIPs = mergedExcludeIPs
		logger.Debugf("Merge '%v' %v to %v", excludeIPsField.String(), excludeIPs, mergedExcludeIPs)
	}

	return nil
}
