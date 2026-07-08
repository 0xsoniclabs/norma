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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/urfave/cli/v2"
)

// Run with `go run ./driver/norma build <scenario-dir-or-file>`

var buildCommand = cli.Command{
	Action:    build,
	Name:      "build",
	Usage:     "builds required Sonic and report renderer Docker images for scenarios",
	UsageText: "norma build [--dry-run] [<scenario-dir-or-file>=scenarios]",
	Flags: []cli.Flag{
		&dryRun,
	},
}

var dryRun = cli.BoolFlag{
	Name:  "dry-run",
	Usage: "prints images and docker build commands without running them",
}

func build(ctx *cli.Context) error {
	// by default, look for scenarios in the "scenarios" directory
	targetPath := "scenarios"
	if ctx.Args().Len() > 0 {
		targetPath = ctx.Args().First()
	}
	runDry := ctx.Bool(dryRun.Name)

	// Collect scenario files from the selected path.
	files, err := collectScenarioFiles(targetPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no scenario files found in %q", targetPath)
	}

	// Parse scenarios and gather client image refs that EnsureImages would build.
	images, err := collectBuildableImages(files)
	if err != nil {
		return err
	}

	slog.Info("build plan", "scenarioFiles", len(files), "images", len(images))
	for _, image := range images {
		slog.Info("build image", "image", image)
	}
	if runDry {
		slog.Info("dry run enabled; no images will be built")
	}

	// Resolve repository root for docker build contexts.
	repoRoot, err := docker.ResolveBuildRoot(".")
	if err != nil {
		return err
	}

	// Ensure client images (or print dry-run plan).
	if len(images) == 0 {
		slog.Info("no buildable client images found in selected scenarios")
	} else if runDry {
		slog.Info("would ensure images (build/pull via EnsureImages)", "images", strings.Join(images, ", "))
	} else {
		if err := docker.EnsureImages(ctx.Context, images, repoRoot); err != nil {
			return err
		}
	}

	for _, image := range []string{
		"hello-world:latest",
		"alpine:latest",
	} {
		pullArgs := []string{"image", "pull", image}
		if runDry {
			slog.Info("would run docker command", "command", "docker "+strings.Join(pullArgs, " "))
			continue
		}

		slog.Info("pulling support image", "image", image)
		if err := runDockerCommand(ctx.Context, repoRoot, pullArgs...); err != nil {
			return err
		}
	}

	// Build the report renderer image (or print dry-run command).
	rCmdArgs := []string{"build", "analysis/report/", "-t", "norma-r-renderer"}
	if runDry {
		slog.Info("would run docker command", "command", "docker "+strings.Join(rCmdArgs, " "))
		slog.Info("done")
		return nil
	}

	slog.Info("building norma-r-renderer")
	if err := runDockerCommand(ctx.Context, repoRoot, rCmdArgs...); err != nil {
		return err
	}

	slog.Info("done")
	return nil
}

func collectScenarioFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access %q: %w", path, err)
	}

	if !info.IsDir() {
		if !isYAML(path) {
			return nil, fmt.Errorf("%q is not a YAML scenario file", path)
		}
		return []string{path}, nil
	}

	var files []string
	err = filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if isYAML(d.Name()) {
			files = append(files, current)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk %q: %w", path, err)
	}

	sort.Strings(files)
	slog.Info("collected scenario files", "count", len(files))
	return files, nil
}

// collectBuildableImages parses scenarios and returns the unique image refs
// that are expected to be built by docker.EnsureImages.
//
// The helper applies scenario defaults (e.g. default client image for empty
// startNode image entries) so build behavior matches runtime node startup
// behavior.
func collectBuildableImages(paths []string) ([]string, error) {
	images := map[string]struct{}{}
	for _, path := range paths {
		scenario, err := parser.ParseSequentialFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scenario %q: %w", path, err)
		}

		if err := scenario.Check(); err != nil {
			return nil, err
		}

		for _, step := range scenario.Steps {
			if step.Function != parser.FuncStartNode {
				continue
			}
			image := driver.ResolveClientImageName(step.ImageName)
			if docker.WillBuildImage(image) {
				images[image] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(images))
	for image := range images {
		result = append(result, image)
	}
	return docker.NormalizeImageRefs(result), nil
}

// runDockerCommand executes a docker CLI command in the given working
// directory with BuildKit enabled and inherited stdio.
func runDockerCommand(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s failed: %w", strings.Join(args, " "), err)
	}
	return nil
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yml" || ext == ".yaml"
}
