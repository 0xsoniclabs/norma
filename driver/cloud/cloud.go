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

// Package cloud runs scenarios on a short-lived GCP VM. The VM, its network
// and its disks exist only for one run: they are deleted however the run
// ends, and GCP deletes the VM by itself once the run's time is up.
package cloud

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

//go:embed main.tf
var terraformConfig []byte

// Variables rather than constants so that tests can shorten or redirect them.
var (
	iamURL          = "https://cloudresourcemanager.googleapis.com/v1/projects/%s:testIamPermissions"
	readyTimeout    = 10 * time.Minute
	pollInterval    = 10 * time.Second
	teardownTimeout = 20 * time.Minute
	// gcloudCallTimeout bounds each call of the leftover check.
	gcloudCallTimeout = 2 * time.Minute
)

var requiredPermissions = []string{
	"iap.tunnelInstances.accessViaIAP",
	"compute.instances.create",
	"compute.networks.create",
	"compute.firewalls.create",
	"compute.routers.create",
}

const (
	remoteRoot = "/opt/norma"
	remoteOut  = remoteRoot + "/out"
)

// Config describes one cloud run.
type Config struct {
	Project, Zone string
	Plan          Plan
	// MaxCost refuses runs whose worst-case cost is higher, in USD.
	MaxCost float64
	// Scenario is the scenario file or directory to run.
	Scenario string
	// OutputDir receives the norma_data_* directories of the run, as with
	// `norma run -o`; empty means the system's temporary directory.
	OutputDir string
	// RunFlags are passed on to `norma run` on the VM.
	RunFlags []string
	// TTY connects the remote run to a terminal, as `--tui` needs.
	TTY bool
	// Root is the Norma repository copied to the VM. Each run keeps its
	// Terraform state and logs in Root/cloud-runs/<run-id>.
	Root           string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Run runs the scenario on a VM created for it and deletes the VM and its
// network afterwards, also when the run fails or is interrupted. It returns
// the norma_data_* directories it collected, even when the run failed.
func Run(ctx context.Context, cfg Config) (collected []string, err error) {
	for _, tool := range []string{"terraform", "gcloud", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			return nil, fmt.Errorf("cloud runs need %s: %w", tool, err)
		}
	}
	if cfg.Plan.WorstCaseCost > cfg.MaxCost {
		return nil, fmt.Errorf("the worst-case cost of $%.2f exceeds the maximum of $%.2f",
			cfg.Plan.WorstCaseCost, cfg.MaxCost)
	}
	if err := checkPermissions(ctx, cfg.Project); err != nil {
		return nil, err
	}

	r := &run{cfg: cfg, id: newRunID()}
	r.name = "norma-run-" + r.id
	r.dir = filepath.Join(cfg.Root, "cloud-runs", r.id)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer catchSignals(cancel)()

	defer func() {
		teardownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), teardownTimeout)
		defer cancel()
		dirs, teardownErr := r.teardown(teardownCtx)
		collected = dirs
		err = errors.Join(err, teardownErr)
	}()
	return nil, r.execute(ctx)
}

type run struct {
	cfg       Config
	id, name  string
	dir       string
	created   bool // the VM and its network may exist
	scenarios bool // the scenario started on the VM
}

