/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// SaveConfig persists the IP list to the configured backend. Callers must not
// report success, or touch DNS, when it fails (issue #200).
func SaveConfig(ipList map[string]IPs, source, configLocation, configName string) error {
	switch source {
	case "disk":
		return SaveYAMLToDisk(ipList, configLocation+"/"+configName)
	case "cr":
		if err := CreateOrUpdateNetworkConfig(ConvertToCRFormat(ipList), configName, configLocation); err != nil {
			return fmt.Errorf("save config: networkconfig %q in namespace %q: %w", configName, configLocation, err)
		}
		return nil
	default:
		return fmt.Errorf("save config: invalid LOAD_CONFIG_FROM value: %q", source)
	}
}

// SaveYAMLToDisk writes the IP list atomically: a temp file in the same
// directory, then a rename over the target. Writing in place with O_TRUNC
// emptied the config before a write that could still fail.
func SaveYAMLToDisk(ipList map[string]IPs, filename string) error {
	yamlData, err := yaml.Marshal(ipList)
	if err != nil {
		return fmt.Errorf("save yaml: marshal: %w", err)
	}

	// Rename replaces a symlink rather than following it, so resolve it first
	// and write next to the real file.
	target := filename
	if resolved, err := filepath.EvalSymlinks(filename); err == nil {
		target = resolved
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("save yaml: create temp file for %q: %w", target, err)
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeded

	if _, err := tmp.Write(yamlData); err != nil {
		tmp.Close()
		return fmt.Errorf("save yaml: write %q: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("save yaml: sync %q: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save yaml: close %q: %w", tmp.Name(), err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("save yaml: chmod %q: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return fmt.Errorf("save yaml: rename onto %q: %w", target, err)
	}

	return nil
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
