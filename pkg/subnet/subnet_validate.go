package subnet

import (
	"context"
	"fmt"

	"github.com/hexiaodai/virtnet/pkg/constant"
	cniip "github.com/hexiaodai/virtnet/pkg/ip"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/ptr"
	"github.com/hexiaodai/virtnet/pkg/types"
	"github.com/hexiaodai/virtnet/pkg/utils/convert"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var (
	ipsField        *field.Path = field.NewPath("spec").Child("ips")
	gatewayField    *field.Path = field.NewPath("spec").Child("gateway")
	excludeIPsField *field.Path = field.NewPath("spec").Child("excludeIPs")
	subnetField     *field.Path = field.NewPath("spec").Child("subnet")
	specField       *field.Path = field.NewPath("spec")
)

func (webhook *SubnetWebhook) validateSubnetIPs(ctx context.Context, subnet *v1alpha1.Subnet) *field.Error {
	for i, r := range subnet.Spec.IPs {
		if err := validateContainsIPRange(ipsField.Index(i), constant.IPv4, subnet.Spec.Subnet, r); err != nil {
			return err
		}
	}

	newIPs, err := cniip.AssembleTotalIPs(constant.IPv4, subnet.Spec.IPs, subnet.Spec.ExcludeIPs)
	if err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to assemble the total IP addresses of the Subnet %s: %v", subnet.Name, err))
	}
	if len(newIPs) == 0 {
		return nil
	}

	var subnetList v1alpha1.SubnetList
	if err := webhook.Client.List(
		ctx,
		&subnetList,
		// client.MatchingLabels{constant.LabelSubnetCIDR: cidr},
	); err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to list Subnets: %v", err))
	}

	for _, pool := range subnetList.Items {
		if pool.Name == subnet.Name {
			continue
		}

		existIPs, err := cniip.AssembleTotalIPs(constant.IPv4, pool.Spec.IPs, pool.Spec.ExcludeIPs)
		if err != nil {
			return field.InternalError(ipsField, fmt.Errorf("failed to assemble the total IP addresses of the existing Subnet %s: %v", pool.Name, err))
		}

		overlapIPs := cniip.IPsIntersectionSet(newIPs, existIPs, false)
		if len(overlapIPs) == 0 {
			continue
		}
		overlapRanges, _ := cniip.ConvertIPsToIPRanges(constant.IPv4, overlapIPs)
		return field.Forbidden(
			ipsField,
			fmt.Sprintf("overlap with Subnet %s in IP ranges %v, total IP addresses of an Subnet are jointly determined by '%v' and '%v'", ipsField.String(), excludeIPsField.String(), pool.Name, overlapRanges),
		)
	}

	return nil
}

func (*SubnetWebhook) validateSubnetGateway(subnet *v1alpha1.Subnet) *field.Error {
	if subnet.Spec.Gateway == nil {
		return nil
	}

	if err := validateContainsIP(gatewayField, constant.IPv4, subnet.Spec.Subnet, ptr.OrEmpty(subnet.Spec.Gateway)); err != nil {
		return err
	}

	for _, r := range subnet.Spec.ExcludeIPs {
		contains, _ := cniip.IPRangeContainsIP(constant.IPv4, r, *subnet.Spec.Gateway)
		if contains {
			return nil
		}
	}

	for idx, ipr := range subnet.Spec.IPs {
		contains, _ := cniip.IPRangeContainsIP(constant.IPv4, ipr, *subnet.Spec.Gateway)
		if contains {
			return field.Invalid(
				ipsField.Index(idx),
				ipr,
				fmt.Sprintf("conflicts with '%v' %s, add the gateway IP address to '%v' or remove it from '%v'", gatewayField.String(), excludeIPsField.String(), ipsField.String(), *subnet.Spec.Gateway),
			)
		}
	}

	return nil
}

func (*SubnetWebhook) validateSubnetExcludeIPs(version types.IPVersion, subnet string, excludeIPs []string) *field.Error {
	for i, r := range excludeIPs {
		if err := validateContainsIPRange(excludeIPsField.Index(i), version, subnet, r); err != nil {
			return err
		}
	}

	return nil
}

