package ippool

import (
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"
	"cni/pkg/utils/convert"
)

type sortByUnallocatedIPsCount []v1alpha1.IPPool

// type sortByUnallocatedIPsCount struct {
// 	items   []v1alpha1.IPPool
// 	average int
// }

func (s sortByUnallocatedIPsCount) Len() int {
	return len(s)
}

func (s sortByUnallocatedIPsCount) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortByUnallocatedIPsCount) Less(i, j int) bool {
	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(s[i].Status.UnallocatedIPs)
	if err != nil {
		// TODO: loggger
		return false
	}
	unallocatedIPs2, err := convert.UnmarshalIPPoolUnallocatedIPs(s[j].Status.UnallocatedIPs)
	if err != nil {
		// TODO: loggger
		return false
	}
	return len(unallocatedIPs) > len(unallocatedIPs2)
}
