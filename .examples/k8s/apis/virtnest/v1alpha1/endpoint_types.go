package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// EndpointGVR is the name of tthe Endpoint GVR.
	EndpointGVR = schema.GroupVersionResource{
		Group:    GroupVersion.Group,
		Version:  GroupVersion.Version,
		Resource: "endpoints",
	}
	// EndpointGVK is the name of tthe Endpoint GVK.
	EndpointGVK = schema.GroupVersionKind{
		Group:   GroupVersion.Group,
		Version: GroupVersion.Version,
		Kind:    "Endpoint",
	}

	// EndpointFinalizer is the name of the finalizer for Endpoint.
	EndpointFinalizer = EndpointGVR.GroupResource().String()
)

// +kubebuilder:resource:categories={cni},path="endpoints",scope="Namespaced",shortName={se},singular="endpoint"
// +kubebuilder:printcolumn:JSONPath=".status.current.ips[0].interface",description="interface",name="INTERFACE",type=string
// +kubebuilder:printcolumn:JSONPath=".status.current.ips[0].ipv4Pool",description="ipv4Pool",name="IPV4POOL",type=string
// +kubebuilder:printcolumn:JSONPath=".status.current.ips[0].ipv4",description="ipv4",name="IPV4",type=string
// +kubebuilder:printcolumn:JSONPath=".status.current.node",description="node",name="NODE",type=string
// +kubebuilder:object:root=true

// Endpoint is the Schema for the Endpoint API
type Endpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status EndpointStatus `json:"status,omitempty"`
}

// EndpointStatus defines the desired state of Endpoint.
type EndpointStatus struct {
	// +kubebuilder:validation:Required
	Current PodIPAllocation `json:"current"`

	// +kubebuilder:validation:Required
	OwnerControllerType string `json:"ownerControllerType"`

	// +kubebuilder:validation:Required
	OwnerControllerName string `json:"ownerControllerName"`
}

type PodIPAllocation struct {
	// +kubebuilder:validation:Required
	UID string `json:"uid"`

	// +kubebuilder:validation:Required
	Node string `json:"node"`

	// +kubebuilder:validation:Required
	IPs []IPAllocationDetail `json:"ips"`
}

type IPAllocationDetail struct {
	// +kubebuilder:validation:Required
	NIC string `json:"interface"`

	// +kubebuilder:validation:Optional
	IPv4 *string `json:"ipv4,omitempty"`

	// +kubebuilder:validation:Optional
	IPv4Pool *string `json:"ipv4Pool,omitempty"`

	//// +kubebuilder:default=0
	//// +kubebuilder:validation:Maximum=4094
	//// +kubebuilder:validation:Minimum=0
	//// +kubebuilder:validation:Optional
	// Vlan *int64 `json:"vlan,omitempty"`

	// +kubebuilder:validation:Optional
	IPv4Gateway *string `json:"ipv4Gateway,omitempty"`

	//// +kubebuilder:validation:Optional
	// IPv6 *string `json:"ipv6,omitempty"`

	// +kubebuilder:validation:Optional
	Routes []Route `json:"routes,omitempty"`
}

type Route struct {
	// +kubebuilder:validation:Required
	Dst string `json:"dst"`

	// +kubebuilder:validation:Required
	Gw string `json:"gw"`
}

// +kubebuilder:object:root=true

// EndpointList contains a list of Endpoint
type EndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Endpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Endpoint{}, &EndpointList{})
}
