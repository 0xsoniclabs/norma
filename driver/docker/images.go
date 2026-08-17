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
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
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

// imageBuildPlan says where the client sources of one image reference come
// from.
type imageBuildPlan struct {
	// source is where the client sources are taken from: a path on disk for a
	// local build, the repository URL for a remote one, and nowhere for an
	// image that is pulled rather than built. It is passed to docker build as
	// the value of the "client-src" build context the repository Dockerfile
	// expects.
	source string
	// gitRef is the branch, tag or commit to build in that repository. It is
	// set for a remote build only.
	gitRef string
}

// builds reports whether the image is built from sources rather than pulled.
func (p imageBuildPlan) builds() bool {
	return p.source != ""
}

// clientSrc is the "client-src" build context docker is given. A remote build
// names the commit the git ref points to right now rather than the ref itself,
// so that what the image contains is decided and recorded here, and not by
// whatever the ref names by the time docker looks.
func (p imageBuildPlan) clientSrc(ctx context.Context) (string, error) {
	if p.gitRef == "" {
		return p.source, nil
	}
	commit, err := resolveCommit(ctx, p.source, p.gitRef)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s#%s", p.source, commit), nil
}

// gitHeadRef is the git ref naming the default branch of a repository.
const gitHeadRef = "HEAD"

// onceCache resolves a value for a key once and hands that result to every
// caller asking later. Concurrent callers wait for the running resolution
// instead of starting a second one. A failed resolution is forgotten, so a
// later caller can try again.
type onceCache struct {
	mutex   sync.Mutex
	entries map[string]*onceEntry
}

// onceEntry is the outcome of one resolution: the value it produced, or the
// error that ended the attempt.
type onceEntry struct {
	done  chan struct{}
	value string
	err   error
}

func (c *onceCache) get(
	ctx context.Context,
	key string,
	resolve func() (string, error),
) (string, error) {
	c.mutex.Lock()
	if entry, found := c.entries[key]; found {
		c.mutex.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-entry.done:
			return entry.value, entry.err
		}
	}
	entry := &onceEntry{done: make(chan struct{})}
	if c.entries == nil {
		c.entries = map[string]*onceEntry{}
	}
	c.entries[key] = entry
	c.mutex.Unlock()

	entry.value, entry.err = resolve()
	close(entry.done)

	if entry.err != nil {
		c.forget(key)
	}
	return entry.value, entry.err
}

func (c *onceCache) forget(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.entries, key)
}

// resolvedImages holds the image every reference resolved to in this run.
var resolvedImages onceCache

// EnsureImage makes the image of the given reference locally available and
// returns the reference this run keeps that image under.
//
// A reference is resolved once per run, and every caller after the first one
// gets the image of that first resolution. Client references are mutable:
// "sonic:local" builds from a working copy and "sonic:main" from a branch, so
// resolving one twice can produce two different clients. The genesis of a
// network is written for the versions its images reported (see ClientVersions),
// and a node started from a later build of the same reference may be a client
// that genesis does not suit, which forks it off the rest of the network.
//
// The image that first resolution produced is therefore kept under a reference
// of this run, and that is what callers start their containers from. Neither
// the reference they asked for nor the bare image survives a run on its own:
// the host is shared with every other build on it, one of which moves the
// reference to its own client, and an image no reference names any more is the
// first thing a docker prune reclaims - while a run keeps starting nodes long
// after it resolved its images. The pinned reference is norma's own, and every
// call re-establishes it, so a host that dropped it does not take the run along.
//
// For Sonic image refs, the image is built from the project's Dockerfile:
//   - sonic, sonic:latest: from the default branch of sonicRepositoryURL
//   - sonic:local: from the currently configured sonic local path (see
//     SetSonicLocalPath / SonicLocalPath, default DefaultSonicLocalPath)
//   - sonic:<branch or tag>: from that ref of sonicRepositoryURL
//   - sonic:<commit hash>: from that commit of sonicRepositoryURL
//
// Other images are pulled.
func EnsureImage(ctx context.Context, imageRef, buildRoot string) (string, error) {
	imageID, err := resolvedImages.get(ctx, imageRef, func() (string, error) {
		return resolveImage(ctx, imageRef, buildRoot)
	})
	if err != nil {
		return "", err
	}
	return pinImage(ctx, imageRef, imageID, buildRoot)
}

// pinnedRepository is the docker repository this run keeps the images it
// resolved under. A reference in it is norma's own, so nothing but norma moves
// it, and it keeps the image it names from being one no reference names.
const pinnedRepository = "norma-pinned"

// pinnedImageRef is the reference the image of the given ID is kept under. It
// names the image itself, so a run that resolved the same image pins it under
// the same reference, and one that resolved another image under another.
func pinnedImageRef(imageID string) string {
	return pinnedRepository + ":" + strings.TrimPrefix(imageID, "sha256:")
}

