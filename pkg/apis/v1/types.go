package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NetworkResourcePlural is the plural of network resource.
	NetworkResourcePlural = "networks"
	// TenantResourcePlural is the plural of tenant resource.
	TenantResourcePlural = "tenants"
)

// These are the valid phases of a network state.
const (
	// NetworkInitializing means the network is just accepted by system
	NetworkInitializing = "Initializing"
	// NetworkActive means the network is available for use in the system
	NetworkActive = "Active"
	// NetworkPending means the network is accepted by system, but it is still
	// processing by network provider
	NetworkPending = "Pending"
	// NetworkFailed means the network is not available
	NetworkFailed = "Failed"
	// NetworkTerminating means the network is undergoing graceful termination
	NetworkTerminating = "Terminating"
)

// These are the valid phases of a tenant state.
const (
	// TenantInitializing means the tenant is just accepted by system
	TenantInitializing = "Initializing"
	// TenantActive means the tenant is available for use in the system
	TenantActive = "Active"
	// TenantPending means the tenant is accepted by system, but it is still
	// processing by tenant provider
	TenantPending = "Pending"
	// TenantFailed means the tenant is not available
	TenantFailed = "Failed"
	// TenantTerminating means the tenant is undergoing graceful termination
	TenantTerminating = "Terminating"
)

// Network describes a Neutron network.
type Network struct {
	// TypeMeta defines type of the object and its API schema version.
	metav1.TypeMeta `json:",inline"`
	// ObjectMeta is metadata that all persisted resources must have.
	metav1.ObjectMeta `json:"metadata"`

	// Spec describes the behavior of a network.
	Spec NetworkSpec `json:"spec"`
	// Status describes the network status.
	Status NetworkStatus `json:"status,omitempty"`
}

// NetworkSpec is the spec of a network.
type NetworkSpec struct {
	// The CIDR of the network.
	CIDR string `json:"cidr"`
	// The gateway IP.
	Gateway string `json:"gateway"`
	// The network ID in Neutron.
	// If provided, wouldn't create a network in Neutron.
	NetworkID string `json:"networkID"`
}

// NetworkStatus is the status of a network.
type NetworkStatus struct {
	// State describes the network state.
	State string `json:"state,omitempty"`
	// Message describes why network is in current state.
	Message string `json:"message,omitempty"`
}

// NetworkList is a list of networks.
type NetworkList struct {
	// TypeMeta defines type of the object and its API schema version.
	metav1.TypeMeta `json:",inline"`
	// ObjectMeta is metadata that all persisted resources must have.
	metav1.ListMeta `json:"metadata"`
	// Items contains a list of networks.
	Items []Network `json:"items"`
}

// Tenant describes a Keystone tenant.
type Tenant struct {
	// TypeMeta defines type of the object and its API schema version.
	metav1.TypeMeta `json:",inline"`
	// ObjectMeta is metadata that all persisted resources must have.
	metav1.ObjectMeta `json:"metadata"`

	// Spec defines the behavior of a tenant.
	Spec TenantSpec `json:"spec"`
	// Status describes the tenant status.
	Status TenantStatus `json:"status,omitempty"`
}

// TenantSpec is the spec of a tenant.
type TenantSpec struct {
	// The username of this user.
	UserName string `json:"username"`
	// The password of this user.
	Password string `json:"password"`
	// The tenant ID in Keystone.
	// If provided, wouldn't create a new tenant in Keystone.
	TenantID string `json:"tenantID"`
}

// TenantStatus is the status of a tenant.
type TenantStatus struct {
	// State describes the tenant state.
	State string `json:"state,omitempty"`
	// Message describes why tenant is in current state.
	Message string `json:"message,omitempty"`
}

// TenantList is a list of tenants.
type TenantList struct {
	// TypeMeta defines type of the object and its API schema version.
	metav1.TypeMeta `json:",inline"`
	// ObjectMeta is metadata that all persisted resources must have.
	metav1.ListMeta `json:"metadata"`
	// Items contains a list of tenants.
	Items []Tenant `json:"items"`
}
