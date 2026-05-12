package mg

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultBinaryDownloadURL = "https://raw.githubusercontent.com/Psiphon-Labs/psiphon-tunnel-core-binaries/master/linux/psiphon-tunnel-core-x86_64"
	DefaultReadyTimeout      = 30
	DefaultStopTimeout       = 10

	ExitUsage            = 64
	ExitBinaryNotFound   = 65
	ExitDownloadFailed   = 66
	ExitInstanceFailed   = 67
	ExitValidationFailed = 68
	ExitLocked           = 69
	ExitNotRunning       = 70
	ExitAlreadyRunning   = 71
	ExitReadyTimeout     = 72
)

type managerState string

const (
	stateNone    managerState = "none"
	stateRunning managerState = "running"
	stateStale   managerState = "stale"
)

type options struct {
	RuntimeRoot        string
	BinaryPath         string
	DownloadIfMissing  bool
	DownloadURL        string
	BaseConfig         string
	HTTPPort           int
	SocksPort          int
	ReadyTimeoutSecond int
}

type activeState struct {
	Region              string
	PID                 int
	HTTPPort            int
	SocksPort           int
	BinaryPath          string
	BaseConfig          string
	DataDir             string
	DownloadIfMissing   bool
	DownloadURL         string
	NoticesPath         string
	ReadyTimeoutSeconds int
	RunDir              string
	StartedAt           string
	StderrPath          string
	StdoutPath          string
}

type app struct {
	stdout   io.Writer
	stderr   io.Writer
	repoRoot string
	owner    string
}

