package sshmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	maxStdoutBytes = 1 << 20
	maxStderrBytes = 256 << 10
)

type ProbeResult struct {
	Latency     time.Duration
	Fingerprint string
}

type Prober interface {
	Check(ctx context.Context, host Host, timeout time.Duration) (ProbeResult, error)
	Exec(ctx context.Context, host Host, timeout time.Duration, command string) (ExecResult, error)
	Stream(ctx context.Context, host Host, timeout time.Duration, command string, stdin io.Reader, stdout, stderr io.Writer) (StreamResult, error)
}

type Dialer struct {
	BaseDir string
}

func NewDialer(baseDir string) *Dialer {
	return &Dialer{BaseDir: baseDir}
}

func (d *Dialer) Check(ctx context.Context, host Host, timeout time.Duration) (ProbeResult, error) {
	start := time.Now()
	client, fingerprint, err := d.dial(ctx, host, timeout)
	if err != nil {
		return ProbeResult{Latency: time.Since(start), Fingerprint: fingerprint}, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return ProbeResult{Latency: time.Since(start), Fingerprint: fingerprint}, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()
	if err := session.Run("echo ops-ok"); err != nil {
		return ProbeResult{Latency: time.Since(start), Fingerprint: fingerprint}, fmt.Errorf("SSH 会话执行失败: %w", err)
	}
	return ProbeResult{Latency: time.Since(start), Fingerprint: fingerprint}, nil
}

func (d *Dialer) Exec(ctx context.Context, host Host, timeout time.Duration, command string) (ExecResult, error) {
	result := ExecResult{}
	if strings.TrimSpace(command) == "" {
		return result, fmt.Errorf("命令不能为空")
	}
	start := time.Now()
	client, fingerprint, err := d.dial(ctx, host, timeout)
	result.Fingerprint = fingerprint
	if err != nil {
		result.LatencyMS = time.Since(start).Milliseconds()
		return result, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		result.LatencyMS = time.Since(start).Milliseconds()
		return result, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()
	stdout := &limitedBuffer{limit: maxStdoutBytes}
	stderr := &limitedBuffer{limit: maxStderrBytes}
	session.Stdout = stdout
	session.Stderr = stderr
	err = session.Run(command)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
			return result, nil
		}
		return result, fmt.Errorf("远程命令执行失败: %w", err)
	}
	return result, nil
}

func (d *Dialer) Stream(ctx context.Context, host Host, timeout time.Duration, command string, stdin io.Reader, stdout, stderr io.Writer) (StreamResult, error) {
	result := StreamResult{}
	if strings.TrimSpace(command) == "" {
		return result, fmt.Errorf("命令不能为空")
	}
	start := time.Now()
	client, fingerprint, err := d.dial(ctx, host, timeout)
	result.Fingerprint = fingerprint
	if err != nil {
		result.LatencyMS = time.Since(start).Milliseconds()
		return result, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		result.LatencyMS = time.Since(start).Milliseconds()
		return result, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
			_ = client.Close()
		case <-done:
		}
	}()
	err = session.Run(command)
	close(done)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
			return result, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, ctxErr
		}
		return result, fmt.Errorf("远程命令执行失败: %w", err)
	}
	return result, nil
}

func (d *Dialer) dial(ctx context.Context, host Host, timeout time.Duration) (*ssh.Client, string, error) {
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}
	address := net.JoinHostPort(host.Address, strconv.Itoa(host.Port))
	dialer := &net.Dialer{Timeout: timeout}
	raw, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, "", fmt.Errorf("连接 %s 失败: %w", address, err)
	}
	_ = raw.SetDeadline(time.Now().Add(timeout))

	var serverKey ssh.PublicKey
	callback, err := d.hostKeyCallback(host, &serverKey)
	if err != nil {
		_ = raw.Close()
		return nil, "", err
	}
	auth, err := d.authMethods(host)
	if err != nil {
		_ = raw.Close()
		return nil, "", err
	}
	clientConfig := &ssh.ClientConfig{
		User:            host.User,
		Auth:            auth,
		HostKeyCallback: callback,
		Timeout:         timeout,
	}
	conn, channels, requests, err := ssh.NewClientConn(raw, address, clientConfig)
	fingerprint := ""
	if serverKey != nil {
		fingerprint = ssh.FingerprintSHA256(serverKey)
	}
	if err != nil {
		_ = raw.Close()
		if fingerprint != "" {
			return nil, fingerprint, fmt.Errorf("%w（服务器主机密钥指纹 %s）", err, fingerprint)
		}
		return nil, fingerprint, err
	}
	// timeout 仅用于连接与握手，长任务会话不能继续沿用该截止时间。
	_ = raw.SetDeadline(time.Time{})
	return ssh.NewClient(conn, channels, requests), fingerprint, nil
}

func (d *Dialer) authMethods(host Host) ([]ssh.AuthMethod, error) {
	switch host.Auth {
	case AuthPassword:
		if host.Password == "" {
			return nil, fmt.Errorf("操作机 %s 未配置密码", host.Name)
		}
		password := host.Password
		return []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for index := range answers {
					answers[index] = password
				}
				return answers, nil
			}),
		}, nil
	case AuthKey:
		signer, err := d.loadSigner(host)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("不支持的认证方式: %s", host.Auth)
	}
}

func (d *Dialer) loadSigner(host Host) (ssh.Signer, error) {
	path := strings.TrimSpace(host.KeyPath)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("无法定位用户目录，请显式填写私钥路径")
		}
		for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			candidate := filepath.Join(home, ".ssh", name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				path = candidate
				break
			}
		}
		if path == "" {
			return nil, fmt.Errorf("未找到默认私钥（~/.ssh/id_ed25519、id_ecdsa、id_rsa），请填写私钥路径")
		}
	} else {
		path = expandPath(d.BaseDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取私钥 %s 失败: %w", path, err)
	}
	if host.Passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(data, []byte(host.Passphrase))
		if err != nil {
			return nil, fmt.Errorf("解析私钥 %s 失败: %w", path, err)
		}
		return signer, nil
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil, fmt.Errorf("私钥 %s 已加密，请填写私钥口令", path)
		}
		return nil, fmt.Errorf("解析私钥 %s 失败: %w", path, err)
	}
	return signer, nil
}

func (d *Dialer) hostKeyCallback(host Host, capture *ssh.PublicKey) (ssh.HostKeyCallback, error) {
	if !host.IsStrictHostKey() {
		return func(_ string, _ net.Addr, key ssh.PublicKey) error {
			*capture = key
			return nil
		}, nil
	}
	path := strings.TrimSpace(host.KnownHostsPath)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("无法定位用户目录，请显式填写 known_hosts 路径或关闭严格校验")
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	} else {
		path = expandPath(d.BaseDir, path)
	}
	verify, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("加载 known_hosts 失败（%s）: %w；可在操作机配置中关闭严格主机密钥校验", path, err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		*capture = key
		if err := verify(hostname, remote, key); err != nil {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
				return fmt.Errorf("主机 %s 不在 known_hosts 中，请先确认指纹并添加，或关闭严格主机密钥校验", hostname)
			}
			return fmt.Errorf("主机密钥校验失败: %w", err)
		}
		return nil
	}, nil
}

func expandPath(baseDir, value string) string {
	path := strings.TrimSpace(value)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil {
			if len(path) == 1 {
				path = home
			} else {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path)
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.buffer.Write(data[:remaining])
		b.truncated = true
		return len(data), nil
	}
	return b.buffer.Write(data)
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}
