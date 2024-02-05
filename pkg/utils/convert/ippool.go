package convert

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/types"
)

func UnmarshalIPPoolAllocatedIPs(data *string) (v1alpha1.PoolIPAllocations, error) {
	if data == nil || len(*data) == 0 {
		return v1alpha1.PoolIPAllocations{}, nil
	}
	var records v1alpha1.PoolIPAllocations
	if err := json.Unmarshal([]byte(*data), &records); err != nil {
		return nil, err
	}
	return records, nil
}

func MarshalIPPoolAllocatedIPs(records v1alpha1.PoolIPAllocations) (*string, error) {
	if len(records) == 0 {
		return nil, nil
	}
	v, err := json.Marshal(records)
	if err != nil {
		return nil, err
	}
	data := string(v)
	return &data, nil
}

func MarshalIPPoolUnallocatedIPs(records v1alpha1.PoolIPUnallocation) (*string, error) {
	if len(records) == 0 {
		return nil, nil
	}
	v, err := json.Marshal(records)
	if err != nil {
		return nil, err
	}
	data := string(v)
	return &data, nil
}

func UnmarshalIPPoolUnallocatedIPs(data *string) (v1alpha1.PoolIPUnallocation, error) {
	if data == nil || len(*data) == 0 {
		return v1alpha1.PoolIPUnallocation{}, nil
	}
	var records v1alpha1.PoolIPUnallocation
	if err := json.Unmarshal([]byte(*data), &records); err != nil {
		return nil, err
	}
	return records, nil
}

func UnmarshalIPAMAnnoSubnet(data string) (types.IPAMAnnoSubnet, error) {
	var ret types.IPAMAnnoSubnet
	if err := json.Unmarshal([]byte(data), &ret); err != nil {
		return types.IPAMAnnoSubnet{}, fmt.Errorf("failed to unmarshal IPAMAnnoSubnet: %v", err)
	}
	if len(ret.Address) > 0 && net.ParseIP(ret.Address) == nil {
		return types.IPAMAnnoSubnet{}, fmt.Errorf("failed to parse IP address %s", ret.Address)
	}

	return ret, nil
}

// func UnmarshalIPAMAnnoPodIPPools(data string) (types.IPAMAnnoPodIPPoolsValue, error) {
// 	var ret types.IPAMAnnoPodIPPoolsValue
// 	if err := json.Unmarshal([]byte(data), &ret); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal IPAMAnnoPodIPPoolsValue: %v", err)
// 	}
// 	return ret, nil
// }

// func UnmarshalIPAMAnnoPodIPPool(data string) (types.IPAMAnnoIPPoolItem, error) {
// 	var ret types.IPAMAnnoIPPoolItem
// 	if err := json.Unmarshal([]byte(data), &ret); err != nil {
// 		return types.IPAMAnnoIPPoolItem{}, fmt.Errorf("failed to unmarshal IPAMAnnoIPPoolItem: %v", err)
// 	}
// 	if len(ret.Address) > 0 && net.ParseIP(ret.Address) == nil {
// 		return types.IPAMAnnoIPPoolItem{}, fmt.Errorf("failed to parse IP address %s", ret.Address)
// 	}

// 	return ret, nil
// }

// func UnmarshalIPPoolAnnoOwned(data string) (types.IPPoolAnnoOwnedValue, error) {
// 	if len(data) == 0 {
// 		return types.IPPoolAnnoOwnedValue{}, nil
// 	}
// 	var ret types.IPPoolAnnoOwnedValue
// 	if err := json.Unmarshal([]byte(data), &ret); err != nil {
// 		return types.IPPoolAnnoOwnedValue{}, fmt.Errorf("failed to unmarshal IPPoolAnnoOwnedValue: %v", err)
// 	}
// 	return ret, nil
// }

// func MarshalIPPoolAnnoOwned(data types.IPPoolAnnoOwnedValue) (string, error) {
// 	if len(data) == 0 {
// 		return "", nil
// 	}
// 	ret, err := json.Marshal(data)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to marshal IPPoolAnnoOwnedValue: %v", err)
// 	}
// 	return string(ret), nil
// }

// func PodTopCtrlToIPPoolLabelExclusiveValue(ns string, topOwnerRef *metav1.OwnerReference) string {
// 	return fmt.Sprintf("%s/%s/%s", topOwnerRef.Kind, ns, topOwnerRef.Name)
// }
