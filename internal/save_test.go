package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readOnlyConfig writes a config into a directory that can be read but not
// written: loading works, saving fails. That is the shape of every failure in
// issue #200 — the request got far enough to change something, then the write
// did not land.
func readOnlyConfig(t *testing.T, yaml string) (dir, name string) {
	t.Helper()
	dir, name = setupTestConfig(t, yaml)
	makeReadOnly(t, dir)
	return dir, name
}

// makeReadOnly removes write permission from dir for the rest of the test. It
// skips when the permission is not enforced, e.g. when tests run as root.
func makeReadOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if probe, err := os.CreateTemp(dir, "probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("directory permissions are not enforced here (running as root?)")
	}
}

func TestSaveYAMLToDisk_RoundTrip(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)
	path := filepath.Join(dir, name)

	ipList, err := LoadProfile("disk", dir, name)
	if err != nil {
		t.Fatal(err)
	}
	ipList["10.31.103"]["7"] = IPInfo{Status: "ASSIGNED", Cluster: "probe"}

	if err := SaveYAMLToDisk(ipList, path); err != nil {
		t.Fatalf("SaveYAMLToDisk: %v", err)
	}

	reloaded, err := LoadProfile("disk", dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded["10.31.103"]["7"]; got.Cluster != "probe" {
		t.Errorf("entry .7 = %+v, want cluster probe", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the config file, found %d entries (temp file left behind?)", len(entries))
	}
}

// Before issue #200 a failed write was only printed, and O_TRUNC had already
// emptied the file.
func TestSaveYAMLToDisk_FailureReturnsErrorAndKeepsFile(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)
	path := filepath.Join(dir, name)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	makeReadOnly(t, dir)

	if err := SaveYAMLToDisk(map[string]IPs{"10.9.9": {}}, path); err == nil {
		t.Fatal("expected an error writing into a read-only directory")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("config changed despite the failed save:\n%s", after)
	}
}

func TestSaveYAMLToDisk_KeepsSymlink(t *testing.T) {
	realDir := t.TempDir()
	realPath := filepath.Join(realDir, "config.yaml")
	if err := os.WriteFile(realPath, []byte(testConfigYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	if err := SaveYAMLToDisk(map[string]IPs{"10.9.9": {"1": {}}}, linkPath); err != nil {
		t.Fatalf("SaveYAMLToDisk: %v", err)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a regular file")
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "10.9.9") {
		t.Errorf("the link target was not updated:\n%s", data)
	}
}

func TestSaveConfig_InvalidSource(t *testing.T) {
	if _, err := saveConfig(map[string]IPs{}, "nfs", t.TempDir(), "config.yaml", ""); err == nil {
		t.Error("expected an error for an unknown LOAD_CONFIG_FROM value")
	}
}
