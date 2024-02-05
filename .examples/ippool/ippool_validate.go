package ippool

import (
	"cni/pkg/constant"
	cniip "cni/pkg/ip"
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"
	"cni/pkg/ptr"
	"cni/pkg/types"
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

var (
	ipsField             *field.Path = field.NewPath("spec").Child("ips")
	gatewayField         *field.Path = field.NewPath("spec").Child("gateway")
	excludeIPsField      *field.Path = field.NewPath("spec").Child("excludeIPs")
	subnetField          *field.Path = field.NewPath("spec").Child("subnet")
	specField            *field.Path = field.NewPath("spec")
	ownerReferencesField *field.Path = field.NewPath("metadata").Child("ownerReferences")
)

func (webhook *IPPoolWebhook) validateIPPoolIPs(ctx context.Context, ipPool *v1alpha1.IPPool) *field.Error {
	for i, r := range ipPool.Spec.IPs {
		if err := validateContainsIPRange(ipsField.Index(i), constant.IPv4, ipPool.Spec.Subnet, r); err != nil {
			return err
		}
	}

	newIPs, err := cniip.AssembleTotalIPs(constant.IPv4, ipPool.Spec.IPs, ipPool.Spec.ExcludeIPs)
	if err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to assemble the total IP addresses of the IPPool %s: %v", ipPool.Name, err))
	}
	if len(newIPs) == 0 {
		return nil
	}

	var ipPoolList v1alpha1.IPPoolList
	if err := webhook.Client.List(
		ctx,
		&ipPoolList,
		// client.MatchingLabels{constant.LabelIPPoolCIDR: cidr},
	); err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to list IPPools: %v", err))
	}

	for _, pool := range ipPoolList.Items {
		if pool.Name == ipPool.Name {
			continue
		}

		existIPs, err := cniip.AssembleTotalIPs(constant.IPv4, pool.Spec.IPs, pool.Spec.ExcludeIPs)
		if err != nil {
			return field.InternalError(ipsField, fmt.Errorf("failed to assemble the total IP addresses of the existing IPPool %s: %v", pool.Name, err))
		}

		overlapIPs := cniip.IPsIntersectionSet(newIPs, existIPs, false)
		if len(overlapIPs) == 0 {
			continue
		}
		overlapRanges, _ := cniip.ConvertIPsToIPRanges(constant.IPv4, overlapIPs)
		return field.Forbidden(
			ipsField,
			fmt.Sprintf("overlap with IPPool %s in IP ranges %v, total IP addresses of an IPPool are jointly determined by '%v' and '%v'", ipsField.String(), excludeIPsField.String(), pool.Name, overlapRanges),
		)
	}

	return nil
}

func (*IPPoolWebhook) validateIPPoolGateway(ipPool *v1alpha1.IPPool) *field.Error {
	if ipPool.Spec.Gateway == nil {
		return nil
	}

	if err := validateContainsIP(gatewayField, constant.IPv4, ipPool.Spec.Subnet, ptr.OrEmpty(ipPool.Spec.Gateway)); err != nil {
		return err
	}

	for _, r := range ipPool.Spec.ExcludeIPs {
		contains, _ := cniip.IPRangeContainsIP(constant.IPv4, r, *ipPool.Spec.Gateway)
		if contains {
			return nil
		}
	}

	for idx, ipr := range ipPool.Spec.IPs {
		contains, _ := cniip.IPRangeContainsIP(constant.IPv4, ipr, *ipPool.Spec.Gateway)
		if contains {
			return field.Invalid(
				ipsField.Index(idx),
				ipr,
				fmt.Sprintf("conflicts with '%v' %s, add the gateway IP address to '%v' or remove it from '%v'", gatewayField.String(), excludeIPsField.String(), ipsField.String(), *ipPool.Spec.Gateway),
			)
		}
	}

	return nil
}

func (*IPPoolWebhook) validateIPPoolExcludeIPs(version types.IPVersion, subnet string, excludeIPs []string) *field.Error {
	for i, r := range excludeIPs {
		if err := validateContainsIPRange(excludeIPsField.Index(i), version, subnet, r); err != nil {
			return err
		}
	}

	return nil
}

