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
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/image"
)

// sonicRepositoryURL is the canonical remote source used when building
// non-local Sonic images on demand.
const sonicRepositoryURL = "https://github.com/0xsoniclabs/sonic.git"

// DefaultSonicLocalPath is the default path used as the build context when
// building the "sonic:local" image. It is interpreted relative to the Norma
// build root (see ResolveBuildRoot) unless overridden with SetSonicLocalPath
// to point at an arbitrary location on disk.
const DefaultSonicLocalPath = "sonic"

// sonicLocalPath is the currently configured path used as the docker build
// context for the "sonic:local" image. It defaults to DefaultSonicLocalPath
// and can be overridden with SetSonicLocalPath (for example from a CLI flag).
var sonicLocalPath = DefaultSonicLocalPath

// SetSonicLocalPath configures the path used as the docker build context for
// the "sonic:local" image. An empty path resets the configuration to the
// built-in default (DefaultSonicLocalPath).
//
// The path may be absolute or relative; when relative it is resolved against
// the Norma build root at build time.
func SetSonicLocalPath(path string) {
	if path == "" {
		sonicLocalPath = DefaultSonicLocalPath
		return
	}
	sonicLocalPath = path
}

// SonicLocalPath returns the currently configured path used as the docker
// build context for the "sonic:local" image.
func SonicLocalPath() string {
	return sonicLocalPath
}

// imageBuildKind describes how an image should be materialized when it is not
// already available locally.
type imageBuildKind int

const (
	// imageBuildNone means no local build strategy applies. In this case the
	// image is obtained by pullImage.
	imageBuildNone imageBuildKind = iota
	// imageBuildSonicRemote means the image should be built from the remote
	// Sonic repository URL (optionally pinned to a tag via #<tag>).
	imageBuildSonicRemote
	// imageBuildSonicLocal means the image should be built from local Sonic
	// sources (the repository's "sonic" directory).
	imageBuildSonicLocal
)

// imageBuildPlan captures the selected provisioning strategy for one image
// reference.
//
// clientSrc is passed to docker build as the value of the "client-src" build
// context expected by the repository Dockerfile. goVersion, when non-empty,
// is passed as the GO_VERSION build arg selecting the Go toolchain; when
// empty, the Dockerfile default applies.
type imageBuildPlan struct {
	kind      imageBuildKind
	clientSrc string
	goVersion string
}

// GoVersionDelimiter separates the source part of a sonic image tag from the
// Go toolchain version the sources are built with, as in
// "sonic:v2.2.0_go1.27.0". It cannot collide with a sonic git tag (vX.Y.Z)
// or a commit hash (hex), so the split is unambiguous.
const GoVersionDelimiter = "_go"

// SplitGoVersion splits a sonic image ref into its source ref and the Go
// toolchain version it pins, e.g. "sonic:local_go1.27.0" into "sonic:local"
// and "1.27.0". Refs that pin no toolchain, and refs of other images, are
// returned unchanged with an empty version.
func SplitGoVersion(imageRef string) (ref, goVersion string) {
	if !strings.HasPrefix(imageRef, "sonic:") {
		return imageRef, ""
	}
	idx := strings.LastIndex(imageRef, GoVersionDelimiter)
	if idx < 0 || idx+len(GoVersionDelimiter) == len(imageRef) {
		return imageRef, ""
	}
	return imageRef[:idx], imageRef[idx+len(GoVersionDelimiter):]
}

