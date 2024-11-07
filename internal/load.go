/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"context"
	"log"
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
var groupVersion = schema.GroupVersion{Group: "example.com", Version: "v1"}
var resource = "networkconfigs"

// READY YAML FILE FROM DISK
func ReadYAMLFileFromDisk(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

func LoadProfile(source, path string) (ipList map[string]IPs) {

	// READ YAML FILE
	var err error
	var yamlData []byte

	switch source {
	case "disk":
		yamlData, err = ReadYAMLFileFromDisk(path)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
	}

	ipList = LoadYAMLStructure(yamlData)
	return
}

// FUNCTION TO GET A NETWORKCONFIG RESOURCE
func GetNetworkConfig(namespace, name string) (*NetworkConfig, error) {

	// CREATE A DYNAMIC CLIENT
	dynClient, err := CreateDynamicKubeConfigClient()
	if err != nil {
		return nil, err
	}

	// RETRIEVE THE RESOURCE
	resourceClient := dynClient.Resource(groupVersion.WithResource(resource)).Namespace(namespace)
	unstructuredConfig, err := resourceClient.Get(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// CONVERT THE UNSTRUCTURED DATA BACK TO NETWORKCONFIG STRUCT
	var networkConfig NetworkConfig
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredConfig.Object, &networkConfig)
	return &networkConfig, err
}
