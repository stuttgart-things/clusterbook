/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
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
