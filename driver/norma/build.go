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
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/urfave/cli/v2"
)

const sonicClientSource = "https://github.com/0xsoniclabs/sonic.git"
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

	images, err := collectSonicImages(files)
	if err != nil {
		return err
	}
	if len(images) == 0 {
		images = []string{driver.DefaultClientDockerImageName}
	}

	fmt.Printf("Found %d scenario file(s) and %d Sonic image(s) to build:\n", len(files), len(images))
	for _, image := range images {
		fmt.Printf("  - %s\n", image)
	}
	if runDry {
		fmt.Printf("Dry run enabled: no images will be built.\n")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	for _, image := range images {
		cmdArgs, err := sonicBuildCommandArgs(image)
		if err != nil {
			return err
		}

		if runDry {
			fmt.Printf("Would run: docker %s\n", strings.Join(cmdArgs, " "))
			continue
		}

		fmt.Printf("Building %s ...\n", image)
		if err := runDockerCommand(ctx.Context, repoRoot, cmdArgs...); err != nil {
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

func collectSonicImages(paths []string) ([]string, error) {
	images := map[string]struct{}{}
	for _, path := range paths {
		scenario, err := parser.ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scenario %q: %w", path, err)
		}

		for _, validator := range scenario.Validators {
			image := validator.ImageName
			if image == "" {
				image = driver.DefaultClientDockerImageName
			}
			if isSonicImage(image) {
				images[image] = struct{}{}
			}
		}

		for _, node := range scenario.Nodes {
			image := node.Client.ImageName
			if image == "" {
				image = driver.DefaultClientDockerImageName
			}
			if isSonicImage(image) {
				images[image] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(images))
	for image := range images {
		result = append(result, image)
	}
	sort.Strings(result)
	return result, nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := cwd
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "analysis", "report", "Dockerfile")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("failed to locate repository root from %q", cwd)
		}
		dir = parent
	}
}

func sonicBuildCommandArgs(image string) ([]string, error) {
	buildContext, err := sonicBuildContext(image)
	if err != nil {
		return nil, err
	}

	return []string{
		"build",
		"--build-context", fmt.Sprintf("client-src=%s", buildContext),
		".",
		"-t", image,
	}, nil
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

func sonicBuildContext(image string) (string, error) {
	switch {
	case image == driver.DefaultClientDockerImageName:
		return sonicClientSource, nil
	case strings.HasPrefix(image, driver.DefaultClientDockerImageName+":"):
		tag := strings.TrimPrefix(image, driver.DefaultClientDockerImageName+":")
		if tag == "" {
			return "", fmt.Errorf("invalid image tag %q", image)
		}
		if tag == "local" {
			return "sonic", nil
		}
		return fmt.Sprintf("%s#%s", sonicClientSource, tag), nil
	default:
		return "", fmt.Errorf("unsupported Sonic image %q", image)
	}
}

func isSonicImage(image string) bool {
	return image == driver.DefaultClientDockerImageName || strings.HasPrefix(image, driver.DefaultClientDockerImageName+":")
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yml" || ext == ".yaml"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