type tunnelNotice struct {
	NoticeType string `json:"noticeType"`
	Data       struct {
		Count int `json:"count"`
	} `json:"data"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	app := &app{
		stdout:   stdout,
		stderr:   stderr,
		repoRoot: resolveRepoRoot(),
		owner:    os.Args[0],
	}

	return app.run(args)
}

func resolveRepoRoot() string {
	if fromEnv := strings.TrimSpace(os.Getenv("PSIPHON_MG_REPO_ROOT")); fromEnv != "" {
		return fromEnv
	}

	if fromExecutable := resolveRepoRootFromExecutable(); fromExecutable != "" {
		return fromExecutable
	}

	wd, err := os.Getwd()
	if err == nil {
		return wd
	}

	return "."
}

func resolveRepoRootFromExecutable() string {
	executablePath, err := os.Executable()
	if err != nil {
		return ""
	}

	currentDir := filepath.Dir(executablePath)
	for {
		if fileExists(filepath.Join(currentDir, "psiphon.config")) && fileExists(filepath.Join(currentDir, "tools", "psiphon-mg", "go.mod")) {
			return currentDir
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return ""
		}
		currentDir = parentDir
	}
}

func (a *app) run(args []string) int {
	if len(args) == 0 {
		a.usage(a.stderr)
		return ExitUsage
	}

	command := args[0]
	remaining := args[1:]
	region := ""

	switch command {
	case "start", "switch":
		if len(remaining) == 0 {
			a.err("%s requires a REGION argument", command)
			a.usage(a.stderr)
			return ExitUsage
		}
		region = remaining[0]
		remaining = remaining[1:]
	case "stop", "status", "current-region":
	case "--help", "-h", "help":
		a.usage(a.stdout)
		return 0
	default:
		a.err("unknown command: %s", command)
		a.usage(a.stderr)
		return ExitUsage
	}

	defaultBaseConfig := filepath.Join(a.repoRoot, "psiphon.config")
	defaultHTTPPort, defaultSocksPort := readDefaultPorts(defaultBaseConfig)
	runtimeRoot := extractRuntimeRoot(filepath.Join(a.repoRoot, ".work", "psiphon-mg"), remaining)

	release, lockCode := a.acquireLock(runtimeRoot)
	if lockCode != 0 {
		return lockCode
	}
	defer release()

	loadedState, stateKind := a.loadState(runtimeRoot)

	opt := options{
		RuntimeRoot:        runtimeRoot,
		DownloadURL:        DefaultBinaryDownloadURL,
		BaseConfig:         defaultBaseConfig,
		HTTPPort:           defaultHTTPPort,
		SocksPort:          defaultSocksPort,
		ReadyTimeoutSecond: DefaultReadyTimeout,
	}

	if command == "switch" && stateKind != stateNone {
		if loadedState.BinaryPath != "" {
			opt.BinaryPath = loadedState.BinaryPath
		}
		opt.DownloadIfMissing = loadedState.DownloadIfMissing
		if loadedState.DownloadURL != "" {
			opt.DownloadURL = loadedState.DownloadURL
		}
		if loadedState.BaseConfig != "" {
			opt.BaseConfig = loadedState.BaseConfig
		}
		if loadedState.HTTPPort > 0 {
			opt.HTTPPort = loadedState.HTTPPort
		}
		if loadedState.SocksPort > 0 {
			opt.SocksPort = loadedState.SocksPort
		}
		if loadedState.ReadyTimeoutSeconds > 0 {
			opt.ReadyTimeoutSecond = loadedState.ReadyTimeoutSeconds
		}
	}

	parseCode := a.parseOptions(command, remaining, &opt)
	if parseCode == -1 {
		return 0
	}
	if parseCode != 0 {
		return parseCode
	}

	switch command {
	case "start":
		return a.commandStart(region, stateKind, loadedState, opt)
	case "switch":
		return a.commandSwitch(region, stateKind, loadedState, opt)
	case "stop":
		return a.commandStop(stateKind, loadedState, opt.RuntimeRoot)
	case "status":
		a.printStatus(opt.RuntimeRoot, stateKind, loadedState)
		return 0
	case "current-region":
		if stateKind != stateRunning {
			return ExitNotRunning
		}
		fmt.Fprintln(a.stdout, loadedState.Region)
		return 0
	default:
		return ExitUsage
	}
}

func (a *app) usage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  psiphon-mg start REGION [options]
  psiphon-mg switch REGION [options]
  psiphon-mg stop [options]
  psiphon-mg status [options]
  psiphon-mg current-region [options]

Commands:
  start REGION       Start a repo-local Psiphon child for REGION.
  switch REGION      Switch to REGION while keeping one active child.
  stop               Stop the active child if present.
  status             Report manager state, region, pid, ports, and notice flags.
  current-region     Print only the active region when a live child is running.

Options:
  --runtime-root PATH           Runtime root (default: ./.work/psiphon-mg).
  --binary PATH                 Explicit binary path.
	--download-if-missing         Reserved for future verified downloads; currently rejected.
	--download-url URL            Reserved for future verified downloads.
  --base-config PATH            Base config template (default: ./psiphon.config).
  --http-port N                 Stable LocalHttpProxyPort (default: 8081).
  --socks-port N                Stable LocalSocksProxyPort (default: 1081).
  --ready-timeout N             Seconds to wait for a tunnel-ready signal.
  --ready-timeout-seconds N     Alias for --ready-timeout.
  --help                        Show this message.

Artifacts:
  runtime-root/
    active.env
    lock/
    runs/run-<region>-XXXXXXXX/
      config.json
      data/
      notices.jsonl
      stdout.log
      stderr.log
      pid
`)
}

func (a *app) parseOptions(command string, args []string, opt *options) int {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--runtime-root":
			if i+1 >= len(args) {
				a.err("--runtime-root requires a value")
				return ExitUsage
			}
			opt.RuntimeRoot = args[i+1]
			i++
		case "--binary":
			if i+1 >= len(args) {
				a.err("--binary requires a value")
				return ExitUsage
			}
			opt.BinaryPath = args[i+1]
			i++
		case "--download-if-missing":
			opt.DownloadIfMissing = true
		case "--download-url":
			if i+1 >= len(args) {
				a.err("--download-url requires a value")
				return ExitUsage
			}
			opt.DownloadURL = args[i+1]
			i++
		case "--base-config":
			if i+1 >= len(args) {
				a.err("--base-config requires a value")
				return ExitUsage
			}
			opt.BaseConfig = args[i+1]
			i++
		case "--http-port":
			if i+1 >= len(args) {
				a.err("--http-port requires a value")
				return ExitUsage
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				a.err("HTTP port must be greater than zero: %s", args[i+1])
				return ExitUsage
			}
			opt.HTTPPort = port
			i++
		case "--socks-port":
			if i+1 >= len(args) {
				a.err("--socks-port requires a value")
				return ExitUsage
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				a.err("SOCKS port must be greater than zero: %s", args[i+1])
				return ExitUsage
			}
			opt.SocksPort = port
			i++
		case "--ready-timeout", "--ready-timeout-seconds":
			if i+1 >= len(args) {
				a.err("%s requires a value", args[i])
				return ExitUsage
			}
			seconds, err := strconv.Atoi(args[i+1])
			if err != nil {
				a.err("ready timeout must be a non-negative integer: %s", args[i+1])
				return ExitUsage
			}
			opt.ReadyTimeoutSecond = seconds
			i++
		case "--help":
			a.usage(a.stdout)
			return -1
		default:
			a.err("unknown %s option: %s", command, args[i])
			return ExitUsage
		}
	}

	if opt.HTTPPort <= 0 {
		a.err("HTTP port must be greater than zero: %d", opt.HTTPPort)
		return ExitUsage
	}
	if opt.SocksPort <= 0 {
		a.err("SOCKS port must be greater than zero: %d", opt.SocksPort)
		return ExitUsage
	}
	if opt.ReadyTimeoutSecond < 0 {
		a.err("ready timeout must be a non-negative integer: %d", opt.ReadyTimeoutSecond)
		return ExitUsage
	}

	return 0
}

