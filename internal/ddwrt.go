/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"golang.org/x/crypto/ssh"
)

// SSHExecutor abstracts SSH command execution.
// Production uses realSSHExecutor; tests inject fakeSSHExecutor or fakeDDWRTServer.
type SSHExecutor interface {
	Run(cmd string) (string, error)
	Close() error
}

// DDWRTClient holds connection config and an optional injected executor.
type DDWRTClient struct {
	Host     string
	User     string
	Password string
	Zone     string
	logger   *pterm.Logger
	executor SSHExecutor // nil in production → real SSH created per call
}

// NewDDWRTClient constructs a DDWRTClient from env-style params.
// Returns nil when ddwrtEnabled != "true", mirroring NewPDNSClient.
func NewDDWRTClient(ddwrtEnabled, host, user, password, zone string) *DDWRTClient {
	logger := pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)

	if strings.ToLower(ddwrtEnabled) != "true" {
		logger.Info("DDWRT INTEGRATION DISABLED")
		return nil
	}
	if host == "" || user == "" || password == "" || zone == "" {
		logger.Warn("DDWRT ENABLED BUT MISSING CONFIG (DDWRT_HOST/DDWRT_USER/DDWRT_PASSWORD/DDWRT_ZONE)")
		return nil
	}

	logger.Info("DDWRT INTEGRATION ENABLED", logger.Args("host", host, "zone", zone))
	return &DDWRTClient{Host: host, User: user, Password: password, Zone: zone, logger: logger}
}

// newDDWRTClientWithExecutor is used in tests to inject a fake SSHExecutor.
func newDDWRTClientWithExecutor(zone string, exec SSHExecutor) *DDWRTClient {
	return &DDWRTClient{
		Zone:     zone,
		logger:   pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace),
		executor: exec,
	}
}

// CreateRecord adds/updates a dnsmasq address entry on DD-WRT via SSH.
// Errors are logged here (not just returned) so failures are never silent,
// mirroring how PDNSClient self-logs inside patchZone.
func (d *DDWRTClient) CreateRecord(hostname, ip string) (err error) {
	fqdn := fmt.Sprintf("%s.%s", hostname, d.Zone)
	newEntry := fmt.Sprintf("address=/%s/%s", fqdn, ip)
	d.logger.Info("DDWRT CREATE DNS RECORD", d.logger.Args("fqdn", fqdn, "ip", ip))
	defer func() {
		if err != nil {
			d.logger.Error("DDWRT CREATE DNS RECORD FAILED", d.logger.Args("fqdn", fqdn, "ip", ip, "err", err.Error()))
		}
	}()

	exec, cleanup, err := d.getExecutor()
	if err != nil {
		return fmt.Errorf("ddwrt ssh connect: %w", err)
	}
	defer cleanup()

	existing, err := exec.Run("nvram get dnsmasq_options")
	if err != nil {
		return fmt.Errorf("ddwrt read dnsmasq_options: %w", err)
	}

	updated := mergeDNSEntry(existing, newEntry, fqdn)
	setCmd := fmt.Sprintf("nvram set dnsmasq_options='%s' && nvram commit && restart_dnsmasq", updated)

	if _, err := exec.Run(setCmd); err != nil {
		return fmt.Errorf("ddwrt write dnsmasq_options: %w", err)
	}

	d.logger.Info("DDWRT DNS RECORD CREATED", d.logger.Args("entry", newEntry))
	return nil
}