func (r *run) execute(ctx context.Context) error {
	tfDir := filepath.Join(r.dir, "tf")
	if err := os.MkdirAll(tfDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tfDir, "main.tf"), terraformConfig, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tfDir, "terraform.tfvars"), r.tfvars(), 0o644); err != nil {
		return err
	}

	slog.Info("creating the VM and its network", "name", r.name, "log", r.path("apply.log"))
	r.created = true
	if err := r.terraform(ctx, "apply.log", "init", "-input=false"); err != nil {
		return err
	}
	if err := r.terraform(ctx, "apply.log", "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return err
	}

	slog.Info("waiting for the VM to install Docker")
	if err := r.waitReady(ctx); err != nil {
		return err
	}

	slog.Info("copying the sources to the VM")
	if err := r.upload(ctx, r.cfg.Root, ".", remoteRoot,
		"--exclude=./.git", "--exclude=./sonic/.git", "--exclude=./sonic/build",
		"--exclude=./build", "--exclude=./cloud-runs"); err != nil {
		return fmt.Errorf("failed to copy the sources: %w", err)
	}
	scenario, err := filepath.Abs(r.cfg.Scenario)
	if err != nil {
		return err
	}
	if err := r.upload(ctx, filepath.Dir(scenario), filepath.Base(scenario), remoteRoot+"/scenario"); err != nil {
		return fmt.Errorf("failed to copy the scenario: %w", err)
	}

	goImage, err := r.goImage()
	if err != nil {
		return err
	}
	slog.Info("building norma on the VM", "log", r.path("build.log"))
	build := fmt.Sprintf("sudo docker run --rm -v %s:/src -w /src %s go build -o build/norma ./driver/norma",
		remoteRoot, goImage)
	if err := logged(r.ssh(ctx, build), r.path("build.log")); err != nil {
		return fmt.Errorf("failed to build norma on the VM: %w", err)
	}

	args := append([]string{"sudo", "./build/norma", "run", "--skip-report-rendering",
		"-o", remoteOut, "--timeout", r.cfg.Plan.Timeout.String()}, r.cfg.RunFlags...)
	args = append(args, remoteRoot+"/scenario/"+filepath.Base(scenario))
	var sshFlags []string
	if r.cfg.TTY {
		sshFlags = []string{"-t"}
	}
	cmd := r.ssh(ctx, "cd "+remoteRoot+" && "+shellJoin(args), sshFlags...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r.cfg.Stdin, r.cfg.Stdout, r.cfg.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 30 * time.Second
	r.scenarios = true
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the scenario failed on the VM: %w", err)
	}
	return nil
}

// teardown collects the results and deletes everything the run created.
func (r *run) teardown(ctx context.Context) ([]string, error) {
	var errs []error
	var collected []string
	if r.scenarios {
		slog.Info("collecting the results")
		dirs, err := r.collect(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to collect the results: %w", err))
		}
		collected = dirs
		usage, err := r.ssh(ctx, "df -h / /mnt/ssd; sudo du -sh /var/lib/docker /var/lib/containerd "+
			remoteRoot+"; sudo docker system df").Output()
		if err == nil {
			err = os.WriteFile(r.path("disk.txt"), usage, 0o644)
		}
		if err != nil {
			slog.Warn("failed to record the VM's disk usage", "error", err)
		}
	}
	if !r.created {
		return collected, errors.Join(errs...)
	}

	slog.Info("deleting the VM and its network", "log", r.path("destroy.log"))
	for attempt := range 3 {
		err := r.terraform(ctx, "destroy.log", "destroy", "-auto-approve", "-input=false", "-no-color")
		if err == nil {
			break
		}
		slog.Warn("terraform destroy failed", "attempt", attempt+1, "error", err)
	}
	left, err := r.leftovers(ctx)
	switch {
	case err != nil:
		errs = append(errs, fmt.Errorf("could not check what is left of run %s; check project %s by hand: %w",
			r.id, r.cfg.Project, err))
	case len(left) > 0:
		errs = append(errs, fmt.Errorf("LEFTOVER resources of run %s in project %s: %s; delete them with `terraform -chdir=%s destroy`",
			r.id, r.cfg.Project, strings.Join(left, ", "), filepath.Join(r.dir, "tf")))
	default:
		slog.Info("all resources of the run are deleted", "run", r.id)
	}
	return collected, errors.Join(errs...)
}

