/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"context"
	"fmt"
	"os"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DEFINE THE NETWORKCONFIG STRUCT
type NetworkConfig struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`
	Spec          NetworkConfigSpec `json:"spec,omitempty"`
}

type NetworkConfigSpec struct {
	Networks map[string][]string `json:"networks"`
}

// CREATE THE CUSTOM RESOURCE GROUPVERSION
var (
	groupVersion = schema.GroupVersion{Group: "github.stuttgart-things.com", Version: "v1"}
	resource     = "networkconfigs"
)

// READY YAML FILE FROM DISK
func ReadYAMLFileFromDisk(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

// LoadProfile reads the NetworkConfig from the configured source (disk or CR).
// It returns an error instead of terminating the process so that callers — web
// handlers, gRPC handlers, and the background reclaimer — can fail a single
// operation gracefully rather than crashing the whole server (and its readiness
// probe) when the config is temporarily missing or unreachable.
func LoadProfile(source, configLocation, configName string) (map[string]IPs, error) {
	switch source {
	// READ NetworkConfig FROM DISK
	case "disk":
		yamlData, err := ReadYAMLFileFromDisk(configLocation + "/" + configName)
		if err != nil {
			return nil, fmt.Errorf("load profile: read yaml from disk %q: %w", configLocation+"/"+configName, err)
		}
		return LoadYAMLStructure(yamlData), nil

	// READ NetworkConfig FROM CR
	case "cr":
		retrievedConfig, err := GetNetworkConfig(configName, configLocation)
		if err != nil {
			return nil, fmt.Errorf("load profile: get networkconfig %q in namespace %q: %w", configName, configLocation, err)
		}
		return ConvertFromCRFormat(retrievedConfig.Spec.Networks), nil

	default:
		return nil, fmt.Errorf("load profile: invalid LOAD_CONFIG_FROM value: %q", source)
	}
}

// FUNCTION TO GET A NETWORKCONFIG RESOURCE
func GetNetworkConfig(resourceName, namespace string) (*NetworkConfig, error) {
	// CREATE A DYNAMIC CLIENT
	dynClient, err := CreateDynamicKubeConfigClient()
	if err != nil {
		return nil, err
	}

	// RETRIEVE THE RESOURCE
	resourceClient := dynClient.Resource(groupVersion.WithResource(resource)).Namespace(namespace)
	unstructuredConfig, err := resourceClient.Get(context.TODO(), resourceName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// CONVERT THE UNSTRUCTURED DATA BACK TO NETWORKCONFIG STRUCT
	var networkConfig NetworkConfig
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredConfig.Object, &networkConfig)
	return &networkConfig, err
}
