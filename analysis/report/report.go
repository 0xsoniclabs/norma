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

package report

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Report is a template for a report to be produced from monitoring data
// collected by Norma. To render the report, use the Report`s render function.
type Report struct {
	name     string
	template []byte
}

//go:embed single_eval_report.Rmd
var singleEvalReportTemplate []byte

//go:embed multi_eval_report.Rmd
var multiEvalReportTemplate []byte

var (
	// SingleEvalReport is a report template covering metrics collected in a single
	// scenario evaluation.
	SingleEvalReport = Report{
		name:     "single_eval_report",
		template: singleEvalReportTemplate,
	}

	// MultiEvalReport is a report template comparing the results of multiple
	// scenario evaluations. The input CSV should be the concatenation of the
	// individual measurement CSV files.
	MultiEvalReport = Report{
		name:     "multi_eval_report",
		template: multiEvalReportTemplate,
	}
)

//go:embed render.R
var renderScript []byte

//go:embed Dockerfile
var rendererDockerfile []byte

const rendererImage = "norma-r-renderer"

// ensureRenderer builds the renderer image unless it exists. An existing image
// is not rebuilt when the Dockerfile changes; `norma build` does that.
func ensureRenderer() error {
	if exec.Command("docker", "image", "inspect", rendererImage).Run() == nil {
		return nil
	}
	slog.Info("building the report renderer image; this happens only once", "image", rendererImage)
	cmd := exec.Command("docker", "build", "-t", rendererImage, "-")
	cmd.Stdin = bytes.NewReader(rendererDockerfile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build %s: %w\n%s", rendererImage, err, out)
	}
	return nil
}

// Render renders this report using the given input data file (in CSV format)
// and places its results into the defined output directory.
func (r *Report) Render(datafile, outputDir, scenario, description, scenarioFilePath string) (outputFile string, err error) {
	if err := ensureRenderer(); err != nil {
		return "", err
	}
	script, err := createTempFile(renderScript, ".R")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, os.Remove(script)) }()

	template, err := createTempFile(r.template, ".Rmd")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, os.Remove(template)) }()

	outputFile = r.name + ".html"
	if scenarioFilePath != "" {
		base := strings.TrimSuffix(
			filepath.Base(scenarioFilePath),
			filepath.Ext(scenarioFilePath),
		)
		if base != "" {
			outputFile = base + ".html"
		}
	}

	cmd := exec.Command("docker", "run", "--rm",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-w", "/output",
		"-v", script+":/render.R:ro",
		"-v", template+":/template.Rmd:ro",
		"-v", datafile+":/input.csv:ro",
		"-v", outputDir+":/output",
		rendererImage,
		"Rscript", "/render.R", "/template.Rmd", "/input.csv", "/output", outputFile,
		scenario,
		description,
		scenarioFilePath,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v\n%v", out.String(), err)
	}

	return outputFile, nil
}

func createTempFile(content []byte, suffix string) (filename string, err error) {
	file, err := os.CreateTemp("", "tmp_*"+suffix)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := file.Write(content); err != nil {
		return "", err
	}
	return file.Name(), nil
}
