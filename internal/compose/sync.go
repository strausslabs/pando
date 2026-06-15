package compose

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"

	"github.com/strausslabs/pando/internal/resource"
	"github.com/strausslabs/pando/internal/scheduler"
)

func (b *Backend) Sync(ctx context.Context, r *resource.Resource, env scheduler.Env, localPath, containerPath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("sync source %s: %w", localPath, err)
	}
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)
	base := filepath.Base(containerPath)
	if err := tarPath(tw, localPath, base, info); err != nil {
		_ = tw.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	dst := filepath.Dir(containerPath)
	name := containerName(env.Project, r.Name)
	if _, err := b.Exec(ctx, env.Worktree, r.Name, []string{"mkdir", "-p", dst}, env); err != nil {
		return fmt.Errorf("sync: create %s in container: %w", dst, err)
	}
	return b.cli.CopyToContainer(ctx, name, dst, buf, container.CopyToContainerOptions{})
}

func tarPath(tw *tar.Writer, localPath, name string, info os.FileInfo) error {
	if info.IsDir() {
		entries, err := os.ReadDir(localPath)
		if err != nil {
			return err
		}
		for _, e := range entries {
			ei, err := e.Info()
			if err != nil {
				return err
			}
			if err := tarPath(tw, filepath.Join(localPath, e.Name()), filepath.Join(name, e.Name()), ei); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(tw, f)
	return err
}
