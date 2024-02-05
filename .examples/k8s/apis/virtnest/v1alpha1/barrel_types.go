package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// BarrelGVR is the name of tthe Barrel GVR.
	BarrelGVR = schema.GroupVersionResource{
		Group:    GroupVersion.Group,
		Version:  GroupVersion.Version,
		Resource: "barrels",
	}
	// BarrelGVK is the name of tthe Barrel GVK.
	BarrelGVK = schema.GroupVersionKind{
		Group:   GroupVersion.Group,
		Version: GroupVersion.Version,
		Kind:    "Barrel",
	}

	// BarrelFinalizer is the name of the finalizer for Barrel.
	BarrelFinalizer = BarrelGVR.GroupResource().String()
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={cni},path="barrels",scope="Cluster",shortName={bl},singular="barrel"
// +kubebuilder:printcolumn:JSONPath=".status.allocatedIPCount",description="allocatedIPCount",name="ALLOCATED-IP-COUNT",type=integer
// +kubebuilder:printcolumn:JSONPath=".status.unallocatedIPCount",description="unallocatedIPCount",name="UNALLOCATED-IP-COUNT",type=integer
// +kubebuilder:printcolumn:JSONPath=".status.totalIPCount",description="totalIPCount",name="TOTAL-IP-COUNT",type=integer

// Barrel is the Schema for the Barrel API
type Barrel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status BarrelStatus `json:"status,omitempty"`
}

// BarrelStatus defines the desired state of Barrel.
type BarrelStatus struct {
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

// +kubebuilder:object:root=true

// BarrelList contains a list of Barrel
type BarrelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Barrel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Barrel{}, &BarrelList{})
}
