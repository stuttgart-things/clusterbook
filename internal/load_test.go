package internal

import (
	"fmt"
	"os"
	"testing"
)

func TestReadYAMLFileFromDisk(t *testing.T) {
	// Beispiel-YAML-Inhalt
	yamlContent := `
10.31.103:
  3:
    status: ""
    cluster: ""
  4:
    status: PENDING
    cluster: losangeles
  5:
    status: ASSIGNED
    cluster: skyami
  6:
    status: ""
    cluster: ""
  7:
    status: ""
    cluster: ""
  8:
    status: ""
    cluster: ""
  9:
    status: PENDING
    cluster: cicd
  10:
    status: ""
    cluster: ""
10.31.104:
  4:
    status: PENDING
    cluster: losangeles
  5:
    status: ASSIGNED
    cluster: miami
  6:
    status: ""
    cluster: ""
  7:
    status: ""
    cluster: ""
`

	// Temporäre Datei erstellen
	tmpFile, err := os.CreateTemp("", "test*.yaml")
	if err != nil {
		t.Fatalf("error creating temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // Aufräumen nach dem Test

	// YAML-Inhalt in die temporäre Datei schreiben
	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("error writing to temporary file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("error closing temporary file: %v", err)
	}

	// Funktion ReadYAMLFileFromDisk testen
	yamlData, err := ReadYAMLFileFromDisk(tmpFile.Name())
	if err != nil {
		t.Fatalf("error reading YAML file: %v", err)
	}

	// Überprüfen, ob die gelesenen Daten mit dem ursprünglichen Inhalt übereinstimmen
	if string(yamlData) != yamlContent {
		t.Errorf("expected %s, got %s", yamlContent, string(yamlData))
	}
}

func TestLoadProfile(t *testing.T) {

	ipList := LoadProfile("disk", "../tests", "config.yaml")
	fmt.Println(ipList)
}
