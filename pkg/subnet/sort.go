package subnet

import (
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/utils/convert"
)

type sortIPPoolWithUnallocatedIPsCount []v1alpha1.IPPool

func (s sortIPPoolWithUnallocatedIPsCount) Len() int {
	return len(s)
}

func (s sortIPPoolWithUnallocatedIPsCount) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortIPPoolWithUnallocatedIPsCount) Less(i, j int) bool {
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