// pinMutex serializes pinning, so that nodes starting at the same time do not
// each resolve the same lost image again.
var pinMutex sync.Mutex

// pinImage keeps the given image under its pinned reference, and returns that
// reference.
//
// Pinning it is also how the host is asked whether it still has it: tagging an
// image is idempotent, and the one way it fails is the image being gone - which
// happens while a run is still starting nodes on it, either because a prune
// reclaimed it or because a concurrent build of the reference it was resolved
// from left it under no reference at all. It is then resolved again, and the run
// only continues if that produced the same image: another one holds another
// client than the genesis of the network was written for.
func pinImage(ctx context.Context, imageRef, imageID, buildRoot string) (string, error) {
	ref := pinnedImageRef(imageID)

	pinMutex.Lock()
	defer pinMutex.Unlock()

	cli, err := NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create docker client: %w", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	err = cli.cli.ImageTag(ctx, imageID, ref)
	if client.IsErrNotFound(err) {
		slog.Warn("the image of a reference is gone, resolving it again",
			"ref", imageRef, "id", imageID)
		again, resolveErr := resolveImage(ctx, imageRef, buildRoot)
		if resolveErr != nil {
			return "", fmt.Errorf("image %s (%s) is gone, and resolving it again failed: %w",
				imageRef, imageID, resolveErr)
		}
		if again != imageID {
			return "", fmt.Errorf("image %s is gone, and its sources now produce %s "+
				"rather than the %s this run was set up for", imageRef, again, imageID)
		}
		err = cli.cli.ImageTag(ctx, imageID, ref)
	}
	if err != nil {
		return "", fmt.Errorf("failed to pin image %s as %q: %w", imageID, ref, err)
	}
	return ref, nil
}

// EnsureImages makes sure the given image refs are locally available, see
// EnsureImage. It serves callers that only want the images built or pulled,
// like the build command; a caller that starts a container from one needs the
// reference EnsureImage returns.
func EnsureImages(ctx context.Context, imageRefs []string, buildRoot string) error {
	refs := NormalizeImageRefs(imageRefs)
	if len(refs) == 0 {
		return nil
	}
	slog.Info("checking images", "refs", refs)
	for _, ref := range refs {
		if _, err := EnsureImage(ctx, ref, buildRoot); err != nil {
			return err
		}
	}
	return nil
}

// resolveImage builds or pulls the image of the given reference and returns the
// ID of the image this produced.
func resolveImage(ctx context.Context, imageRef, buildRoot string) (string, error) {
	start := time.Now()
	plan := planImage(imageRef)

	var id string
	if plan.builds() {
		root, err := ResolveBuildRoot(buildRoot)
		if err != nil {
			return "", err
		}
		clientSrc, err := plan.clientSrc(ctx)
		if err != nil {
			return "", err
		}
		slog.Info("building image", "ref", imageRef, "buildRoot", root, "clientSrc", clientSrc)
		if id, err = buildImage(ctx, root, imageRef, clientSrc); err != nil {
			return "", err
		}
	} else {
		cli, err := NewClient()
		if err != nil {
			return "", fmt.Errorf("failed to create docker client: %w", err)
		}
		defer func() {
			_ = cli.Close()
		}()
		slog.Info("pulling image", "ref", imageRef)
		if err := pullImage(ctx, cli, imageRef); err != nil {
			return "", err
		}
		if id, err = imageID(ctx, cli, imageRef); err != nil {
			return "", err
		}
	}

	slog.Info("image ready", "ref", imageRef, "id", id, "took", time.Since(start))
	return id, nil
}

// commitHash matches a git commit hash: the object name git ls-remote lists a
// ref with, and the one git ref that no repository has to be asked about.
var commitHash = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// peeledSuffix marks the line on which git ls-remote reports the commit an
// annotated tag points to, next to the line reporting the tag object itself.
const peeledSuffix = "^{}"

// resolveCommit asks the remote repository which commit the given git ref
// points to at this moment. A ref the repository does not know is taken to be a
// commit hash, which is a ref only the repository itself can resolve.
func resolveCommit(ctx context.Context, repositoryURL, gitRef string) (string, error) {
	// The candidates are the refs git itself would try, in its order of
	// precedence: a bare name is ambiguous, and a repository may well hold both
	// a tag and a branch of that name.
	candidates := []string{gitHeadRef}
	if gitRef != gitHeadRef {
		candidates = []string{"refs/tags/" + gitRef, "refs/heads/" + gitRef}
	}

	// Only the candidates are asked for: a repository the size of Sonic's has
	// thousands of refs, and one is wanted. A pattern is matched against a whole
	// ref name, so the peeled line of a tag is only reported if it is asked for
	// under that name too.
	args := []string{"ls-remote", repositoryURL}
	for _, candidate := range candidates {
		args = append(args, candidate, candidate+peeledSuffix)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to ask %s for the commit of %q: %w\n%s",
			repositoryURL, gitRef, err, strings.TrimSpace(string(output)))
	}
	commits := parseRemoteRefs(output)

	for _, candidate := range candidates {
		if commit, found := commits[candidate]; found {
			return commit, nil
		}
	}
	if commitHash.MatchString(gitRef) {
		return gitRef, nil
	}
	return "", fmt.Errorf("repository %s has no branch or tag %q",
		repositoryURL, gitRef)
}

