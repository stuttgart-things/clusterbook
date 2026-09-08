/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de

fakeDDWRTServer: an in-process SSH server that simulates DD-WRT nvram commands.
No Docker, no external dependencies — just golang.org/x/crypto/ssh.

Supported commands (matching what DDWRTClient sends):
  nvram get dnsmasq_options          → returns current nvram store value
  nvram set dnsmasq_options='...'    → stores value, parses and tracks entries
  nvram commit                       → no-op (acknowledged)
  restart_dnsmasq                    → reload, unless disabled via FailCommand
  stopservice/startservice dnsmasq   → reload
  killall -HUP dnsmasq               → reload
  compound: cmd1 && cmd2 && cmd3     → executes each part, stopping at the first failure

Unknown commands exit 127 ("command not found"), the way a real DD-WRT shell
does — which is what issue #187 hit for restart_dnsmasq over non-interactive SSH.
*/

package internal

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// FakeDDWRTServer is an in-process SSH server with nvram state.
type FakeDDWRTServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
	mu       sync.Mutex
	nvram    map[string]string // key → value store

	// failing holds commands forced to exit 127 even though they are
	// otherwise supported, so tests can reproduce a router where
	// restart_dnsmasq is not resolvable.
	failing map[string]bool

	// reloads counts successful dnsmasq reloads, and lastReload records the
	// command that performed the most recent one.
	reloads    int
	lastReload string

	// Addr is the local address the server is listening on (host:port).
	Addr string
}

// NewFakeDDWRTServer starts a fake DD-WRT SSH server on a random local port.
// Call Close() when done.
func NewFakeDDWRTServer(user, password string) (*FakeDDWRTServer, error) {
	// Generate a throw-away host key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	srv := &FakeDDWRTServer{
		nvram: map[string]string{
			"dnsmasq_options": "", // empty by default, just like a fresh DD-WRT
		},
		failing: map[string]bool{},
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if conn.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("invalid credentials")
		},
	}
	cfg.AddHostKey(signer)
	srv.config = cfg

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	srv.listener = ln
	srv.Addr = ln.Addr().String()

	go srv.serve()
	return srv, nil
}

// NvramGet returns the current nvram value for key (used in test assertions).
func (s *FakeDDWRTServer) NvramGet(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nvram[key]
}

// NvramSet sets a nvram value directly (used in test setup).
func (s *FakeDDWRTServer) NvramSet(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nvram[key] = value
}

// FailCommand makes cmd exit 127, simulating a router on which that command is
// not resolvable over a non-interactive SSH session.
func (s *FakeDDWRTServer) FailCommand(cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failing[cmd] = true
}

// Reloads returns how many times dnsmasq was reloaded and the command that did
// it last, so tests can assert the running daemon actually picked up the change.
func (s *FakeDDWRTServer) Reloads() (int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloads, s.lastReload
}

// Close shuts down the fake server.
func (s *FakeDDWRTServer) Close() { s.listener.Close() }

// serve accepts SSH connections in a loop.
func (s *FakeDDWRTServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go s.handleConn(conn)
	}
}

func (s *FakeDDWRTServer) handleConn(nConn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, s.config)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		go s.handleSession(ch, requests)
	}
}

func (s *FakeDDWRTServer) handleSession(ch ssh.Channel, requests <-chan *ssh.Request) {
	defer ch.Close()
	for req := range requests {
		if req.Type != "exec" {
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}

		// Decode the command from the exec payload (4-byte length prefix + command)
		if len(req.Payload) < 4 {
			req.Reply(false, nil)
			continue
		}
		cmdLen := int(req.Payload[0])<<24 | int(req.Payload[1])<<16 | int(req.Payload[2])<<8 | int(req.Payload[3])
		if len(req.Payload) < 4+cmdLen {
			req.Reply(false, nil)
			continue
		}
		fullCmd := string(req.Payload[4 : 4+cmdLen])
		req.Reply(true, nil)

		output, status := s.executeCompound(fullCmd)
		ch.Write([]byte(output))

		exitStatus := []byte{
			byte(status >> 24), byte(status >> 16), byte(status >> 8), byte(status),
		}
		ch.SendRequest("exit-status", false, exitStatus)
		return
	}
}

// executeCompound handles "cmd1 && cmd2 && cmd3" style compound commands,
// stopping at the first part that exits non-zero the way a shell's && does.
func (s *FakeDDWRTServer) executeCompound(cmd string) (string, uint32) {
	var output strings.Builder

	for _, part := range strings.Split(cmd, "&&") {
		out, status := s.executeOne(strings.TrimSpace(part))
		output.WriteString(out)
		if status != 0 {
			return output.String(), status
		}
	}

	return output.String(), 0
}

// dnsmasqReloadFakes are the commands that bring the running dnsmasq back up.
// "stopservice dnsmasq" is accepted separately below: it succeeds but is only
// half of a restart, so it must not count as a reload on its own.
var dnsmasqReloadFakes = map[string]bool{
	"restart_dnsmasq":      true,
	"startservice dnsmasq": true,
	"killall -HUP dnsmasq": true,
}

// executeOne handles a single DD-WRT command, returning its output and exit
// status. Status 127 is "command not found".
func (s *FakeDDWRTServer) executeOne(cmd string) (string, uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failing[cmd] {
		return fmt.Sprintf("sh: %s: not found\n", cmd), 127
	}

	switch {
	// nvram get <key>
	case strings.HasPrefix(cmd, "nvram get "):
		key := strings.TrimSpace(strings.TrimPrefix(cmd, "nvram get "))
		return s.nvram[key], 0

	// nvram set key='value'  or  nvram set key=value
	case strings.HasPrefix(cmd, "nvram set "):
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "nvram set "))
		eqIdx := strings.Index(rest, "=")
		if eqIdx < 0 {
			return "", 1
		}
		key := rest[:eqIdx]
		val := rest[eqIdx+1:]
		// Strip surrounding single quotes if present
		val = strings.TrimPrefix(val, "'")
		val = strings.TrimSuffix(val, "'")
		s.nvram[key] = val
		return "", 0

	// nvram commit → no-op
	case cmd == "nvram commit":
		return "", 0

	// Half of a restart: succeeds, but does not bring dnsmasq back up.
	case cmd == "stopservice dnsmasq":
		return "", 0

	// Any of the reload variants → record that dnsmasq picked up the change.
	case dnsmasqReloadFakes[cmd]:
		s.reloads++
		s.lastReload = cmd
		return "", 0

	default:
		return fmt.Sprintf("sh: %s: not found\n", cmd), 127
	}
}
