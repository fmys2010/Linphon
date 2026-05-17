package mg

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type stagedApp struct {
	stdout    io.Writer
	stderr    io.Writer
	repoRoot  string
	usageName string
}

type stagedOptions struct {
	BinaryPath          string
	BaseConfig          string
	RuntimeRoot         string
	RegionsCSV          string
	WaitSeconds         int
	StartupGraceSeconds int
	DownloadIfMissing   bool
	DownloadURL         string
	DownloadURLProvided bool
}

func RunStaged(args []string, stdout, stderr io.Writer) int {
	return RunStagedNamed(args, "run-psiphon-staged", stdout, stderr)
}

func RunStagedNamed(args []string, usageName string, stdout, stderr io.Writer) int {
	if usageName == "" {
		usageName = "run-psiphon-staged"
	}

	app := &stagedApp{
		stdout:    stdout,
		stderr:    stderr,
		repoRoot:  resolveRepoRoot(),
		usageName: usageName,
	}
	return app.run(args)
}

func (a *stagedApp) run(args []string) int {
	opt := stagedOptions{
		BaseConfig:          filepath.Join(a.repoRoot, "psiphon.config"),
		RuntimeRoot:         filepath.Join(a.repoRoot, ".work", "psiphon-harness-staged"),
		WaitSeconds:         5,
		StartupGraceSeconds: 2,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--binary":
			if i+1 >= len(args) {
				a.err("--binary requires a value")
				return ExitUsage
			}
			opt.BinaryPath = args[i+1]
			i++
		case "--download-if-missing":
			a.downloadDisabled()
			return ExitDownloadFailed
		case "--download-url":
			if i+1 >= len(args) {
				a.err("--download-url requires a value")
				return ExitUsage
			}
			opt.DownloadURL = args[i+1]
			opt.DownloadURLProvided = true
			a.downloadDisabled()
			return ExitDownloadFailed
		case "--base-config":
			if i+1 >= len(args) {
				a.err("--base-config requires a value")
				return ExitUsage
			}
			opt.BaseConfig = args[i+1]
			i++
		case "--runtime-root":
			if i+1 >= len(args) {
				a.err("--runtime-root requires a value")
				return ExitUsage
			}
			opt.RuntimeRoot = args[i+1]
			i++
		case "--regions":
			if i+1 >= len(args) {
				a.err("--regions requires a value")
				return ExitUsage
			}
			opt.RegionsCSV = args[i+1]
			i++
		case "--wait-seconds":
			if i+1 >= len(args) {
				a.err("--wait-seconds requires a value")
				return ExitUsage
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value < 0 {
				a.err("wait seconds must be a non-negative integer: %s", args[i+1])
				return ExitUsage
			}
			opt.WaitSeconds = value
			i++
		case "--startup-grace-seconds":
			if i+1 >= len(args) {
				a.err("--startup-grace-seconds requires a value")
				return ExitUsage
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value < 0 {
				a.err("startup grace seconds must be a non-negative integer: %s", args[i+1])
				return ExitUsage
			}
			opt.StartupGraceSeconds = value
			i++
		case "--help":
			a.usage(a.stdout)
			return 0
		default:
			a.err("unknown option: %s", args[i])
			a.usage(a.stderr)
			return ExitUsage
		}
	}

	if err := os.MkdirAll(opt.RuntimeRoot, 0o755); err != nil {
		a.err("failed to create runtime root: %v", err)
		return ExitValidationFailed
	}

	resultsPath := filepath.Join(opt.RuntimeRoot, "stage-results.tsv")
	resultsFile, err := os.Create(resultsPath)
	if err != nil {
		a.err("failed to create stage results: %v", err)
		return ExitValidationFailed
	}
	if _, err := fmt.Fprintln(resultsFile, "stage\texit_code\trun_dir\tsummary_path\tmetrics_path"); err != nil {
		_ = resultsFile.Close()
		a.err("failed to initialize stage results: %v", err)
		return ExitValidationFailed
	}

	overallExit := 0
	for _, count := range []int{3, 8, 28} {
		runName := fmt.Sprintf("stage-%d", count)
		runDir := filepath.Join(opt.RuntimeRoot, "runs", runName)
		summaryPath := filepath.Join(runDir, "summary.tsv")
		metricsPath := filepath.Join(runDir, "metrics-final.tsv")
		httpBase := 18080 + (count * 10)
		socksBase := 11080 + (count * 10)

		a.log("running %d instance stage", count)
		_ = os.RemoveAll(runDir)

		harnessArgs := []string{
			"run",
			"--base-config", opt.BaseConfig,
			"--runtime-root", opt.RuntimeRoot,
			"--run-name", runName,
			"--count", strconv.Itoa(count),
			"--http-port-base", strconv.Itoa(httpBase),
			"--socks-port-base", strconv.Itoa(socksBase),
			"--wait-seconds", strconv.Itoa(opt.WaitSeconds),
			"--startup-grace-seconds", strconv.Itoa(opt.StartupGraceSeconds),
		}
		if opt.RegionsCSV != "" {
			harnessArgs = append(harnessArgs, "--regions", opt.RegionsCSV)
		}
		if opt.BinaryPath != "" {
			harnessArgs = append(harnessArgs, "--binary", opt.BinaryPath)
		}

		stageExit := RunMultiInstance(harnessArgs, a.stdout, a.stderr)
		if stageExit != 0 {
			overallExit = 1
		}
		if _, err := fmt.Fprintf(resultsFile, "%d\t%d\t%s\t%s\t%s\n", count, stageExit, runDir, summaryPath, metricsPath); err != nil {
			_ = resultsFile.Close()
			a.err("failed to append stage result: %v", err)
			return ExitValidationFailed
		}
	}

	if err := resultsFile.Close(); err != nil {
		a.err("failed to finalize stage results: %v", err)
		return ExitValidationFailed
	}

	a.log("results: %s", resultsPath)
	return overallExit
}

func (a *stagedApp) usage(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  %s [options]

Options:
  --binary PATH                 Explicit binary path.
  --download-if-missing         Disabled until executable authenticity verification exists.
  --download-url URL            Disabled until executable authenticity verification exists.
  --base-config PATH            Base config template.
  --runtime-root PATH           Runtime root for staged runs.
  --regions CSV                 Override comma-separated region list.
  --wait-seconds N              Seconds to wait before final metrics per stage.
  --startup-grace-seconds N     Seconds to allow each stage to initialize.
  --help                        Show this message.
`, a.usageName)
}

func (a *stagedApp) log(format string, args ...any) {
	fmt.Fprintf(a.stdout, "[staged] %s\n", fmt.Sprintf(format, args...))
}

func (a *stagedApp) err(format string, args ...any) {
	fmt.Fprintf(a.stderr, "[staged] ERROR: %s\n", fmt.Sprintf(format, args...))
}

func (a *stagedApp) downloadDisabled() {
	a.err("automatic download is disabled until executable authenticity verification exists")
}
