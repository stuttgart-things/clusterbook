/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"log"
	"os"
)

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
