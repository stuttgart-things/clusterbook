/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"strings"
	"testing"

	"github.com/pterm/pterm"
)

// ── Unit tests: pure helpers (no SSH) ─────────────────────────────────────────

func TestMergeDNSEntry_NewEntry(t *testing.T) {
	result := mergeDNSEntry("", "address=/myapp.sthings.lab/10.31.103.6", "myapp.sthings.lab")
	if result != "address=/myapp.sthings.lab/10.31.103.6" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestMergeDNSEntry_DeduplicatesExisting(t *testing.T) {
	existing := "address=/myapp.sthings.lab/10.31.103.5\naddress=/other.sthings.lab/10.31.103.7"
	result := mergeDNSEntry(existing, "address=/myapp.sthings.lab/10.31.103.6", "myapp.sthings.lab")

	count := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, "/myapp.sthings.lab/") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 entry for myapp.sthings.lab, got %d in:\n%s", count, result)
	}
}

func TestMergeDNSEntry_PreservesOtherEntries(t *testing.T) {
	existing := "address=/other.sthings.lab/10.31.103.7"
	result := mergeDNSEntry(existing, "address=/myapp.sthings.lab/10.31.103.6", "myapp.sthings.lab")

	if !strings.Contains(result, "/other.sthings.lab/") {
		t.Error("other.sthings.lab should be preserved")
	}
	if !strings.Contains(result, "/myapp.sthings.lab/") {
		t.Error("myapp.sthings.lab should be present")
	}
}

func TestRemoveDNSEntry_Removes(t *testing.T) {
	existing := "address=/myapp.sthings.lab/10.31.103.6\naddress=/other.sthings.lab/10.31.103.7"
	result := removeDNSEntry(existing, "myapp.sthings.lab")

	if strings.Contains(result, "/myapp.sthings.lab/") {
		t.Error("myapp.sthings.lab should be removed")
	}
	if !strings.Contains(result, "/other.sthings.lab/") {
		t.Error("other.sthings.lab should be preserved")
	}
}

func TestRemoveDNSEntry_NotPresent(t *testing.T) {
	existing := "address=/other.sthings.lab/10.31.103.7"
	result := removeDNSEntry(existing, "myapp.sthings.lab")
	if result != existing {
		t.Errorf("expected unchanged output, got %q", result)
	}
}

// ── Unit tests: mock executor (no network) ────────────────────────────────────

// fakeExecutor implements SSHExecutor in memory — no SSH, no network.
type fakeExecutor struct {
	nvram map[string]string
	calls []string
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{nvram: map[string]string{"dnsmasq_options": ""}}
}

func (f *fakeExecutor) Run(cmd string) (string, error) {
	f.calls = append(f.calls, cmd)
	// Handle compound commands (cmd1 && cmd2 && cmd3)
	for _, part := range strings.Split(cmd, "&&") {
		part = strings.TrimSpace(part)
		switch {
		case part == "nvram get dnsmasq_options":
			return f.nvram["dnsmasq_options"], nil
		case strings.HasPrefix(part, "nvram set dnsmasq_options="):
			val := strings.TrimPrefix(part, "nvram set dnsmasq_options=")
			val = strings.Trim(val, "'")
			f.nvram["dnsmasq_options"] = val
		case part == "nvram commit", part == "restart_dnsmasq":
			// no-op
		}
	}
	return "", nil
}

func (f *fakeExecutor) Close() error { return nil }

func TestDDWRTClient_CreateRecord_Mock(t *testing.T) {
	exec := newFakeExecutor()
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	if err := client.CreateRecord("myapp", "10.31.103.6"); err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}

	opts := exec.nvram["dnsmasq_options"]
	if !strings.Contains(opts, "address=/myapp.sthings.lab/10.31.103.6") {
		t.Errorf("expected entry in dnsmasq_options, got: %q", opts)
	}
}

func TestDDWRTClient_CreateRecord_Idempotent_Mock(t *testing.T) {
	exec := newFakeExecutor()
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	client.CreateRecord("myapp", "10.31.103.5")
	client.CreateRecord("myapp", "10.31.103.6") // update IP

	opts := exec.nvram["dnsmasq_options"]
	count := strings.Count(opts, "/myapp.sthings.lab/")
	if count != 1 {
		t.Errorf("expected exactly 1 entry for myapp, got %d in: %q", count, opts)
	}
	if !strings.Contains(opts, "10.31.103.6") {
		t.Errorf("expected updated IP 10.31.103.6, got: %q", opts)
	}
}

