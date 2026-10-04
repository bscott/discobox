package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
	"github.com/spf13/cobra"
)

// newSSHProxyCommand is the `ProxyCommand` the emitted ssh_config names. It
// splices its own stdin and stdout onto `GET /ssh/connect`, so `ssh` reaches
// the server's sshd over the endpoint the API already answers on.
//
// It is hidden because nothing types it: ssh runs it, once per connection, with
// the arguments the config was written with.
func (a *App) newSSHProxyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-proxy",
		Short: "Carry one SSH connection to the server over the API endpoint",
		Long: `Carry one SSH connection to the server over the API endpoint.

This is the ProxyCommand ` + "`discobox admin ssh-config`" + ` writes. It reads SSH's wire
protocol on stdin, writes it on stdout, and carries it to the server's SSH
ingress over the same endpoint every other request uses — so ssh reaches a
discobox whether or not the server binds an SSH port of its own.`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runSSHProxy(cmd)
		},
	}
	return cmd
}

func (a *App) runSSHProxy(cmd *cobra.Command) error {
	dialer, err := a.sshConnectDialer()
	if err != nil {
		return err
	}
	remote, err := dialer.dial(cmd.Context())
	if err != nil {
		return fmt.Errorf("connect to the server's SSH ingress: %w", err)
	}
	defer remote.Close()
	// Nothing is written to stdout but the connection: it is the wire ssh is
	// reading. Errors go to stderr, which ssh already relays to its own.
	spliceSSHConnect(cmd.Context(), readWriter{r: cmd.InOrStdin(), w: cmd.OutOrStdout()}, remote)
	return nil
}

// readWriter pairs the two halves of a process's standard streams into the one
// stream the splice reads and writes. os.Stdin and os.Stdout are separate
// files, and the splice's local side is one connection's worth of bytes.
type readWriter struct {
	r io.Reader
	w io.Writer
}

func (rw readWriter) Read(p []byte) (int, error)  { return rw.r.Read(p) }
func (rw readWriter) Write(p []byte) (int, error) { return rw.w.Write(p) }

// sshConnectDialer opens one byte stream to the server's sshd over the
// transport the API already answers on: a `GET /ssh/connect` websocket, whose
// stream the server hands to the same sshd its TCP listener feeds.
//
// It is the only way this CLI reaches that sshd: the `ProxyCommand` an emitted
// ssh_config names, and the one `tools ssh` and `cp` pass on the command line
// (ssh_client.go), both run `admin ssh-proxy`, which dials through this.
type sshConnectDialer struct {
	url    string
	client *http.Client
}

// sshConnectDialer resolves the endpoint and the client the websocket is
// dialed with.
func (a *App) sshConnectDialer() (sshConnectDialer, error) {
	baseURL, httpClient, err := a.httpClient()
	if err != nil {
		return sshConnectDialer{}, err
	}
	socketURL, err := sshConnectWebSocketURL(baseURL)
	if err != nil {
		return sshConnectDialer{}, err
	}
	return sshConnectDialer{url: socketURL, client: httpClient}, nil
}

// dial returns the websocket as a net.Conn. Closing it closes the websocket.
func (d sshConnectDialer) dial(ctx context.Context) (net.Conn, error) {
	wsConn, resp, err := websocket.Dial(ctx, d.url, &websocket.DialOptions{HTTPClient: d.client})
	if resp != nil && resp.Body != nil {
		// The handshake response body carries nothing once the connection is
		// upgraded, but it is still a body: leaving it open leaks the
		// underlying connection on every session.
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return websocket.NetConn(ctx, wsConn, websocket.MessageBinary), nil
}

// spliceSSHConnect pumps bytes both ways until either side finishes or the
// context is canceled. Neither stream is closed here: the caller owns both.
func spliceSSHConnect(ctx context.Context, local io.ReadWriter, remote io.ReadWriter) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, local); done <- struct{}{} }()
	go func() { _, _ = io.Copy(local, remote); done <- struct{}{} }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// sshConnectWebSocketURL turns the API base URL into the websocket URL for the
// SSH connect route. A unix-socket endpoint keeps its scheme-less host: the
// HTTP client dials the socket regardless of what the URL says, and the host is
// only there to make it a valid URL.
func sshConnectWebSocketURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", fmt.Errorf("parse server URL %q: %w", baseURL, err)
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = "/ssh/connect"
	return parsed.String(), nil
}
