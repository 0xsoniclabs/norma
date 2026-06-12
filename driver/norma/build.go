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

const defaultScenarioPath = "scenarios"

var execCommandContext = exec.CommandContext

// Run with `go run ./driver/norma build <scenario-dir-or-file>`

var buildCommand = cli.Command{
	Action: build,
	Name:   "build",
	Usage:  "builds required Sonic and report renderer Docker images for scenarios",
	Flags: []cli.Flag{
		&dryRun,
	},
}

var dryRun = cli.BoolFlag{
	Name:  "dry-run",
	Usage: "prints images and docker build commands without running them",
}

func build(ctx *cli.Context) error {
	targetPath := defaultScenarioPath
	if ctx.Args().Len() > 0 {
		targetPath = ctx.Args().First()
	}
	runDry := ctx.Bool(dryRun.Name)

	files, err := collectScenarioFiles(targetPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no scenario files found in %q", targetPath)
	}

	images, err := collectBuildableImages(files)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d scenario file(s) and %d image(s) to build:\n", len(files), len(images))
	for _, image := range images {
		fmt.Printf("  - %s\n", image)
	}
	if runDry {
		fmt.Printf("Dry run enabled: no images will be built.\n")
	}

	repoRoot, err := docker.ResolveBuildRoot(".")
	if err != nil {
		return err
	}

	if len(images) == 0 {
		fmt.Printf("No buildable client images were found in the selected scenarios.\n")
	} else if runDry {
		fmt.Printf("Would ensure images (build/pull via EnsureImages): %s\n", strings.Join(images, ", "))
	} else {
		if err := docker.EnsureImages(ctx.Context, images, repoRoot); err != nil {
			return err
		}
	}
	rCmdArgs := rRendererBuildCommandArgs()
	if runDry {
		fmt.Printf("Would run: docker %s\n", strings.Join(rCmdArgs, " "))
		fmt.Printf("Done.\n")
		return nil
	}

	fmt.Printf("Building norma-r-renderer ...\n")
	if err := runDockerCommand(ctx.Context, repoRoot, rCmdArgs...); err != nil {
		return err
	}

	fmt.Printf("Done.\n")
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
	return files, nil
}

func collectBuildableImages(paths []string) ([]string, error) {
	images := map[string]struct{}{}
	for _, path := range paths {
		scenario, err := parser.ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scenario %q: %w", path, err)
		}

		if err := scenario.Check(); err != nil {
			return nil, err
		}

		for _, validator := range driver.NewValidators(scenario.Validators) {
			if docker.WillBuildImage(validator.ImageName) {
				images[validator.ImageName] = struct{}{}
			}
		}

		for _, node := range scenario.Nodes {
			image := driver.ResolveClientImageName(node.Client.ImageName)
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

func rRendererBuildCommandArgs() []string {
	return []string{"build", "analysis/report/", "-t", "norma-r-renderer"}
}

func runDockerCommand(ctx context.Context, dir string, args ...string) error {
	cmd := execCommandContext(ctx, "docker", args...)
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
