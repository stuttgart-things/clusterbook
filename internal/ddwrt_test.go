/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"fmt"
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

func TestLookupDNSEntry_Found(t *testing.T) {
	existing := "address=/myapp.sthings.lab/10.31.103.6\naddress=/other.sthings.lab/10.31.103.7"
	if ip := lookupDNSEntry(existing, "myapp.sthings.lab"); ip != "10.31.103.6" {
		t.Errorf("expected 10.31.103.6, got %q", ip)
	}
}

func TestLookupDNSEntry_NotFound(t *testing.T) {
	existing := "address=/other.sthings.lab/10.31.103.7"
	if ip := lookupDNSEntry(existing, "myapp.sthings.lab"); ip != "" {
		t.Errorf("expected empty result, got %q", ip)
	}
}

// ── Unit tests: mock executor (no network) ────────────────────────────────────

// mustCreateRecord fails the test if the record could not be written.
func mustCreateRecord(t *testing.T, c *DDWRTClient, hostname, ip string) {
	t.Helper()
	if err := c.CreateRecord(hostname, ip); err != nil {
		t.Fatalf("CreateRecord(%s, %s): %v", hostname, ip, err)
	}
}

// fakeExecutor implements SSHExecutor in memory — no SSH, no network.
type fakeExecutor struct {
	nvram map[string]string
	calls []string

	// fail lists commands that exit 127, so a test can model a router where
	// restart_dnsmasq is not resolvable over non-interactive SSH (issue #187).
	fail map[string]bool

	// reloads counts the reload commands that succeeded.
	reloads int
}

func newFakeExecutor(failing ...string) *fakeExecutor {
	f := &fakeExecutor{
		nvram: map[string]string{"dnsmasq_options": ""},
		fail:  map[string]bool{},
	}
	for _, cmd := range failing {
		f.fail[cmd] = true
	}
	return f
}

