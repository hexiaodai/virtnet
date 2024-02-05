package types

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AppNamespacedName struct {
	GroupVersionKind schema.GroupVersionKind
	Namespace        string
	Name             string
}

func (app *AppNamespacedName) String() string {
	bytes, err := json.Marshal(app)
	if err != nil {
		// TODO: Log recorder
		return ""
	}
	return string(bytes)
}

// type IPAMAnnoPodSubnetValue []IPAMAnnoIPPoolItem

type IPAMAnnoSubnet struct {
	NIC        string `json:"interface"`
	IPv4Subnet string `json:"ipv4"`
	Address    string `json:"address"`
}

// type IPAMAnnoPodIPPoolsValue []IPAMAnnoIPPoolItem

// type IPAMAnnoIPPoolItem struct {
// 	NIC       string   `json:"interface"`
// 	IPv4Pools []string `json:"ipv4"`
// 	Address   string   `json:"address"`

// 	idx int
// }

// func (i *IPAMAnnoIPPoolItem) Next(args ...bool) string {
// 	if len(i.IPv4Pools) == 0 {
// 		return ""
// 	}

// 	loop := false
// 	if len(args) > 0 {
// 		loop = args[0]
// 	}

// 	if i.idx > len(i.IPv4Pools)-1 && loop {
// 		i.idx = 0
// 	}

// 	return i.IPv4Pools[i.idx]
// }

// type IPPoolAnnoOwnedValue []string
