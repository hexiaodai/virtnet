package ip

import (
	"net"

	"github.com/hexiaodai/virtnet/pkg/types"
)

func AssembleTotalIPs(ipVersion types.IPVersion, ipRanges, excludedIPRanges []string) ([]net.IP, error) {
	ips, err := ParseIPRanges(ipVersion, ipRanges)
	if nil != err {
		return nil, err
	}
	excludeIPs, err := ParseIPRanges(ipVersion, excludedIPRanges)
	if nil != err {
		return nil, err
	}
	totalIPs := IPsDiffSet(ips, excludeIPs, false)

	return totalIPs, nil
}

// func UnmarshalIPPoolAllocatedIPs(data *string) (v1alpha1.PoolIPAllocations, error) {
// 	if data == nil {
// 		return nil, nil
// 	}

// 	var records v1alpha1.PoolIPAllocations
// 	if err := json.Unmarshal([]byte(*data), &records); err != nil {
// 		return nil, err
// 	}

// 	return records, nil
// }
