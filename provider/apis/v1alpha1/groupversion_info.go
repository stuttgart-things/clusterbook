package v1alpha1

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// Package type metadata.
const (
	Group   = "ipam.clusterbook.io"
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register these objects.
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionResource scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
	SchemeBuilder.Register(&ProviderConfigUsage{}, &ProviderConfigUsageList{})
	SchemeBuilder.Register(&IPAssignment{}, &IPAssignmentList{})
	SchemeBuilder.Register(&Network{}, &NetworkList{})
}

// ProviderConfig type metadata.
var (
	ProviderConfigKind             = reflect.TypeOf(ProviderConfig{}).Name()
	ProviderConfigGroupKind        = schema.GroupKind{Group: Group, Kind: ProviderConfigKind}.String()
	ProviderConfigKindAPIVersion   = ProviderConfigKind + "." + SchemeGroupVersion.String()
	ProviderConfigGroupVersionKind = SchemeGroupVersion.WithKind(ProviderConfigKind)
)

// IPAssignment type metadata.
var (
	IPAssignmentKind             = reflect.TypeOf(IPAssignment{}).Name()
	IPAssignmentGroupKind        = schema.GroupKind{Group: Group, Kind: IPAssignmentKind}.String()
	IPAssignmentKindAPIVersion   = IPAssignmentKind + "." + SchemeGroupVersion.String()
	IPAssignmentGroupVersionKind = SchemeGroupVersion.WithKind(IPAssignmentKind)
)

// Network type metadata.
var (
	NetworkKind             = reflect.TypeOf(Network{}).Name()
	NetworkGroupKind        = schema.GroupKind{Group: Group, Kind: NetworkKind}.String()
	NetworkKindAPIVersion   = NetworkKind + "." + SchemeGroupVersion.String()
	NetworkGroupVersionKind = SchemeGroupVersion.WithKind(NetworkKind)
)