func TestDDWRTClient_DeleteRecord_Mock(t *testing.T) {
	exec := newFakeExecutor()
	exec.nvram["dnsmasq_options"] = "address=/myapp.sthings.lab/10.31.103.6\naddress=/other.sthings.lab/10.31.103.7"

	client := newDDWRTClientWithExecutor("sthings.lab", exec)
	if err := client.DeleteRecord("myapp"); err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}

	opts := exec.nvram["dnsmasq_options"]
	if strings.Contains(opts, "/myapp.sthings.lab/") {
		t.Errorf("myapp.sthings.lab should be removed, got: %q", opts)
	}
	if !strings.Contains(opts, "/other.sthings.lab/") {
		t.Errorf("other.sthings.lab should be preserved, got: %q", opts)
	}
}

func TestDDWRTClient_MultipleRecords_Mock(t *testing.T) {
	exec := newFakeExecutor()
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	client.CreateRecord("app1", "10.31.103.6")
	client.CreateRecord("app2", "10.31.103.7")
	client.CreateRecord("app3", "10.31.103.8")

	opts := exec.nvram["dnsmasq_options"]
	for _, expected := range []string{
		"address=/app1.sthings.lab/10.31.103.6",
		"address=/app2.sthings.lab/10.31.103.7",
		"address=/app3.sthings.lab/10.31.103.8",
	} {
		if !strings.Contains(opts, expected) {
			t.Errorf("missing %q in: %q", expected, opts)
		}
	}
}

// ── Integration tests: fake SSH server (real SSH stack, fake nvram) ───────────

func TestDDWRTClient_CreateRecord_FakeSSH(t *testing.T) {
	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	defer srv.Close()

	client := &DDWRTClient{
		Host:     srv.Addr,
		User:     "root",
		Password: "testpass",
		Zone:     "sthings.lab",
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
	}

	if err := client.CreateRecord("myapp", "10.31.103.6"); err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}

	opts := srv.NvramGet("dnsmasq_options")
	if !strings.Contains(opts, "address=/myapp.sthings.lab/10.31.103.6") {
		t.Errorf("expected DNS entry in nvram, got: %q", opts)
	}
}

func TestDDWRTClient_DeleteRecord_FakeSSH(t *testing.T) {
	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	defer srv.Close()

	// Pre-populate nvram
	srv.NvramSet("dnsmasq_options",
		"address=/myapp.sthings.lab/10.31.103.6\naddress=/other.sthings.lab/10.31.103.7")

	client := &DDWRTClient{
		Host:     srv.Addr,
		User:     "root",
		Password: "testpass",
		Zone:     "sthings.lab",
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
	}

	if err := client.DeleteRecord("myapp"); err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}

	opts := srv.NvramGet("dnsmasq_options")
	if strings.Contains(opts, "/myapp.sthings.lab/") {
		t.Errorf("entry should be gone, got: %q", opts)
	}
	if !strings.Contains(opts, "/other.sthings.lab/") {
		t.Errorf("other entry should be preserved, got: %q", opts)
	}
}

func TestDDWRTClient_Idempotent_FakeSSH(t *testing.T) {
	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	defer srv.Close()

	client := &DDWRTClient{
		Host:     srv.Addr,
		User:     "root",
		Password: "testpass",
		Zone:     "sthings.lab",
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
	}

	// Create twice — second call should update, not duplicate
	client.CreateRecord("myapp", "10.31.103.5")
	client.CreateRecord("myapp", "10.31.103.6")

	opts := srv.NvramGet("dnsmasq_options")
	count := strings.Count(opts, "/myapp.sthings.lab/")
	if count != 1 {
		t.Errorf("expected 1 entry, got %d in: %q", count, opts)
	}
	if !strings.Contains(opts, "10.31.103.6") {
		t.Errorf("expected updated IP, got: %q", opts)
	}
}

func TestFakeDDWRTServer_WrongPassword(t *testing.T) {
	srv, err := NewFakeDDWRTServer("root", "correctpass")
	if err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	defer srv.Close()

	client := &DDWRTClient{
		Host:     srv.Addr,
		User:     "root",
		Password: "wrongpass",
		Zone:     "sthings.lab",
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
	}

	err = client.CreateRecord("myapp", "10.31.103.6")
	if err == nil {
		t.Error("expected auth error with wrong password")
	}
}