func (a *app) commandStart(region string, stateKind managerState, state activeState, opt options) int {
	if stateKind == stateRunning {
		a.err("region %s is already active with pid %d; use switch or stop first", state.Region, state.PID)
		return ExitAlreadyRunning
	}

	if stateKind == stateStale {
		a.cleanupStaleState(opt.RuntimeRoot, state)
	}

	code := a.validateStartInputs(region, opt)
	if code != 0 {
		return code
	}

	resolvedBinary, code := a.resolveManagerBinary(opt)
	if code != 0 {
		return code
	}

	return a.launchRegion(region, resolvedBinary, opt)
}

func (a *app) commandSwitch(region string, stateKind managerState, state activeState, opt options) int {
	code := a.validateStartInputs(region, opt)
	if code != 0 {
		return code
	}

	shouldStop := false
	if stateKind == stateRunning {
		if state.Region == region && tunnelsReadyFlag(state.NoticesPath) == "yes" {
			a.log("region %s is already active and tunnel-ready", region)
			return 0
		}
		shouldStop = true
	} else if stateKind == stateStale {
		a.cleanupStaleState(opt.RuntimeRoot, state)
	}

	resolvedBinary, code := a.resolveManagerBinary(opt)
	if code != 0 {
		return code
	}

	if shouldStop {
		a.stopActiveState(opt.RuntimeRoot, state)
	}

	return a.launchRegion(region, resolvedBinary, opt)
}

func (a *app) commandStop(stateKind managerState, state activeState, runtimeRoot string) int {
	if stateKind == stateStale {
		a.cleanupStaleState(runtimeRoot, state)
		return 0
	}

	if stateKind != stateRunning {
		a.log("no active region to stop")
		_ = removeStateFile(runtimeRoot)
		return 0
	}

	a.stopActiveState(runtimeRoot, state)
	return 0
}

func (a *app) validateStartInputs(region string, opt options) int {
	if !isKnownRegion(a.repoRoot, region) {
		a.err("unknown region code: %s", region)
		return ExitValidationFailed
	}
	if _, err := os.Stat(opt.BaseConfig); err != nil {
		a.err("base config not found: %s", opt.BaseConfig)
		return ExitUsage
	}
	return 0
}

func (a *app) resolveManagerBinary(opt options) (string, int) {
	if binaryPath, ok := resolveBinary(a.repoRoot, opt.BinaryPath, opt.RuntimeRoot); ok {
		return binaryPath, 0
	}

	if opt.DownloadIfMissing {
		a.err("--download-if-missing is disabled in the Go manager until executable authenticity verification is implemented; provide --binary or place psiphon-tunnel-core-x86_64 in the repo/runtime bin path")
		return "", ExitDownloadFailed
	}

	a.err("unable to locate psiphon-tunnel-core-x86_64")
	return "", ExitBinaryNotFound
}

func (a *app) log(format string, args ...any) {
	fmt.Fprintf(a.stdout, "[psiphon-mg] %s\n", fmt.Sprintf(format, args...))
}

func (a *app) err(format string, args ...any) {
	fmt.Fprintf(a.stderr, "[psiphon-mg] ERROR: %s\n", fmt.Sprintf(format, args...))
}
