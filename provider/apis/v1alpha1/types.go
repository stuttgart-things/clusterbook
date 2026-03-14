package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- ProviderConfig ---

// ProviderConfigSpec defines the desired state of ProviderConfig.
type ProviderConfigSpec struct {
	// Credentials source - typically "None" for clusterbook.
	// +kubebuilder:default="None"
	Credentials *xpv1.CommonCredentialSelectors `json:"credentials,omitempty"`

	// Endpoint is the URL of the clusterbook REST API.
	Endpoint string `json:"endpoint"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// ProviderConfig configures the clusterbook IPAM provider.
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec        `json:"spec"`
	Status xpv1.ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig.
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// ProviderConfigUsage indicates that a resource is using a ProviderConfig.
type ProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv1.ProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ProviderConfigUsageList contains a list of ProviderConfigUsage.
type ProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfigUsage `json:"items"`
}

// --- IPAssignment ---

// IPAssignmentParameters defines the desired IP assignment.
type IPAssignmentParameters struct {
	// NetworkKey is the network key in clusterbook (e.g. "10.31.103").
	NetworkKey string `json:"networkKey"`

	// Cluster is the name of the cluster to assign IPs to.
	Cluster string `json:"cluster"`

	// CountIPs is the number of IPs to assign.
	// +kubebuilder:default=1
	CountIPs int `json:"countIPs,omitempty"`

	// Status is the assignment status (e.g. "ASSIGNED", "PENDING").
	// +kubebuilder:default="ASSIGNED"
	Status string `json:"status,omitempty"`

	// CreateDNS controls whether DNS records should be created.
	// +kubebuilder:default=false
	CreateDNS bool `json:"createDns,omitempty"`
}

// IPAssignmentObservation holds the observed state from clusterbook.
type IPAssignmentObservation struct {
	// IPAddresses is the list of assigned IP addresses.
	IPAddresses []string `json:"ipAddresses,omitempty"`

	// IPAddress is the first assigned IP address (convenience field).
	IPAddress string `json:"ipAddress,omitempty"`
}

// IPAssignmentSpec defines the desired state of IPAssignment.
type IPAssignmentSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       IPAssignmentParameters `json:"forProvider"`
}

// IPAssignmentStatus defines the observed state of IPAssignment.
type IPAssignmentStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          IPAssignmentObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,clusterbook}
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// IPAssignment is the Schema for the ipassignments API.
type IPAssignment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPAssignmentSpec   `json:"spec"`
	Status IPAssignmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IPAssignmentList contains a list of IPAssignment.
type IPAssignmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPAssignment `json:"items"`
}

// --- Network ---

// NetworkParameters defines the desired network configuration.
type NetworkParameters struct {
	// NetworkKey is the network key in clusterbook (e.g. "10.31.103").
	NetworkKey string `json:"networkKey"`

	// IPs is a flat list of IP suffixes to create.
	IPs []string `json:"ips,omitempty"`

	// IPFrom is the start of a range of IP suffixes to generate.
	IPFrom int `json:"ipFrom,omitempty"`

	// IPTo is the end of a range of IP suffixes to generate.
	IPTo int `json:"ipTo,omitempty"`

	// CIDR notation for automatic network generation.
	CIDR string `json:"cidr,omitempty"`

	// Reserved IPs to exclude when using CIDR mode.
	Reserved []string `json:"reserved,omitempty"`
}

// NetworkObservation holds the observed state from clusterbook.
type NetworkObservation struct {
	// TotalIPs is the total number of IPs in the network.
	TotalIPs int `json:"totalIPs,omitempty"`

	// AssignedIPs is the number of assigned IPs.
	AssignedIPs int `json:"assignedIPs,omitempty"`

	// AvailableIPs is the number of available IPs.
	AvailableIPs int `json:"availableIPs,omitempty"`
}

// NetworkSpec defines the desired state of Network.
type NetworkSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       NetworkParameters `json:"forProvider"`
}

// NetworkStatus defines the observed state of Network.
type NetworkStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          NetworkObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,clusterbook}
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// Network is the Schema for the networks API.
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkSpec   `json:"spec"`
	Status NetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkList contains a list of Network.
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Network `json:"items"`
}