// EnsureImages makes sure the given image refs are locally available.
//
// The function provides the runtime image provisioning path used by
// scenario execution. It performs the following steps:
//
//  1. Normalize and deduplicate image references.
//  2. Resolve the Norma build root (see ResolveBuildRoot).
//  3. For each image:
//     - choose build or pull strategy via planImage;
//     - build (Sonic-specific refs) or pull (all other refs).
//
// For Sonic image refs, it lazily builds from the project's Dockerfile:
//   - sonic: from sonicRepositoryURL
//   - sonic:local: from the currently configured sonic local path (see
//     SetSonicLocalPath / SonicLocalPath, default DefaultSonicLocalPath)
//   - sonic:<tag>: from sonicRepositoryURL#<tag>
//   - sonic:<commit hash>: from sonicRepositoryURL#<commit hash>
//
// A "_go<version>" suffix on any of the sonic tags selects the Go toolchain
// used for the build (e.g. sonic:local_go1.27.0); see SplitGoVersion.
//
// Other images are pulled if missing.
//
// The operation is idempotent with respect to already present tags, and relies
// on Docker's own image/layer cache for repeated builds.
func EnsureImages(ctx context.Context, imageRefs []string, buildRoot string) error {
	if len(imageRefs) == 0 {
		return nil
	}
	if buildRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		buildRoot = cwd
	}

	buildRoot, err := ResolveBuildRoot(buildRoot)
	if err != nil {
		return err
	}
	slog.Info("resolved build root", "path", buildRoot)

	refs := NormalizeImageRefs(imageRefs)
	slog.Info("checking images", "refs", refs)
	cli, err := NewClient()
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	for _, ref := range refs {

		plan := planImage(ref)
		start := time.Now()
		switch plan.kind {
		case imageBuildSonicRemote, imageBuildSonicLocal:
			slog.Info("building image", "ref", ref,
				"clientSrc", plan.clientSrc, "goVersion", plan.goVersion)
			if err := buildImage(ctx, buildRoot, ref, plan.clientSrc, plan.goVersion); err != nil {
				return err
			}
		case imageBuildNone:
			slog.Info("pulling image", "ref", ref)
			if err := pullImage(ctx, cli, ref); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported image plan for %q", ref)
		}
		slog.Info("image ready", "ref", ref, "took", time.Since(start))
	}

	return nil
}

// NormalizeImageRefs removes empty refs, deduplicates, and returns image refs
// in lexical order.
func NormalizeImageRefs(in []string) []string {
	set := map[string]bool{}
	for _, imageRef := range in {
		if strings.TrimSpace(imageRef) == "" {
			continue
		}
		set[imageRef] = true
	}
	out := make([]string, 0, len(set))
	for imageRef := range set {
		out = append(out, imageRef)
	}
	sort.Strings(out)
	return out
}

// pullImage pulls imageRef from the configured registry and consumes the entire
// pull stream.
//
// Fully draining the stream ensures the pull operation completes and daemon
// resources are released before returning.
func pullImage(ctx context.Context, cli *Client, imageRef string) error {
	reader, err := cli.cli.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %q: %w", imageRef, err)
	}
	defer func() {
		_ = reader.Close()
	}()

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("failed to read pull output for image %q: %w", imageRef, err)
	}
	return nil
}