// collect moves the norma_data_* directories of the VM into the output
// directory. The _latest links are left out; they point into the VM.
func (r *run) collect(ctx context.Context) ([]string, error) {
	dest := r.cfg.OutputDir
	if dest == "" {
		dest = os.TempDir()
	}
	staging, err := os.MkdirTemp(dest, ".norma-cloud-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	remote := r.ssh(ctx, fmt.Sprintf("cd %s && sudo tar --exclude='norma_data_*_latest' -czf - .", remoteOut))
	local := exec.CommandContext(ctx, "tar", "-C", staging, "-xzf", "-")
	if err := pipe(remote, local); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "norma_data_") {
			continue
		}
		dir := filepath.Join(dest, entry.Name())
		if err := os.Rename(filepath.Join(staging, entry.Name()), dir); err != nil {
			return dirs, err
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// leftovers lists what the run created that still exists.
func (r *run) leftovers(ctx context.Context) ([]string, error) {
	kinds := []struct {
		args   []string
		filter string
	}{
		{[]string{"instances"}, "labels.norma-run=" + r.id},
		{[]string{"disks"}, "labels.norma-run=" + r.id},
		{[]string{"firewall-rules"}, "name=" + r.name},
		{[]string{"routers"}, "name=" + r.name},
		{[]string{"networks", "subnets"}, "name=" + r.name},
		{[]string{"networks"}, "name=" + r.name},
	}
	var left []string
	for _, kind := range kinds {
		args := append([]string{"compute"}, kind.args...)
		args = append(args, "list", "--project", r.cfg.Project, "--filter", kind.filter,
			"--format", "value(name)", "--verbosity=error")
		callCtx, cancel := context.WithTimeout(ctx, gcloudCallTimeout)
		out, err := exec.CommandContext(callCtx, "gcloud", args...).Output()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("gcloud %s: %w", strings.Join(args, " "), err)
		}
		for name := range strings.FieldsSeq(string(out)) {
			left = append(left, strings.Join(kind.args, " ")+" "+name)
		}
	}
	return left, nil
}

func (r *run) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(readyTimeout)
	for {
		attemptCtx, cancel := context.WithDeadline(ctx, deadline)
		out, err := r.ssh(attemptCtx, "test -f /ready").CombinedOutput()
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() == nil && time.Now().After(deadline) {
			return fmt.Errorf("the VM was not ready after %s: %s", readyTimeout, bytes.TrimSpace(out))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// upload copies path, relative to dir, into the remote directory.
func (r *run) upload(ctx context.Context, dir, path, remoteDir string, excludes ...string) error {
	args := append([]string{"-C", dir}, excludes...)
	local := exec.CommandContext(ctx, "tar", append(args, "-czf", "-", path)...)
	remote := r.ssh(ctx, fmt.Sprintf("sudo mkdir -p %s %s && sudo tar -C %s -xzf -", remoteDir, remoteOut, remoteDir))
	return pipe(local, remote)
}

func (r *run) ssh(ctx context.Context, command string, sshFlags ...string) *exec.Cmd {
	args := []string{"compute", "ssh", r.name, "--project", r.cfg.Project, "--zone", r.cfg.Zone,
		"--tunnel-through-iap", "--quiet", "--verbosity=error", "--command", command}
	if len(sshFlags) > 0 {
		args = append(append(args, "--"), sshFlags...)
	}
	return exec.CommandContext(ctx, "gcloud", args...)
}

func (r *run) terraform(ctx context.Context, logName string, args ...string) error {
	cache := os.Getenv("TF_PLUGIN_CACHE_DIR")
	if cache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cache = filepath.Join(home, ".terraform.d", "plugin-cache")
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "terraform", append([]string{"-chdir=" + filepath.Join(r.dir, "tf")}, args...)...)
	cmd.Env = append(os.Environ(), "TF_PLUGIN_CACHE_DIR="+cache)
	if err := logged(cmd, r.path(logName)); err != nil {
		return fmt.Errorf("terraform %s: %w", args[0], err)
	}
	return nil
}

func (r *run) tfvars() []byte {
	return fmt.Appendf(nil, "project      = %q\nzone         = %q\nrun_id       = %q\n"+
		"machine_type = %q\ndisk_gb      = %d\nttl_minutes  = %d\n",
		r.cfg.Project, r.cfg.Zone, r.id, r.cfg.Plan.Machine.Name, bootDiskGB,
		int(math.Ceil(r.cfg.Plan.TTL.Minutes())))
}

// goImage is the Go image the client images are built with, in the Debian
// release of the VM, so that the norma binary runs there.
func (r *run) goImage() (string, error) {
	dockerfile, err := os.ReadFile(filepath.Join(r.cfg.Root, "Dockerfile"))
	if err != nil {
		return "", err
	}
	image := regexp.MustCompile(`golang:[0-9.]+`).Find(dockerfile)
	if image == nil {
		return "", errors.New("no golang image in the Dockerfile")
	}
	return string(image) + "-trixie", nil
}

func (r *run) path(name string) string {
	return filepath.Join(r.dir, name)
}

// checkPermissions fails unless the gcloud account may do everything a run
// needs, before anything is created.
func checkPermissions(ctx context.Context, project string) error {
	token, err := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token").Output()
	if err != nil {
		return fmt.Errorf("failed to get a gcloud access token; run `gcloud auth login`: %w", err)
	}
	body, err := json.Marshal(map[string][]string{"permissions": requiredPermissions})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(iamURL, project), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check the permissions in project %s: %w", project, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to check the permissions in project %s: %s: %s", project, resp.Status, msg)
	}
	var granted struct{ Permissions []string }
	if err := json.NewDecoder(resp.Body).Decode(&granted); err != nil {
		return err
	}
	var missing []string
	for _, permission := range requiredPermissions {
		if !slices.Contains(granted.Permissions, permission) {
			missing = append(missing, permission)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing permissions in project %s: %s "+
			"(iap.tunnelInstances.accessViaIAP comes with roles/iap.tunnelResourceAccessor)",
			project, strings.Join(missing, ", "))
	}
	return nil
}

