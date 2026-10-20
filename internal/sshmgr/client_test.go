package sshmgr

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type testServerOptions struct {
	password      string
	authorizedKey ssh.PublicKey
}

type testSSHServer struct {
	listener net.Listener
	hostKey  ssh.PublicKey
	wait     sync.WaitGroup
}

func startTestSSHServer(t *testing.T, options testServerOptions) *testSSHServer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	config := &ssh.ServerConfig{}
	if options.password != "" {
		config.PasswordCallback = func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if metadata.User() == "tester" && string(password) == options.password {
				return nil, nil
			}
			return nil, fmt.Errorf("denied")
		}
	}
	if options.authorizedKey != nil {
		config.PublicKeyCallback = func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if metadata.User() == "tester" && bytes.Equal(key.Marshal(), options.authorizedKey.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("denied")
		}
	}
	config.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &testSSHServer{listener: listener, hostKey: hostSigner.PublicKey()}
	go server.serve(config)
	t.Cleanup(server.Close)
	return server
}

func (s *testSSHServer) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *testSSHServer) Close() {
	_ = s.listener.Close()
	s.wait.Wait()
}

func (s *testSSHServer) serve(config *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wait.Add(1)
		go func() {
			defer s.wait.Done()
			s.handleConnection(conn, config)
		}()
	}
}

func (s *testSSHServer) handleConnection(conn net.Conn, config *ssh.ServerConfig) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(requests)
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go handleTestSession(channel, channelRequests)
	}
}

func handleTestSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		var payload struct {
			Command string
		}
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			_ = request.Reply(false, nil)
			continue
		}
		_ = request.Reply(true, nil)
		status := uint32(0)
		switch {
		case strings.Contains(payload.Command, "exit 7"):
			status = 7
		case payload.Command == "echo ops-ok":
			_, _ = channel.Write([]byte("ops-ok\n"))
		case strings.Contains(payload.Command, "stdin-echo"):
			data, _ := io.ReadAll(channel)
			_, _ = channel.Write([]byte("got:" + string(data)))
		case strings.Contains(payload.Command, "stderr"):
			_, _ = channel.Stderr().Write([]byte("stderr-line\n"))
		case strings.Contains(payload.Command, "slow"):
			time.Sleep(800 * time.Millisecond)
			_, _ = channel.Write([]byte("slow-done\n"))
		default:
			_, _ = channel.Write([]byte("remote:" + payload.Command + "\n"))
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct {
			Status uint32
		}{Status: status}))
		return
	}
}

func TestDialerPasswordAuthAndExec(t *testing.T) {
	server := startTestSSHServer(t, testServerOptions{password: "secret"})
	dialer := NewDialer(t.TempDir())
	strict := false
	host := Host{
		Name:          "password-host",
		Address:       "127.0.0.1",
		Port:          server.Port(),
		User:          "tester",
		Auth:          AuthPassword,
		Password:      "secret",
		StrictHostKey: &strict,
	}

	probe, err := dialer.Check(context.Background(), host, 5*time.Second)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if probe.Fingerprint == "" {
		t.Fatal("expected host key fingerprint")
	}

	result, err := dialer.Exec(context.Background(), host, 5*time.Second, "echo hello")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(result.Stdout, "remote:echo hello") || result.ExitCode != 0 {
		t.Fatalf("unexpected exec result: %+v", result)
	}

	exited, err := dialer.Exec(context.Background(), host, 5*time.Second, "exit 7")
	if err != nil {
		t.Fatalf("exec exit 7: %v", err)
	}
	if exited.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", exited.ExitCode)
	}

	failing := host
	failing.Password = "wrong"
	if _, err := dialer.Check(context.Background(), failing, 5*time.Second); err == nil {
		t.Fatal("expected authentication failure")
	}
}

func TestDialerKeyAuthWithPassphrase(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate user key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("user signer: %v", err)
	}
	server := startTestSSHServer(t, testServerOptions{authorizedKey: signer.PublicKey()})
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "test", []byte("passphrase"))
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	strict := false
	host := Host{
		Name:          "key-host",
		Address:       "127.0.0.1",
		Port:          server.Port(),
		User:          "tester",
		Auth:          AuthKey,
		KeyPath:       keyPath,
		Passphrase:    "passphrase",
		StrictHostKey: &strict,
	}
	dialer := NewDialer(t.TempDir())
	if _, err := dialer.Check(context.Background(), host, 5*time.Second); err != nil {
		t.Fatalf("check: %v", err)
	}

	host.Passphrase = "wrong"
	if _, err := dialer.Check(context.Background(), host, 5*time.Second); err == nil {
		t.Fatal("expected wrong passphrase failure")
	}
}

func TestDialerStream(t *testing.T) {
	server := startTestSSHServer(t, testServerOptions{password: "secret"})
	dialer := NewDialer(t.TempDir())
	strict := false
	host := Host{
		Name:          "stream-host",
		Address:       "127.0.0.1",
		Port:          server.Port(),
		User:          "tester",
		Auth:          AuthPassword,
		Password:      "secret",
		StrictHostKey: &strict,
	}
	var stdout, stderr bytes.Buffer
	result, err := dialer.Stream(context.Background(), host, 5*time.Second, "stdin-echo", strings.NewReader("bundle-data"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if stdout.String() != "got:bundle-data" {
		t.Fatalf("stdout = %q", stdout.String())
	}

	stdout.Reset()
	result, err = dialer.Stream(context.Background(), host, 5*time.Second, "exit 7", nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("stream exit 7: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}

	stdout.Reset()
	if _, err := dialer.Stream(context.Background(), host, 400*time.Millisecond, "slow-task", nil, &stdout, &stderr); err != nil {
		t.Fatalf("stream should outlive connect timeout: %v", err)
	}
	if stdout.String() != "slow-done\n" {
		t.Fatalf("slow stdout = %q", stdout.String())
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dialer.Stream(cancelled, host, 5*time.Second, "stdin-echo", strings.NewReader("x"), io.Discard, io.Discard); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestDialerStrictHostKey(t *testing.T) {
	server := startTestSSHServer(t, testServerOptions{password: "secret"})
	dialer := NewDialer(t.TempDir())
	strict := true
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{net.JoinHostPort("127.0.0.1", strconv.Itoa(server.Port()))}, server.hostKey)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	host := Host{
		Name:           "strict-host",
		Address:        "127.0.0.1",
		Port:           server.Port(),
		User:           "tester",
		Auth:           AuthPassword,
		Password:       "secret",
		StrictHostKey:  &strict,
		KnownHostsPath: knownHostsPath,
	}
	if _, err := dialer.Check(context.Background(), host, 5*time.Second); err != nil {
		t.Fatalf("check with known host: %v", err)
	}

	host.KnownHostsPath = filepath.Join(t.TempDir(), "missing_known_hosts")
	if _, err := dialer.Check(context.Background(), host, 5*time.Second); err == nil || !strings.Contains(err.Error(), "known_hosts") {
		t.Fatalf("expected known_hosts error, got %v", err)
	}
}