func (webhook *SubnetWebhook) validateSubnetCIDR(ctx context.Context, subnet *v1alpha1.Subnet) *field.Error {
	if err := cniip.IsCIDR(constant.IPv4, subnet.Spec.Subnet); err != nil {
		return field.Invalid(
			subnetField,
			subnet.Spec.Subnet,
			err.Error(),
		)
	}

	// TODO: Use label selector.
	var subnetList v1alpha1.SubnetList
	if err := webhook.Client.List(ctx, &subnetList); err != nil {
		return field.InternalError(subnetField, fmt.Errorf("failed to list Subnets: %v", err))
	}

	for _, subnet := range subnetList.Items {
		// TODO: check IPVersion
		if subnet.Name == subnet.Name {
			return field.InternalError(subnetField, fmt.Errorf("Subnet %s already exists", subnet.Name))
		}

		if subnet.Spec.Subnet == subnet.Spec.Subnet {
			continue
		}

		overlap, err := cniip.IsCIDROverlap(constant.IPv4, subnet.Spec.Subnet, subnet.Spec.Subnet)
		if err != nil {
			return field.InternalError(subnetField, fmt.Errorf("failed to compare whether '%v' overlaps: %v", subnetField.String(), err))
		}

		if overlap {
			return field.Invalid(
				subnetField,
				subnet.Spec.Subnet,
				fmt.Sprintf("overlap with Subnet %s which '%v' is %s", subnet.Name, subnetField.String(), subnet.Spec.Subnet),
			)
		}
	}

	return nil
}

func (*SubnetWebhook) validateSubnetShouldNotBeChanged(oldSubnet, newSubnet *v1alpha1.Subnet) *field.Error {
	// TODO: check IPVersion
	if newSubnet.Spec.Subnet != oldSubnet.Spec.Subnet {
		return field.Forbidden(
			subnetField,
			"is not changeable",
		)
	}
	return nil
}

func (webhook *SubnetWebhook) validateSubnetIPInUse(subnet *v1alpha1.Subnet) *field.Error {
	ippools := v1alpha1.IPPoolList{}
	if err := webhook.Client.List(context.TODO(), &ippools, &client.ListOptions{
		Namespace:     subnet.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{constant.SubnetLabelOwner: subnet.Name}),
	}); err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to list Subnets: %w", err))
	}

	ippoolsAllocatedIPs := v1alpha1.PoolIPAllocations{}
	for _, ippool := range ippools.Items {
		result, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
		if err != nil {
			return field.InternalError(ipsField, fmt.Errorf("failed to unmarshal the allocated IP records of IPPool %s: %v", ippool.Name, err))
		}
		for ip, allocation := range result {
			ippoolsAllocatedIPs[ip] = allocation
		}
	}

	totalIPs, err := cniip.AssembleTotalIPs(constant.IPv4, subnet.Spec.IPs, subnet.Spec.ExcludeIPs)
	if err != nil {
		return field.InternalError(ipsField, fmt.Errorf("failed to assemble the total IP addresses of the Subnet %s: %v", subnet.Name, err))
	}

	totalIPsMap := map[string]struct{}{}
	for _, ip := range totalIPs {
		totalIPsMap[ip.String()] = struct{}{}
	}

	for ip, allocation := range ippoolsAllocatedIPs {
		if _, ok := totalIPsMap[ip]; !ok {
			return field.Forbidden(
				ipsField,
				fmt.Sprintf("remove an IP address %s that is being used by Pod %s, total IP addresses of an Subnet are jointly determined by '%v' and '%v'", ip, allocation.NamespacedName, ipsField.String(), excludeIPsField.String()),
			)
		}
	}

	return nil
}

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
			fmt.Sprintf("not pertains to the '%v' %s of Subnet", subnetField.String(), subnet),
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
			fmt.Sprintf("not pertains to the '%v' %s of Subnet", subnetField.String(), subnet),
		)
	}

	return nil
}
