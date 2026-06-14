package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/guyStrauss/pando/internal/client"
	"github.com/guyStrauss/pando/internal/discovery"
)

func ensureClient(g *globalFlags) (*client.Client, error) {
	if g.socket != "" {
		return client.New(g.socket), nil
	}
	info, found, running := discovery.Resolve(ctx())
	if found && running {
		return client.New(info.Socket), nil
	}
	gitDir := discovery.GitCommonDir(ctx())
	if gitDir == "" {
		return nil, fmt.Errorf("not inside a git repository; run pando from a repo or pass --socket")
	}
	socket, err := spawnDaemon(g, gitDir)
	if err != nil {
		return nil, err
	}
	return client.New(socket), nil
}

func spawnDaemon(g *globalFlags, gitDir string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := discovery.EnsureDir(); err != nil {
		return "", err
	}
	logFile, err := os.OpenFile(discovery.LogPath(gitDir), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(self, "daemon", "--config", g.config)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	_ = cmd.Process.Release()

	socket := discovery.SocketPath(gitDir)
	if err := waitForSocket(socket, 10*time.Second); err != nil {
		return "", fmt.Errorf("daemon did not come up (see %s): %w", discovery.LogPath(gitDir), err)
	}
	return socket, nil
}

func waitForSocket(socket string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	cl := client.New(socket)
	for time.Now().Before(deadline) {
		c, cancel := context.WithTimeout(ctx(), 200*time.Millisecond)
		err := cl.Health(c)
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}
