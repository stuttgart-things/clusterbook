/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de

fakeDDWRTServer: an in-process SSH server that simulates DD-WRT nvram commands.
No Docker, no external dependencies — just golang.org/x/crypto/ssh.

Supported commands (matching what DDWRTClient sends):
  nvram get dnsmasq_options          → returns current nvram store value
  nvram set dnsmasq_options='...'    → stores value, parses and tracks entries
  nvram commit                       → no-op (acknowledged)
  restart_dnsmasq                    → no-op (acknowledged)
  compound: cmd1 && cmd2 && cmd3     → executes each part in sequence
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

		output := s.executeCompound(fullCmd)
		ch.Write([]byte(output))

		// Send exit-status 0
		exitStatus := []byte{0, 0, 0, 0}
		ch.SendRequest("exit-status", false, exitStatus)
		return
	}
}

// executeCompound handles "cmd1 && cmd2 && cmd3" style compound commands.
func (s *FakeDDWRTServer) executeCompound(cmd string) string {
	parts := strings.Split(cmd, "&&")
	var output strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		out := s.executeOne(part)
		if out != "" {
			output.WriteString(out)
		}
	}
	return output.String()
}

// executeOne handles a single DD-WRT nvram command.
func (s *FakeDDWRTServer) executeOne(cmd string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	// nvram get <key>
	case strings.HasPrefix(cmd, "nvram get "):
		key := strings.TrimPrefix(cmd, "nvram get ")
		key = strings.TrimSpace(key)
		return s.nvram[key]

	// nvram set key='value'  or  nvram set key=value
	case strings.HasPrefix(cmd, "nvram set "):
		rest := strings.TrimPrefix(cmd, "nvram set ")
		rest = strings.TrimSpace(rest)
		eqIdx := strings.Index(rest, "=")
		if eqIdx < 0 {
			return ""
		}
		key := rest[:eqIdx]
		val := rest[eqIdx+1:]
		// Strip surrounding single quotes if present
		val = strings.TrimPrefix(val, "'")
		val = strings.TrimSuffix(val, "'")
		s.nvram[key] = val
		return ""

	// nvram commit → no-op
	case cmd == "nvram commit":
		return ""

	// restart_dnsmasq → no-op
	case cmd == "restart_dnsmasq":
		return ""

	default:
		return fmt.Sprintf("sh: %s: not found\n", cmd)
	}
}