// buildImage builds imageRef using the repository Dockerfile located under
// buildRoot.
//
// The build passes "client-src=<clientSrc>" as an additional build context,
// matching the Dockerfile convention used by Norma. A non-empty goVersion is
// passed as the GO_VERSION build arg; when empty, the Dockerfile default
// toolchain applies. BuildKit is enabled explicitly via environment variable
// to keep behavior aligned with existing developer workflows.
//
// On failure, the function returns the docker command error along with captured
// combined stdout/stderr output to aid diagnostics.
func buildImage(ctx context.Context, buildRoot, imageRef, clientSrc, goVersion string) error {
	args := []string{"build", "--build-context", fmt.Sprintf("client-src=%s", clientSrc)}
	if goVersion != "" {
		args = append(args, "--build-arg", "GO_VERSION="+goVersion)
	}
	args = append(args, ".", "-t", imageRef)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = buildRoot
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build image %q: %w\n%s", imageRef, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// planImage resolves imageRef into an internal build plan.
//
// A "_go<version>" suffix on a sonic tag selects the Go toolchain and is
// split off before the source dispatch, so "sonic:local_go1.27.0" builds the
// same sources as "sonic:local" (see SplitGoVersion). The remaining ref maps
// as:
//   - "sonic"       => remote build from sonicRepositoryURL
//   - "sonic:local" => local build from the currently configured sonic local
//     path (see SetSonicLocalPath / SonicLocalPath, default
//     DefaultSonicLocalPath)
//   - "sonic:<tag>" => remote build from sonicRepositoryURL#<tag>
//   - "sonic:<commit hash>" => remote build from sonicRepositoryURL#<commit hash>
//   - everything else => no build strategy (pull)
//
// The returned plan is consumed by EnsureImages.
func planImage(imageRef string) imageBuildPlan {
	ref, goVersion := SplitGoVersion(imageRef)
	if ref == "sonic" {
		return imageBuildPlan{kind: imageBuildSonicRemote, clientSrc: sonicRepositoryURL, goVersion: goVersion}
	}
	if ref == "sonic:local" {
		return imageBuildPlan{kind: imageBuildSonicLocal, clientSrc: sonicLocalPath, goVersion: goVersion}
	}
	if ref == "sonic:latest" {
		// "latest" is a Docker tag convention, not a git ref; build from repo HEAD.
		return imageBuildPlan{kind: imageBuildSonicRemote, clientSrc: sonicRepositoryURL, goVersion: goVersion}
	}
	if strings.HasPrefix(ref, "sonic:") {
		tag := strings.TrimPrefix(ref, "sonic:")
		if tag != "" {
			return imageBuildPlan{
				kind:      imageBuildSonicRemote,
				clientSrc: fmt.Sprintf("%s#%s", sonicRepositoryURL, tag),
				goVersion: goVersion,
			}
		}
	}
	return imageBuildPlan{kind: imageBuildNone}
}

// WillBuildImage reports whether EnsureImages will build (not pull) the given
// image reference.
//
// This is true for Sonic image refs handled via local or remote source build
// contexts (e.g. sonic, sonic:local, sonic:<tag-or-commit>). For all other
// refs, EnsureImages will pull instead.
func WillBuildImage(imageRef string) bool {
	plan := planImage(imageRef)
	return plan.kind == imageBuildSonicRemote || plan.kind == imageBuildSonicLocal
}

// deduplicateAndSort removes blank entries, deduplicates refs, and returns
// them in lexical order.
//
// Sorting keeps execution deterministic and log output stable across runs.
func deduplicateAndSort(in []string) []string {
	return NormalizeImageRefs(in)
}

// resolveBuildRoot finds the Norma repository root to execute docker builds.
// See ResolveBuildRoot.
func resolveBuildRoot(startDir string) (string, error) {
	return ResolveBuildRoot(startDir)
}

// normaModulePath is the Go module path of this repository. It is the only
// marker that uniquely identifies the Norma root: generic markers such as
// Dockerfile or Makefile are also present in the vendored `sonic/`
// sub-tree, so matching on those would happily build the wrong project.
const normaModulePath = "module github.com/0xsoniclabs/norma"

// ResolveBuildRoot finds the Norma repository root to execute docker builds.
//
// Starting from startDir, it walks up parent directories until it finds a
// directory containing both:
//   - Dockerfile
//   - go.mod declaring the Norma module
//
// This guards against running docker build in unrelated directories while
// keeping call sites simple.
func ResolveBuildRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve build root: %w", err)
	}

	for {
		if fileExists(filepath.Join(dir, "Dockerfile")) &&
			isNormaModuleDir(dir) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New(
		"unable to locate norma build root with Dockerfile and go.mod for " +
			"module github.com/0xsoniclabs/norma")
}

// isNormaModuleDir reports whether dir holds the go.mod of the Norma module.
func isNormaModuleDir(dir string) bool {
	content, err := os.ReadFile(filepath.Join(dir, "go.mod")) //#nosec G304 -- path is derived from the search root
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(content)) {
		if strings.TrimSpace(line) == normaModulePath {
			return true
		}
	}
	return false
}

// fileExists reports whether path exists and is a regular file-like entry
// (i.e. not a directory).
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
