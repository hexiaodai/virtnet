package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// SubnetGVR is the name of the Subnet GVR.
	SubnetGVR = schema.GroupVersionResource{
		Group:    GroupVersion.Group,
		Version:  GroupVersion.Version,
		Resource: "subnets",
	}
	// SubnetGVK is the name of the Subnet GVK.
	SubnetGVK = schema.GroupVersionKind{
		Group:   GroupVersion.Group,
		Version: GroupVersion.Version,
		Kind:    "Subnet",
	}

	// SubnetFinalizer is the name of the finalizer for Subnet.
	SubnetFinalizer = SubnetGVR.GroupResource().String()
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={virtnet},path="subnets",scope="Cluster",shortName={sb},singular="subnet"
// +kubebuilder:printcolumn:JSONPath=".spec.gateway",description="gateway",name="GATEWAY",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.subnet",description="subnet",name="SUBNET",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.replicas",description="replicas",name="REPLICAS",type=integer
// +kubebuilder:printcolumn:JSONPath=".status.totalIPCount",description="totalIPCount",name="TOTAL-IP-COUNT",type=integer

// Subnet is the Schema for the Subnet API
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec"`
	Status SubnetStatus `json:"status,omitempty"`
}

// SubnetSpec defines the desired state of Subnet.
type SubnetSpec struct {
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
}

// SubnetStatus defines the desired state of Subnet.
type SubnetStatus struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Optional
	TotalIPCount *int64 `json:"totalIPCount,omitempty"`
}

// +kubebuilder:object:root=true

// SubnetList contains a list of Subnet
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subnet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Subnet{}, &SubnetList{})
}
