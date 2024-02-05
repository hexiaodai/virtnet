package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// IPPoolGVR is the name of tthe IPPool GVR.
	IPPoolGVR = schema.GroupVersionResource{
		Group:    GroupVersion.Group,
		Version:  GroupVersion.Version,
		Resource: "ippools",
	}
	// IPPoolGVK is the name of tthe IPPool GVK.
	IPPoolGVK = schema.GroupVersionKind{
		Group:   GroupVersion.Group,
		Version: GroupVersion.Version,
		Kind:    "IPPool",
	}

	// IPPoolFinalizer is the name of the finalizer for IPPool.
	IPPoolFinalizer = IPPoolGVR.GroupResource().String()
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={cni},path="ippools",scope="Cluster",shortName={ipp},singular="ippool"
// +kubebuilder:printcolumn:JSONPath=".spec.subnet",description="subnet",name="SUBNET",type=string
// +kubebuilder:printcolumn:JSONPath=".status.totalIPCount",description="totalIPCount",name="TOTAL-IP-COUNT",type=integer

// IPPool is the Schema for the IPPool API
type IPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPPoolSpec   `json:"spec"`
	Status IPPoolStatus `json:"status,omitempty"`
}

// IPPoolSpec defines the desired state of IPPool.
type IPPoolSpec struct {
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=1024
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Optional
	Replicas int `json:"replicas"`

	// +kubebuilder:validation:Required
	Subnet string `json:"subnet"`

	// +kubebuilder:validation:Optional
	Gateway *string `json:"gateway,omitempty"`

	// +kubebuilder:validation:Optional
	IPs []string `json:"ips,omitempty"`

	// +kubebuilder:validation:Optional
	ExcludeIPs []string `json:"excludeIPs,omitempty"`

	// +kubebuilder:validation:Maximum=4094
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	// Vlan *int `json:"vlan,omitempty"`
}

// IPPoolStatus defines the desired state of IPPool.
type IPPoolStatus struct {
	// +kubebuilder:validation:Optional
	// MergedIPs []string `json:"mergedIPs,omitempty"`

	// +kubebuilder:validation:Optional
	// AllocatedIPs *string `json:"allocatedIPs,omitempty"`

	// +kubebuilder:validation:Optional
	// UnallocatedIPs *string `json:"unallocatedIPs,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	TotalIPCount *int64 `json:"totalIPCount,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	// AllocatedIPCount *int64 `json:"allocatedIPCount,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	// UnallocatedIPCount *int64 `json:"unallocatedIPCount,omitempty"`

	// // +kubebuilder:validation:Optional
	// SharedInUnallocatedIPs *SharedUnallocatedIPs `json:"sharedInUnallocatedIPs,omitempty"`

	// // +kubebuilder:validation:Optional
	// SharedOutUnallocatedIPs *SharedUnallocatedIPs `json:"sharedOutUnallocatedIPs,omitempty"`

	// // +kubebuilder:validation:Optional
	// SharedInallocatedIPs *SharedAllocatedIPs `json:"sharedInAllocatedIPs,omitempty"`

	// // +kubebuilder:validation:Optional
	// SharedOutallocatedIPs *SharedAllocatedIPs `json:"sharedOutAllocatedIPs,omitempty"`
}

// type SharedAllocatedIPs struct {
// 	// +kubebuilder:validation:Required
// 	Name string `json:"name,omitempty"`

// 	// +kubebuilder:validation:Required
// 	AllocatedIPs string `json:"unallocatedIPs,omitempty"`
// }

// type SharedUnallocatedIPs struct {
// 	// +kubebuilder:validation:Required
// 	Name string `json:"name,omitempty"`

// 	// +kubebuilder:validation:Required
// 	UnallocatedIPs string `json:"unallocatedIPs,omitempty"`
// }

// PoolIPAllocations is a map of IP allocation details indexed by IP address.
type PoolIPAllocations map[string]PoolIPAllocation

type PoolIPAllocation struct {
	NamespacedName string `json:"pod"`
	PodUID         string `json:"podUid"`
}

// PoolIPUnallocation is a slice of IP Unallocation details value by IP address.
type PoolIPUnallocation []string

// +kubebuilder:object:root=true

// IPPoolList contains a list of IPPool
type IPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPPool{}, &IPPoolList{})
}
