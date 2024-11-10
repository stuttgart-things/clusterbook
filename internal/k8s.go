/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"os"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func CreateDynamicKubeConfigClient() (dynClient dynamic.Interface, err error) {
	// Build the Kubernetes config
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return nil, err
	}

	// Create a dynamic client using the config
	dynClient, err = createDynamicClient(kubeConfig)
	if err != nil {
		return nil, err
	}

	return
}

// FUNCTION TO CREATE A DYNAMIC CLIENT FOR CRDS
func createDynamicClient(config *rest.Config) (dynamic.Interface, error) {
	return dynamic.NewForConfig(config)
}