func (f *fakeExecutor) Run(cmd string) (string, error) {
	f.calls = append(f.calls, cmd)

	if f.fail[cmd] {
		return "", fmt.Errorf("Process exited with status 127")
	}

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
		case part == "nvram commit", part == "stopservice dnsmasq":
			// no-op
		case part == "restart_dnsmasq", part == "startservice dnsmasq", part == "killall -HUP dnsmasq":
			f.reloads++
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

	mustCreateRecord(t, client, "myapp", "10.31.103.5")
	mustCreateRecord(t, client, "myapp", "10.31.103.6") // update IP

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

	mustCreateRecord(t, client, "app1", "10.31.103.6")
	mustCreateRecord(t, client, "app2", "10.31.103.7")
	mustCreateRecord(t, client, "app3", "10.31.103.8")

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

func TestDDWRTClient_TestDNS_Match_Mock(t *testing.T) {
	exec := newFakeExecutor()
	client := newDDWRTClientWithExecutor("sthings.lab", exec)
	mustCreateRecord(t, client, "myapp", "10.31.103.6")

	fqdn, resolved, match, err := client.TestDNS("myapp", "10.31.103.6")
	if err != nil {
		t.Fatalf("TestDNS failed: %v", err)
	}
	if fqdn != "myapp.sthings.lab" {
		t.Errorf("unexpected fqdn: %q", fqdn)
	}
	if resolved != "10.31.103.6" || !match {
		t.Errorf("expected match on 10.31.103.6, got resolved=%q match=%v", resolved, match)
	}
}

func TestDDWRTClient_TestDNS_Mismatch_Mock(t *testing.T) {
	exec := newFakeExecutor()
	client := newDDWRTClientWithExecutor("sthings.lab", exec)
	mustCreateRecord(t, client, "myapp", "10.31.103.6")

	_, resolved, match, err := client.TestDNS("myapp", "10.31.103.99")
	if err != nil {
		t.Fatalf("TestDNS failed: %v", err)
	}
	if match {
		t.Error("expected no match for differing IP")
	}
	if resolved != "10.31.103.6" {
		t.Errorf("expected resolved 10.31.103.6, got %q", resolved)
	}
}

func TestDDWRTClient_TestDNS_Missing_Mock(t *testing.T) {
	exec := newFakeExecutor()
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	if _, _, _, err := client.TestDNS("myapp", "10.31.103.6"); err == nil {
		t.Error("expected error when no entry exists")
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
	mustCreateRecord(t, client, "myapp", "10.31.103.5")
	mustCreateRecord(t, client, "myapp", "10.31.103.6")

	opts := srv.NvramGet("dnsmasq_options")
	count := strings.Count(opts, "/myapp.sthings.lab/")
	if count != 1 {
		t.Errorf("expected 1 entry, got %d in: %q", count, opts)
	}
	if !strings.Contains(opts, "10.31.103.6") {
		t.Errorf("expected updated IP, got: %q", opts)
	}
}

func TestDDWRTClient_TestDNS_FakeSSH(t *testing.T) {
	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	defer srv.Close()

	srv.NvramSet("dnsmasq_options", "address=/myapp.sthings.lab/10.31.103.6")

	client := &DDWRTClient{
		Host:     srv.Addr,
		User:     "root",
		Password: "testpass",
		Zone:     "sthings.lab",
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
	}

	fqdn, resolved, match, err := client.TestDNS("myapp", "10.31.103.6")
	if err != nil {
		t.Fatalf("TestDNS failed: %v", err)
	}
	if fqdn != "myapp.sthings.lab" || resolved != "10.31.103.6" || !match {
		t.Errorf("unexpected result: fqdn=%q resolved=%q match=%v", fqdn, resolved, match)
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

// ── Issue #187: the write must not be one `&&` chain ─────────────────────────

// TestDDWRTClient_WriteRunsStepsSeparately pins the shape of the write. The old
// `set && commit && restart_dnsmasq` chain reported one exit 127 that named
// none of the three, which is what made the router's split state undiagnosable.
func TestDDWRTClient_WriteRunsStepsSeparately(t *testing.T) {
	exec := newFakeExecutor()
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	if err := client.CreateRecord("myapp", "10.31.103.6"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"nvram get dnsmasq_options",
		"nvram set dnsmasq_options='address=/myapp.sthings.lab/10.31.103.6'",
		"nvram commit",
		"restart_dnsmasq",
	}
	if len(exec.calls) != len(want) {
		t.Fatalf("expected %d separate commands, got %d: %q", len(want), len(exec.calls), exec.calls)
	}
	for i, cmd := range want {
		if exec.calls[i] != cmd {
			t.Errorf("call %d: expected %q, got %q", i, cmd, exec.calls[i])
		}
		if strings.Contains(exec.calls[i], "&&") && !strings.HasPrefix(exec.calls[i], "stopservice") {
			t.Errorf("call %d must not chain commands with &&: %q", i, exec.calls[i])
		}
	}
}

// TestDDWRTClient_FallsBackWhenRestartDnsmasqMissing is the exact reported
// environment: restart_dnsmasq exits 127, everything else works.
func TestDDWRTClient_FallsBackWhenRestartDnsmasqMissing(t *testing.T) {
	exec := newFakeExecutor("restart_dnsmasq")
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	if err := client.CreateRecord("myapp", "10.31.103.6"); err != nil {
		t.Fatalf("expected the fallback reload to succeed, got: %v", err)
	}

	if exec.nvram["dnsmasq_options"] != "address=/myapp.sthings.lab/10.31.103.6" {
		t.Errorf("nvram not written: %q", exec.nvram["dnsmasq_options"])
	}
	if exec.reloads == 0 {
		t.Error("dnsmasq was never reloaded — the record would not be served")
	}
}

// TestDDWRTClient_FallsBackToKillall exercises the last resort in the chain.
func TestDDWRTClient_FallsBackToKillall(t *testing.T) {
	exec := newFakeExecutor("restart_dnsmasq", "stopservice dnsmasq && startservice dnsmasq")
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	if err := client.CreateRecord("myapp", "10.31.103.6"); err != nil {
		t.Fatalf("expected killall -HUP to succeed, got: %v", err)
	}
	if exec.reloads != 1 {
		t.Errorf("expected exactly one successful reload, got %d", exec.reloads)
	}
}

// TestDDWRTClient_ReloadFailureIsNamedAndExplained checks the error text when
// no reload command works: it must say the step and that NVRAM was committed,
// because that is the split state an operator has to reconcile by hand.
func TestDDWRTClient_ReloadFailureIsNamedAndExplained(t *testing.T) {
	exec := newFakeExecutor("restart_dnsmasq", "stopservice dnsmasq && startservice dnsmasq", "killall -HUP dnsmasq")
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	err := client.CreateRecord("myapp", "10.31.103.6")
	if err == nil {
		t.Fatal("expected an error when no reload command works")
	}

	for _, want := range []string{"reload dnsmasq", "nvram committed", "restart_dnsmasq", "killall -HUP dnsmasq"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// TestDDWRTClient_FailingStepIsNamed makes sure a failure in an earlier step is
// not reported as the generic "write dnsmasq_options" it used to be.
func TestDDWRTClient_FailingStepIsNamed(t *testing.T) {
	tests := []struct {
		name    string
		failing string
		want    string
	}{
		{"commit", "nvram commit", "ddwrt nvram commit"},
		{"set", "nvram set dnsmasq_options='address=/myapp.sthings.lab/10.31.103.6'", "ddwrt nvram set dnsmasq_options"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := newFakeExecutor(tt.failing)
			client := newDDWRTClientWithExecutor("sthings.lab", exec)

			err := client.CreateRecord("myapp", "10.31.103.6")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("expected error naming %q, got: %v", tt.want, err)
			}
		})
	}
}

// TestDDWRTClient_DeleteRecordReloads guards the delete half: a record removed
// from NVRAM but still served is exactly the stale-record case from the issue.
func TestDDWRTClient_DeleteRecordReloads(t *testing.T) {
	exec := newFakeExecutor()
	exec.nvram["dnsmasq_options"] = "address=/myapp.sthings.lab/10.31.103.6"
	client := newDDWRTClientWithExecutor("sthings.lab", exec)

	if err := client.DeleteRecord("myapp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.nvram["dnsmasq_options"] != "" {
		t.Errorf("entry not removed: %q", exec.nvram["dnsmasq_options"])
	}
	if exec.reloads == 0 {
		t.Error("dnsmasq was never reloaded — the record would still be served")
	}
}

// ── Fake SSH server: the same behaviour over a real SSH stack ────────────────

func TestDDWRTClient_RestartDnsmasqNotFound_FakeSSH(t *testing.T) {
	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	defer srv.Close()

	// The router in issue #187: restart_dnsmasq is not resolvable, exit 127.
	srv.FailCommand("restart_dnsmasq")

	client := &DDWRTClient{
		Host:     srv.Addr,
		User:     "root",
		Password: "testpass",
		Zone:     "sthings.lab",
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
	}

	if err := client.CreateRecord("dnstest-probe", "192.168.10.173"); err != nil {
		t.Fatalf("expected the fallback reload to carry the write through, got: %v", err)
	}

	if got := srv.NvramGet("dnsmasq_options"); got != "address=/dnstest-probe.sthings.lab/192.168.10.173" {
		t.Errorf("unexpected nvram content: %q", got)
	}

	reloads, last := srv.Reloads()
	if reloads == 0 {
		t.Fatal("dnsmasq was never reloaded")
	}
	if last == "restart_dnsmasq" {
		t.Errorf("reload should have come from a fallback, got %q", last)
	}
}

func TestDDWRTClient_NoReloadCommandWorks_FakeSSH(t *testing.T) {
	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	defer srv.Close()

	for _, cmd := range dnsmasqReloadCommands {
		for _, part := range strings.Split(cmd, "&&") {
			srv.FailCommand(strings.TrimSpace(part))
		}
	}

	client := &DDWRTClient{
		Host:     srv.Addr,
		User:     "root",
		Password: "testpass",
		Zone:     "sthings.lab",
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
	}

	err = client.CreateRecord("dnstest-probe", "192.168.10.173")
	if err == nil {
		t.Fatal("expected an error when dnsmasq cannot be reloaded")
	}
	if !strings.Contains(err.Error(), "nvram committed") {
		t.Errorf("error must state that nvram was committed, got: %v", err)
	}

	// The split state the issue describes: NVRAM holds the new value while the
	// running daemon does not. The error is what makes it visible.
	if got := srv.NvramGet("dnsmasq_options"); got == "" {
		t.Error("expected nvram to hold the committed value")
	}
	if reloads, _ := srv.Reloads(); reloads != 0 {
		t.Errorf("expected no successful reload, got %d", reloads)
	}
}
