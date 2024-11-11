/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func SaveYAMLToDisk(ipList map[string]IPs, filename string) {
	// Marshal the data to YAML format
	yamlData, err := yaml.Marshal(ipList)
	if err != nil {
		fmt.Printf("Error marshaling YAML: %v\n", err)
		return
	}

	// Open the file with O_CREATE and O_TRUNC flags to overwrite if it exists
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	// Write the YAML data to the file
	_, err = file.Write(yamlData)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	fmt.Printf("YAML data successfully written to %s\n", filename)
}

func CreateOrUpdateNetworkConfig(info map[string][]string, resourceName, namespace string) error {

	networkConfig := &NetworkConfig{
		TypeMeta: v1.TypeMeta{
			APIVersion: groupVersion.String(),
			Kind:       "NetworkConfig",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Spec: NetworkConfigSpec{
			info,
		},
	}

	// CREATE A DYNAMIC CLIENT
	dynClient, err := CreateDynamicKubeConfigClient()
	if err != nil {
		return err
	}

	// CONVERT THE NETWORKCONFIG STRUCT TO AN UNSTRUCTURED FORMAT
	unstructuredConfig, err := runtime.DefaultUnstructuredConverter.ToUnstructured(networkConfig)
	if err != nil {
		return err
	}

	// SET THE GROUP VERSION RESOURCE
	resourceClient := dynClient.Resource(groupVersion.WithResource(resource)).Namespace(namespace)

	// TRY TO UPDATE THE RESOURCE IF IT ALREADY EXISTS
	existingResource, err := resourceClient.Get(context.TODO(), networkConfig.Name, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// If not found, create a new one
			_, err = resourceClient.Create(context.TODO(), &unstructured.Unstructured{
				Object: unstructuredConfig,
			}, v1.CreateOptions{})
			return err
		}
		return err // Handle other errors
	}

	// IF IT EXISTS, UPDATE THE RESOURCE
	unstructuredConfig["metadata"] = existingResource.Object["metadata"] // Retain the existing metadata (e.g., UID, resource version)
	_, err = resourceClient.Update(context.TODO(), &unstructured.Unstructured{
		Object: unstructuredConfig,
	}, v1.UpdateOptions{})
	return err
}
