package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"installer/internal/command"
	"installer/internal/runner"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSH runs commands on a remote host over SSH using public-key authentication.
type SSH struct {
	Host    string // "host" or "host:port" — defaults to port 22
	User    string
	KeyPath string // path to ed25519 private key file
}

func (e *SSH) Run(ctx context.Context, cmd command.Command, opts ExecOptions) (command.Result, error) {
	auth, closeAgent, err := e.authMethods()
	if err != nil {
		return command.Result{}, err
	}
	defer closeAgent()

	config := &ssh.ClientConfig{
		User:            e.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: replace with TOFU once host key pinning is implemented
	}

	host := e.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "22")
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return command.Result{}, fmt.Errorf("ssh dial %s: %w", host, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return command.Result{}, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	if opts.Stdin != nil {
		session.Stdin = opts.Stdin
	}

	args := append([]string{cmd.Name()}, cmd.Args()...)
	escaped := make([]string, len(args))
	for i, a := range args {
		escaped[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}
	cmdStr := strings.Join(escaped, " ")

	var output bytes.Buffer
	w := io.MultiWriter(&output, runner.Output(ctx))
	session.Stdout = w
	session.Stderr = w

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			session.Signal(ssh.SIGTERM)
		case <-done:
		}
	}()
	defer close(done)

	err = session.Run(cmdStr)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return command.Result{}, err
		}
	}

	return command.Result{
		Output:   output.String(),
		ExitCode: exitCode,
	}, nil
}

// authMethods prefers keys held by ssh-agent (SSH_AUTH_SOCK), which also covers
// passphrase-protected keys, and falls back to the unencrypted key at KeyPath.
func (e *SSH) authMethods() ([]ssh.AuthMethod, func(), error) {
	var methods []ssh.AuthMethod
	closeFn := func() {}

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
			closeFn = func() { conn.Close() }
		}
	}

	keyBytes, readErr := os.ReadFile(e.KeyPath)
	if readErr == nil {
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err == nil {
			methods = append(methods, ssh.PublicKeys(signer))
		} else if len(methods) == 0 {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, closeFn, fmt.Errorf("key %s is passphrase protected: add it to ssh-agent (ssh-add %s) or use an unencrypted key", e.KeyPath, e.KeyPath)
			}
			return nil, closeFn, fmt.Errorf("parse key %s: %w", e.KeyPath, err)
		}
	} else if len(methods) == 0 {
		return nil, closeFn, fmt.Errorf("read key %s: %w", e.KeyPath, readErr)
	}
	return methods, closeFn, nil
}