// DeleteRecord removes a dnsmasq address entry from DD-WRT via SSH.
// Errors are logged here (not just returned) so failures are never silent.
func (d *DDWRTClient) DeleteRecord(hostname string) (err error) {
	fqdn := fmt.Sprintf("%s.%s", hostname, d.Zone)
	d.logger.Info("DDWRT DELETE DNS RECORD", d.logger.Args("fqdn", fqdn))
	defer func() {
		if err != nil {
			d.logger.Error("DDWRT DELETE DNS RECORD FAILED", d.logger.Args("fqdn", fqdn, "err", err.Error()))
		}
	}()

	exec, cleanup, err := d.getExecutor()
	if err != nil {
		return fmt.Errorf("ddwrt ssh connect: %w", err)
	}
	defer cleanup()

	existing, err := exec.Run("nvram get dnsmasq_options")
	if err != nil {
		return fmt.Errorf("ddwrt read dnsmasq_options: %w", err)
	}

	updated := removeDNSEntry(existing, fqdn)
	setCmd := fmt.Sprintf("nvram set dnsmasq_options='%s' && nvram commit && restart_dnsmasq", updated)

	if _, err := exec.Run(setCmd); err != nil {
		return fmt.Errorf("ddwrt write dnsmasq_options: %w", err)
	}

	d.logger.Info("DDWRT DNS RECORD DELETED", d.logger.Args("fqdn", fqdn))
	return nil
}

// TestDNS verifies the dnsmasq address entry for the cluster's FQDN exists on
// DD-WRT and resolves to the expected IP. It reads dnsmasq_options over SSH and
// inspects the managed `address=/{fqdn}/{ip}` record directly — an in-cluster
// net.LookupHost would hit the pod resolver (CoreDNS), not the DD-WRT router.
// Returns (fqdn, resolvedIP, match, error), mirroring PDNSClient.TestDNS.
func (d *DDWRTClient) TestDNS(cluster, expectedIP string) (string, string, bool, error) {
	if d == nil {
		return "", "", false, fmt.Errorf("DDWRT not enabled")
	}

	fqdn := fmt.Sprintf("%s.%s", cluster, d.Zone)

	exec, cleanup, err := d.getExecutor()
	if err != nil {
		return fqdn, "", false, fmt.Errorf("ddwrt ssh connect: %w", err)
	}
	defer cleanup()

	existing, err := exec.Run("nvram get dnsmasq_options")
	if err != nil {
		return fqdn, "", false, fmt.Errorf("ddwrt read dnsmasq_options: %w", err)
	}

	resolved := lookupDNSEntry(existing, fqdn)
	if resolved == "" {
		return fqdn, "", false, fmt.Errorf("no dnsmasq entry for %s", fqdn)
	}

	return fqdn, resolved, resolved == expectedIP, nil
}

// getExecutor returns injected executor (tests) or a fresh real SSH executor.
func (d *DDWRTClient) getExecutor() (SSHExecutor, func(), error) {
	if d.executor != nil {
		return d.executor, func() {}, nil
	}
	real, err := newRealSSHExecutor(d.Host, d.User, d.Password)
	if err != nil {
		return nil, nil, err
	}
	return real, func() { real.Close() }, nil
}

// ── Real SSH executor ────────────────────────────────────────────────────────

type realSSHExecutor struct{ client *ssh.Client }

func newRealSSHExecutor(host, user, password string) (*realSSHExecutor, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := host
	if !strings.Contains(addr, ":") {
		addr += ":22"
	}
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &realSSHExecutor{client: client}, nil
}

func (r *realSSHExecutor) Run(cmd string) (string, error) {
	sess, err := r.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.Output(cmd)
	return strings.TrimSpace(string(out)), err
}

func (r *realSSHExecutor) Close() error { return r.client.Close() }

// ── Pure helper functions (no SSH, fully unit-testable) ──────────────────────

func mergeDNSEntry(existing, newEntry, fqdn string) string {
	var lines []string
	for _, line := range strings.Split(existing, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "/"+fqdn+"/") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(append(lines, newEntry), "\n")
}

// lookupDNSEntry returns the IP of the `address=/{fqdn}/{ip}` entry in
// dnsmasq_options content, or "" if no matching entry exists.
func lookupDNSEntry(existing, fqdn string) string {
	marker := "/" + fqdn + "/"
	for _, line := range strings.Split(existing, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, marker); idx >= 0 {
			return line[idx+len(marker):]
		}
	}
	return ""
}

func removeDNSEntry(existing, fqdn string) string {
	var lines []string
	for _, line := range strings.Split(existing, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "/"+fqdn+"/") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
