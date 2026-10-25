package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSH is a thin wrapper around an ssh.Client scoped to a single profile.
type SSH struct {
	client   *ssh.Client
	useSudo  bool
	sudoPass string
}

func hostPort(p *Profile) string {
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

func readSigner(path, passphrase string) (ssh.Signer, error) {
	if path == "" {
		return nil, errors.New("key path is empty")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(b)
	if err == nil {
		return signer, nil
	}
	var pm *ssh.PassphraseMissingError
	if errors.As(err, &pm) || strings.Contains(err.Error(), "passphrase") {
		return ssh.ParsePrivateKeyWithPassphrase(b, []byte(passphrase))
	}
	return nil, err
}

// dial connects to the profile's host. It performs TOFU on the host key: the
// first successful connection records the fingerprint via onFP; later
// connections must match it.
func dial(p *Profile, onFP func(string)) (*SSH, error) {
	var auth []ssh.AuthMethod
	switch p.AuthType {
	case "key":
		sg, err := readSigner(p.KeyPath, p.KeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("ssh key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(sg))
	default:
		if p.Password == "" {
			return nil, errors.New("password is empty")
		}
		auth = append(auth, ssh.Password(p.Password))
	}

	var gotFP string
	cb := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		gotFP = ssh.FingerprintSHA256(key)
		if p.HostKeyFP != "" && p.HostKeyFP != gotFP {
			return fmt.Errorf("host key mismatch for %s\n  pinned: %s\n  live:   %s", hostname, p.HostKeyFP, gotFP)
		}
		return nil
	}
	cfg := &ssh.ClientConfig{
		User:            p.User,
		Auth:            auth,
		HostKeyCallback: cb,
		Timeout:         20 * time.Second,
	}
	// net.DialTimeout bounds the TCP connect; the handshake needs its own deadline
	// (cfg.Timeout only covers the dial), so a blackholed port cannot hang forever.
	conn, err := net.DialTimeout("tcp", hostPort(p), 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("tcp %s: %w", hostPort(p), err)
	}
	_ = conn.SetDeadline(time.Now().Add(25 * time.Second))
	cc, chans, reqs, err := ssh.NewClientConn(conn, hostPort(p), cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", hostPort(p), err)
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(cc, chans, reqs)
	if onFP != nil && gotFP != "" {
		onFP(gotFP)
	}
	s := &SSH{client: client}
	// Detect whether we need sudo for privileged commands.
	if out, err := s.rawRun("id -u"); err == nil && strings.TrimSpace(out) != "0" {
		s.useSudo = true
		s.sudoPass = p.Password
	}
	return s, nil
}

func (s *SSH) Close() {
	if s.client != nil {
		_ = s.client.Close()
	}
}

// rawRun executes a command without any sudo wrapping.
func (s *SSH) rawRun(cmd string) (string, error) {
	sess, err := s.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

// Run executes a command (wrapped in sudo if the login user is not root) and
// returns the combined output. A non-zero exit is returned as an error.
func (s *SSH) Run(cmd string) (string, error) {
	return s.RunStdin(cmd, nil)
}

// RunStdin is Run with data piped to the command's stdin (after the sudo password).
func (s *SSH) RunStdin(cmd string, in []byte) (string, error) {
	sess, err := s.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	var pieces []byte
	real := cmd
	if s.useSudo {
		real = "sudo -S -p '' bash -c " + shq(cmd)
		if s.sudoPass != "" {
			pieces = append(pieces, []byte(s.sudoPass+"\n")...)
		}
	}
	pieces = append(pieces, in...)

	// For large payloads stream in chunks through a pipe instead of handing a
	// single multi-megabyte reader to the session; this keeps the SSH channel
	// window moving and significantly improves throughput.
	if len(pieces) > 1<<20 {
		stdin, err := sess.StdinPipe()
		if err != nil {
			return "", err
		}
		go func() {
			defer stdin.Close()
			const chunk = 256 * 1024
			for i := 0; i < len(pieces); i += chunk {
				end := i + chunk
				if end > len(pieces) {
					end = len(pieces)
				}
				if _, werr := stdin.Write(pieces[i:end]); werr != nil {
					return
				}
			}
		}()
		out, err := sess.CombinedOutput(real)
		return string(out), err
	}

	if len(pieces) > 0 {
		sess.Stdin = bytes.NewReader(pieces)
	}
	out, err := sess.CombinedOutput(real)
	return string(out), err
}

// MustRun runs a command and, on non-zero exit, returns an error carrying the
// command output (trimmed) for the log.
func (s *SSH) MustRun(cmd string) (string, error) {
	out, err := s.Run(cmd)
	if err != nil {
		return out, fmt.Errorf("command failed: %s\n%s", cmd, strings.TrimSpace(out))
	}
	return out, nil
}

// Upload writes data to remotePath (atomically via a .new file) and chmods it.
// Larger payloads are gzip-compressed in transit (the evilginx binary shrinks to
// roughly a third), falling back to plain base64 if gzip is unavailable.
func (s *SSH) Upload(data []byte, remotePath string, mode os.FileMode) error {
	dir := shq(dirOf(remotePath))
	target := shq(remotePath)
	perm := fmt.Sprintf("%o", mode.Perm())

	try := func(pipe string, body []byte) error {
		cmd := fmt.Sprintf("mkdir -p %s && %s > %s.tmp && mv -f %s.tmp %s && chmod %s %s",
			dir, pipe, target, target, target, perm, target)
		out, err := s.RunStdin(cmd, body)
		if err != nil {
			return fmt.Errorf("upload %s: %w\n%s", remotePath, err, strings.TrimSpace(out))
		}
		return nil
	}

	if len(data) > 64*1024 {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(data)
		_ = zw.Close()
		enc := base64.StdEncoding.EncodeToString(buf.Bytes())
		if err := try("base64 -d | gzip -dc", []byte(enc)); err == nil {
			return nil
		}
	}
	return try("base64 -d", []byte(base64.StdEncoding.EncodeToString(data)))
}

func dirOf(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

// shq single-quotes a string for POSIX shells.
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ReadFile returns the contents of a remote file.
func (s *SSH) ReadFile(path string) ([]byte, error) {
	out, err := s.Run("cat " + shq(path))
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// Test performs a light connectivity + identity probe.
type Probe struct {
	User    string `json:"user"`
	UID     string `json:"uid"`
	Arch    string `json:"arch"`
	OS      string `json:"os"`
	Kernel  string `json:"kernel"`
	MemoryMB int   `json:"memory_mb"`
	DiskFreeMB int64 `json:"disk_free_mb"`
	SSHPort int    `json:"ssh_port"`
}

func (s *SSH) Probe() (*Probe, error) {
	pr := &Probe{}
	if out, err := s.MustRun("id -un"); err == nil {
		pr.User = strings.TrimSpace(out)
	}
	if out, err := s.MustRun("id -u"); err == nil {
		pr.UID = strings.TrimSpace(out)
	}
	if out, _ := s.Run("uname -m"); out != "" {
		pr.Arch = strings.TrimSpace(out)
	}
	if out, _ := s.Run("uname -sr"); out != "" {
		pr.Kernel = strings.TrimSpace(out)
	}
	if out, _ := s.Run(". /etc/os-release 2>/dev/null; echo $PRETTY_NAME"); out != "" {
		pr.OS = strings.TrimSpace(out)
	}
	if out, _ := s.Run("awk '/MemTotal/{print int($2/1024)}' /proc/meminfo"); out != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
			pr.MemoryMB = n
		}
	}
	if out, _ := s.Run("df -Pm / | awk 'NR==2{print $4}'"); out != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64); err == nil {
			pr.DiskFreeMB = n
		}
	}
	return pr, nil
}
