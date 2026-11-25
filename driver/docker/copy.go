// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package docker

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
)

// CopyTo copies the file or directory at hostPath into the container, where
// it is named containerPath. Missing parent directories are created. Files
// move through the Docker API, so the daemon need not share the driver's
// filesystem.
func (c *Container) CopyTo(ctx context.Context, hostPath, containerPath string) error {
	archive := tarPath(hostPath, containerPath)
	defer func() { _ = archive.Close() }()
	err := c.client.cli.CopyToContainer(ctx, c.id, "/", archive, container.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("failed to copy %s into the container: %w", hostPath, err)
	}
	return nil
}

// CopyFrom copies the file at containerPath out of the container to hostPath.
func (c *Container) CopyFrom(ctx context.Context, containerPath, hostPath string) error {
	archive, stat, err := c.client.cli.CopyFromContainer(ctx, c.id, containerPath)
	if err != nil {
		return fmt.Errorf("failed to copy %s out of the container: %w", containerPath, err)
	}
	defer func() { _ = archive.Close() }()
	if stat.Mode.IsDir() {
		return fmt.Errorf("%s is not a file in the container", containerPath)
	}

	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s is not a file in the container", containerPath)
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg {
			return writeFile(hostPath, reader, header.FileInfo().Mode())
		}
	}
}

func writeFile(hostPath string, content io.Reader, mode fs.FileMode) (err error) {
	f, err := os.OpenFile(hostPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	_, err = io.Copy(f, content)
	return err
}

// tarPath streams a tar archive of the file or directory tree at hostPath,
// rooted at name and including the parent directories of name. Streaming
// keeps multi-gigabyte files out of memory.
func tarPath(hostPath, name string) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		writer.CloseWithError(writeTar(tar.NewWriter(writer), hostPath, path.Clean("/" + name)[1:]))
	}()
	return reader
}

func writeTar(archive *tar.Writer, hostPath, name string) error {
	var parents []string
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		parents = append([]string{parent}, parents...)
	}
	for _, parent := range parents {
		if err := archive.WriteHeader(&tar.Header{
			Typeflag: tar.TypeDir, Name: parent + "/", Mode: 0o755,
		}); err != nil {
			return err
		}
	}
	// A symlinked root stands for what it points to, as a bind mount did.
	hostPath, err := filepath.EvalSymlinks(hostPath)
	if err != nil {
		return errors.Join(err, archive.Close())
	}
	err = filepath.WalkDir(hostPath, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(hostPath, current)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var link string
		if entry.Type()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(current); err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = path.Join(name, filepath.ToSlash(rel))
		if entry.IsDir() {
			header.Name += "/"
		}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		f, err := os.Open(current)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(archive, f)
		return err
	})
	return errors.Join(err, archive.Close())
}