// catchSignals cancels the run on the first SIGINT or SIGTERM and keeps later
// ones from killing the process while the VM and its network are deleted.
func catchSignals(cancel context.CancelFunc) (stop func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for first := true; ; first = false {
			select {
			case <-signals:
				if first {
					slog.Warn("interrupted: stopping the run; the VM and its network are still deleted")
					cancel()
				} else {
					slog.Warn("still deleting the VM and its network; this cannot be interrupted")
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

// logged runs cmd with its output appended to the file at path, and returns
// the end of that output when it fails.
func logged(cmd *exec.Cmd, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var tail bytes.Buffer
	cmd.Stdout = io.MultiWriter(f, &tail)
	cmd.Stderr = cmd.Stdout
	if err := cmd.Run(); err != nil {
		lines := strings.Split(strings.TrimSpace(tail.String()), "\n")
		return fmt.Errorf("%w; see %s:\n%s", err, path, strings.Join(lines[max(0, len(lines)-20):], "\n"))
	}
	return nil
}

// pipe runs from and to with the output of from as the input of to.
func pipe(from, to *exec.Cmd) error {
	out, err := from.StdoutPipe()
	if err != nil {
		return err
	}
	to.Stdin = out
	var fromErr, toErr bytes.Buffer
	from.Stderr, to.Stderr = &fromErr, &toErr
	if err := from.Start(); err != nil {
		return err
	}
	if err := to.Run(); err != nil {
		_ = from.Wait()
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(toErr.Bytes()))
	}
	if err := from.Wait(); err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(fromErr.Bytes()))
	}
	return nil
}

// shellJoin quotes args for a POSIX shell.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	safe := regexp.MustCompile(`^[A-Za-z0-9_./=:@%+-]+$`)
	for i, arg := range args {
		if safe.MatchString(arg) {
			quoted[i] = arg
		} else {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
	}
	return strings.Join(quoted, " ")
}

func newRunID() string {
	user := strings.ToLower(os.Getenv("USER"))
	user = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(user, "-")
	if user == "" {
		user = "norma"
	}
	user = user[:min(len(user), 20)]
	return fmt.Sprintf("%s-%s-%d", user, time.Now().Format("20060102-150405"), rand.IntN(100000))
}
