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

// saveConfig persists the IP list to the configured backend and returns the new
// version. Callers must not report success, or touch DNS, when it fails (issue
// #200). It is reached only through LedgerWrite.Save, which holds the write lock
// and supplies the version it loaded (issue #199).
func saveConfig(ipList map[string]IPs, source, configLocation, configName, loadedVersion string) (string, error) {
	switch source {
	case "disk":
		return "", SaveYAMLToDisk(ipList, configLocation+"/"+configName)
	case "cr":
		version, err := CreateOrUpdateNetworkConfig(ConvertToCRFormat(ipList), configName, configLocation, loadedVersion)
		if err != nil {
			return "", fmt.Errorf("save config: networkconfig %q in namespace %q: %w", configName, configLocation, err)
		}
		return version, nil
	default:
		return "", fmt.Errorf("save config: invalid LOAD_CONFIG_FROM value: %q", source)
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

// CreateOrUpdateNetworkConfig writes the NetworkConfig only if it is still at
// expectedVersion — the resourceVersion the caller loaded, or "" when it loaded a
// CR that did not exist. Otherwise it returns ErrLedgerConflict. It returns the
// resourceVersion after the write.
//
// Before issue #199 it fetched the object again right before updating and copied
// that fresh resourceVersion, so the apiserver's own conflict check always passed
// and the last writer silently won.
func CreateOrUpdateNetworkConfig(info map[string][]string, resourceName, namespace, expectedVersion string) (string, error) {
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
	dynClient, err := newDynamicClient()
	if err != nil {
		return "", err
	}

	// CONVERT THE NETWORKCONFIG STRUCT TO AN UNSTRUCTURED FORMAT
	unstructuredConfig, err := runtime.DefaultUnstructuredConverter.ToUnstructured(networkConfig)
	if err != nil {
		return "", err
	}

	// SET THE GROUP VERSION RESOURCE
	resourceClient := dynClient.Resource(groupVersion.WithResource(resource)).Namespace(namespace)

	existingResource, err := resourceClient.Get(context.TODO(), networkConfig.Name, v1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", err
		}
		if expectedVersion != "" {
			return "", fmt.Errorf("%w: deleted since it was loaded at resourceVersion %s", ErrLedgerConflict, expectedVersion)
		}

		// NOT FOUND AND NONE WAS LOADED: CREATE
		created, err := resourceClient.Create(context.TODO(), &unstructured.Unstructured{
			Object: unstructuredConfig,
		}, v1.CreateOptions{})
		if errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("%w: created by another writer", ErrLedgerConflict)
		}
		if err != nil {
			return "", err
		}
		return created.GetResourceVersion(), nil
	}

	if current := existingResource.GetResourceVersion(); current != expectedVersion {
		return "", fmt.Errorf("%w: resourceVersion is %s, loaded %q", ErrLedgerConflict, current, expectedVersion)
	}

	// UPDATE, KEEPING THE EXISTING METADATA (LABELS, OWNERS, AND THE LOADED
	// RESOURCEVERSION, SO A WRITE SINCE THE GET ABOVE STILL FAILS WITH 409)
	unstructuredConfig["metadata"] = existingResource.Object["metadata"]
	updated, err := resourceClient.Update(context.TODO(), &unstructured.Unstructured{
		Object: unstructuredConfig,
	}, v1.UpdateOptions{})
	if errors.IsConflict(err) {
		return "", fmt.Errorf("%w: %v", ErrLedgerConflict, err)
	}
	if err != nil {
		return "", err
	}
	return updated.GetResourceVersion(), nil
}