func (webhook *IPPoolWebhook) validateIPPoolCIDR(ctx context.Context, ipPool *v1alpha1.IPPool) *field.Error {
	if err := cniip.IsCIDR(constant.IPv4, ipPool.Spec.Subnet); err != nil {
		return field.Invalid(
			subnetField,
			ipPool.Spec.Subnet,
			err.Error(),
		)
	}

	// TODO: Use label selector.
	var ipPoolList v1alpha1.IPPoolList
	if err := webhook.Client.List(ctx, &ipPoolList); err != nil {
		return field.InternalError(subnetField, fmt.Errorf("failed to list IPPools: %v", err))
	}

	for _, pool := range ipPoolList.Items {
		// TODO: check IPVersion
		if pool.Name == ipPool.Name {
			return field.InternalError(subnetField, fmt.Errorf("IPPool %s already exists", ipPool.Name))
		}

		if pool.Spec.Subnet == ipPool.Spec.Subnet {
			continue
		}

		overlap, err := cniip.IsCIDROverlap(constant.IPv4, ipPool.Spec.Subnet, pool.Spec.Subnet)
		if err != nil {
			return field.InternalError(subnetField, fmt.Errorf("failed to compare whether '%v' overlaps: %v", subnetField.String(), err))
		}

		if overlap {
			return field.Invalid(
				subnetField,
				ipPool.Spec.Subnet,
				fmt.Sprintf("overlap with IPPool %s which '%v' is %s", pool.Name, subnetField.String(), pool.Spec.Subnet),
			)
		}
	}

	return nil
}

func (*IPPoolWebhook) validateIPPoolShouldNotBeChanged(oldIPPool, newIPPool *v1alpha1.IPPool) *field.Error {
	// TODO: check IPVersion
	if newIPPool.Spec.Subnet != oldIPPool.Spec.Subnet {
		return field.Forbidden(
			subnetField,
			"is not changeable",
		)
	}
	return nil
}

func (webhook *IPPoolWebhook) validateIPPoolIPInUse(ipPool *v1alpha1.IPPool) *field.Error {
	allocatedRecords, err := cniip.UnmarshalIPPoolAllocatedIPs(ipPool.Status.AllocatedIPs)
	if err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to unmarshal the allocated IP records of IPPool %s: %v", ipPool.Name, err))
	}

	totalIPs, err := cniip.AssembleTotalIPs(constant.IPv4, ipPool.Spec.IPs, ipPool.Spec.ExcludeIPs)
	if err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to assemble the total IP addresses of the IPPool %s: %v", ipPool.Name, err))
	}

	totalIPsMap := map[string]struct{}{}
	for _, ip := range totalIPs {
		totalIPsMap[ip.String()] = struct{}{}
	}

	for ip, allocation := range allocatedRecords {
		if _, ok := totalIPsMap[ip]; !ok {
			return field.Forbidden(
				ipsField,
				fmt.Sprintf("remove an IP address %s that is being used by Pod %s, total IP addresses of an IPPool are jointly determined by '%v' and '%v'", ip, allocation.NamespacedName, ipsField.String(), excludeIPsField.String()),
			)
		}
	}

	return nil
}

// func (webhook *IPPoolWebhook) validateIPPoolOwner(ipPool *v1alpha1.IPPool) *field.Error {
// 	owners := ipPool.GetOwnerReferences()

// 	var ippoolOwner *metav1.OwnerReference
// 	for idx, owner := range owners {
// 		if v1alpha1.IPPoolGVK.Kind == owner.Kind && v1alpha1.IPPoolGVK.GroupVersion().String() == owner.APIVersion {
// 			if ippoolOwner != nil {
// 				return field.Duplicate(
// 					ownerReferencesField.Index(idx),
// 					owners,
// 				)
// 			}
// 			ippoolOwner = &owner
// 		}
// 	}

// 	if ippoolOwner == nil && reflect.DeepEqual(ipPool.Spec, v1alpha1.IPPoolSpec{}) {
// 		return field.Invalid(
// 			specField,
// 			ipPool.Spec,
// 			fmt.Sprintf("IPPool without IPPoolOwner is not allowed to have empty '%v'", specField.String()),
// 		)
// 	}

// 	if ippoolOwner != nil && !reflect.DeepEqual(ipPool.Spec, v1alpha1.IPPoolSpec{}) {
// 		return field.Invalid(
// 			specField,
// 			ipPool.Spec,
// 			fmt.Sprintf("IPPool with IPPoolOwner is not allowed to have '%v'", specField.String()),
// 		)
// 	}

// 	return nil
// }

func validateContainsIPRange(fieldPath *field.Path, version types.IPVersion, subnet string, ipRange string) *field.Error {
	contains, err := cniip.ContainsIPRange(version, subnet, ipRange)
	if err != nil {
		return field.Invalid(
			fieldPath,
			ipRange,
			err.Error(),
		)
	}

	if !contains {
		return field.Invalid(
			fieldPath,
			ipRange,
			fmt.Sprintf("not pertains to the '%v' %s of IPPool", subnetField.String(), subnet),
		)
	}

	return nil
}

func validateContainsIP(fieldPath *field.Path, version types.IPVersion, subnet string, ip string) *field.Error {
	contains, err := cniip.ContainsIP(version, subnet, ip)
	if err != nil {
		return field.Invalid(
			fieldPath,
			ip,
			err.Error(),
		)
	}

	if !contains {
		return field.Invalid(
			fieldPath,
			ip,
			fmt.Sprintf("not pertains to the '%v' %s of IPPool", subnetField.String(), subnet),
		)
	}

	return nil
}
