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
// +kubebuilder:resource:categories={virtnet},path="ippools",scope="Cluster",shortName={ipp},singular="ippool"
// +kubebuilder:printcolumn:JSONPath=".spec.gateway",description="gateway",name="GATEWAY",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.subnet",description="subnet",name="SUBNET",type=string
// +kubebuilder:printcolumn:JSONPath=".status.allocatedIPCount",description="allocatedIPCount",name="ALLOCATED-IP-COUNT",type=integer
// +kubebuilder:printcolumn:JSONPath=".status.unallocatedIPCount",description="unallocatedIPCount",name="UNALLOCATED-IP-COUNT",type=integer
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
	// +kubebuilder:validation:Required
	Subnet string `json:"subnet"`

	// +kubebuilder:validation:Optional
	Gateway *string `json:"gateway,omitempty"`

	// +kubebuilder:validation:Optional
	IPs []string `json:"ips,omitempty"`

	// +kubebuilder:validation:Optional
	ExcludeIPs []string `json:"excludeIPs,omitempty"`
}

// IPPoolStatus defines the desired state of IPPool.
type IPPoolStatus struct {
	// +kubebuilder:validation:Optional
	AllocatedIPs *string `json:"allocatedIPs,omitempty"`

	// +kubebuilder:validation:Optional
	UnallocatedIPs *string `json:"unallocatedIPs,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	AllocatedIPCount *int64 `json:"allocatedIPCount,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	UnallocatedIPCount *int64 `json:"unallocatedIPCount,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	TotalIPCount *int64 `json:"totalIPCount,omitempty"`
}

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
