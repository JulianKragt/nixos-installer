package pipeline

import (
	"context"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// BootstrapKey connects to addr:22 as root with password, installs
// authorizedKeys into /root/.ssh/authorized_keys, and disconnects.
// It is used only once, before key auth is set up.
// TODO: replace InsecureIgnoreHostKey with TOFU once host key pinning is implemented.
func BootstrapKey(ctx context.Context, addr, password, authorizedKeys string) error {
	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         10 * time.Second,
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(addr, "22"))
	if err != nil {
		return err
	}
	// Closing conn interrupts ssh.NewClientConn and sess.Run, which take no ctx.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return err
	}
	client := ssh.NewClient(c, chans, reqs)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(authorizedKeys)
	return sess.Run(
		"mkdir -p /root/.ssh && chmod 700 /root/.ssh && " +
			"touch /root/.ssh/authorized_keys && chmod 600 /root/.ssh/authorized_keys && " +
			"cat - >> /root/.ssh/authorized_keys && " +
			"sort -u -o /root/.ssh/authorized_keys /root/.ssh/authorized_keys",
	)
}