// parseRemoteRefs reads the refs and the commits they point to from the output
// of git ls-remote. An annotated tag is listed twice: as the tag object and,
// marked with "^{}", as the commit it points to. The latter is what checking
// out the tag yields, so it takes precedence.
func parseRemoteRefs(output []byte) map[string]string {
	commits := map[string]string{}
	peeled := map[string]bool{}
	for line := range strings.Lines(string(output)) {
		fields := strings.Fields(line)
		if len(fields) != 2 || !commitHash.MatchString(fields[0]) {
			continue
		}
		commit, ref := fields[0], fields[1]
		if tag, found := strings.CutSuffix(ref, peeledSuffix); found {
			commits[tag] = commit
			peeled[tag] = true
			continue
		}
		if !peeled[ref] {
			commits[ref] = commit
		}
	}
	return commits
}

// imageID reads the ID of the locally available image of the given reference.
func imageID(ctx context.Context, cli *Client, imageRef string) (string, error) {
	info, _, err := cli.cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to inspect image %q: %w", imageRef, err)
	}
	return info.ID, nil
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
// matching the Dockerfile convention used by Norma. BuildKit is enabled
// explicitly via environment variable to keep behavior aligned with existing
// developer workflows.
//
// On failure, the function returns the docker command error along with captured
// combined stdout/stderr output to aid diagnostics.
//
// The ID is what the build reports for its own result, not what the tag names
// afterwards. The tag is shared with every other build on the host, and a run
// of another norma builds its own client under it, so between tagging and
// asking the tag it can already name an image this build never produced.
func buildImage(ctx context.Context, buildRoot, imageRef, clientSrc string) (string, error) {
	idFile, err := os.CreateTemp("", "norma-image-id-*")
	if err != nil {
		return "", fmt.Errorf("failed to create the id file for image %q: %w", imageRef, err)
	}
	idPath := idFile.Name()
	_ = idFile.Close()
	defer func() {
		_ = os.Remove(idPath)
	}()

	args := []string{"build", "--build-context", fmt.Sprintf("client-src=%s", clientSrc),
		"--iidfile", idPath, ".", "-t", imageRef}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = buildRoot
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to build image %q: %w\n%s", imageRef, err, strings.TrimSpace(string(output)))
	}

	id, err := os.ReadFile(idPath) //#nosec G304 -- the path is the temporary file created above
	if err != nil {
		return "", fmt.Errorf("failed to read the id of image %q: %w", imageRef, err)
	}
	if len(strings.TrimSpace(string(id))) == 0 {
		return "", fmt.Errorf("the build of image %q reported no image id", imageRef)
	}
	return strings.TrimSpace(string(id)), nil
}

// planImage resolves imageRef into an internal build plan.
//
// Current mapping rules:
//   - "sonic", "sonic:latest" => remote build from the default branch of
//     sonicRepositoryURL; "latest" is a Docker tag convention, not a git ref
//   - "sonic:local" => local build from the currently configured sonic local
//     path (see SetSonicLocalPath / SonicLocalPath, default
//     DefaultSonicLocalPath)
//   - "sonic:<branch, tag or commit hash>" => remote build from that ref of
//     sonicRepositoryURL
//   - everything else => no build strategy (pull)
//
// The returned plan is consumed by resolveImage.
func planImage(imageRef string) imageBuildPlan {
	if imageRef == "sonic" || imageRef == "sonic:latest" {
		return imageBuildPlan{source: sonicRepositoryURL, gitRef: gitHeadRef}
	}
	if imageRef == "sonic:local" {
		return imageBuildPlan{source: sonicLocalPath}
	}
	if tag, found := strings.CutPrefix(imageRef, "sonic:"); found && tag != "" {
		return imageBuildPlan{source: sonicRepositoryURL, gitRef: tag}
	}
	return imageBuildPlan{}
}

// WillBuildImage reports whether EnsureImage will build (not pull) the given
// image reference.
//
// This is true for Sonic image refs handled via local or remote source build
// contexts (e.g. sonic, sonic:local, sonic:<tag-or-commit>). All other refs are
// pulled instead.
func WillBuildImage(imageRef string) bool {
	return planImage(imageRef).builds()
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
