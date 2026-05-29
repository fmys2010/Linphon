package mg

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type processStat struct {
	state          byte
	processGroupID int
}

func makeRunDir(runtimeRoot, region string) (string, error) {
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "runs"), 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(filepath.Join(runtimeRoot, "runs"), "run-"+region+"-")
}

func waitForTunnelReady(pid int, noticesPath string, timeoutSeconds int) bool {
	for elapsed := 0; elapsed <= timeoutSeconds; elapsed++ {
		if tunnelsReady(noticesPath) {
			return true
		}
		if !processAlive(pid) {
			return false
		}
		if elapsed == timeoutSeconds {
			break
		}
		time.Sleep(time.Second)
	}
	return false
}

func stopPID(pid int, timeoutSeconds int) {
	if pid == 0 || (!processAlive(pid) && !processGroupAlive(pid)) {
		return
	}

	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	for elapsed := 0; processAlive(pid) || processGroupAlive(pid); elapsed++ {
		if elapsed >= timeoutSeconds {
			if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			break
		}
		time.Sleep(time.Second)
	}
	for processAlive(pid) || processGroupAlive(pid) {
		time.Sleep(time.Second)
	}
}

func trackedPIDMatchesState(state activeState) bool {
	if state.PID <= 0 || !processAlive(state.PID) {
		return false
	}

	stat, ok := readProcessStat(state.PID)
	if !ok || stat.processGroupID != state.PID {
		return false
	}

	argv := processArgv(state.PID)
	if len(argv) == 0 {
		return false
	}

	expectedPairs := stateExpectedProcessArgs(state)
	if len(expectedPairs) == 0 {
		return false
	}

	for _, expectedPair := range expectedPairs {
		if !argvHasExactFlagValue(argv, expectedPair[0], expectedPair[1]) {
			return false
		}
	}

	return true
}

func stateExpectedProcessArgs(state activeState) [][2]string {
	if state.Provider == installedProviderVG {
		if state.ConfigPath == "" {
			return nil
		}
		return [][2]string{{"--config", state.ConfigPath}}
	}

	expectedPairs := [][2]string{}
	if state.ConfigPath != "" {
		expectedPairs = append(expectedPairs, [2]string{"-config", state.ConfigPath})
	} else if state.RunDir != "" {
		expectedPairs = append(expectedPairs, [2]string{"-config", filepath.Join(state.RunDir, "config.json")})
	}
	if state.DataDir != "" {
		expectedPairs = append(expectedPairs, [2]string{"-dataRootDirectory", state.DataDir})
	}
	if state.NoticesPath != "" {
		expectedPairs = append(expectedPairs, [2]string{"-notices", state.NoticesPath})
	}
	return expectedPairs
}

func trackedProcessGroupSurvives(state activeState) bool {
	return state.PID > 0 && !processAlive(state.PID) && processGroupAlive(state.PID)
}

func processAlive(pid int) bool {
	stat, ok := readProcessStat(pid)
	return ok && stat.state != 'Z'
}

func processGroupAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		memberPID, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		stat, ok := readProcessStat(memberPID)
		if ok && stat.processGroupID == pid && stat.state != 'Z' {
			return true
		}
	}

	return false
}

func readProcessStat(pid int) (processStat, bool) {
	if pid <= 0 {
		return processStat{}, false
	}

	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return processStat{}, false
	}

	closeIndex := bytes.LastIndexByte(content, ')')
	if closeIndex < 0 || closeIndex+2 >= len(content) {
		return processStat{}, false
	}

	fields := strings.Fields(string(content[closeIndex+2:]))
	if len(fields) < 3 || len(fields[0]) != 1 {
		return processStat{}, false
	}

	processGroupID, err := strconv.Atoi(fields[2])
	if err != nil {
		return processStat{}, false
	}

	return processStat{
		state:          fields[0][0],
		processGroupID: processGroupID,
	}, true
}

func processArgv(pid int) []string {
	if pid <= 0 {
		return nil
	}

	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(content) == 0 {
		return nil
	}

	parts := bytes.Split(content, []byte{0})
	argv := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		argv = append(argv, string(part))
	}
	return argv
}

func argvHasExactFlagValue(argv []string, flag, value string) bool {
	if flag == "" || value == "" {
		return false
	}

	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}

	return false
}

func (a *app) launchRegion(region, binaryPath string, opt options) int {
	runDir, err := makeRunDir(opt.RuntimeRoot, region)
	if err != nil {
		a.err("failed to create run directory: %v", err)
		return ExitUsage
	}

	configPath := filepath.Join(runDir, "config.json")
	dataDir := filepath.Join(runDir, "data")
	noticesPath := filepath.Join(runDir, "notices.jsonl")
	stdoutPath := filepath.Join(runDir, "stdout.log")
	stderrPath := filepath.Join(runDir, "stderr.log")
	pidPath := filepath.Join(runDir, "pid")
	remoteFilename := "remote_server_list_" + filepath.Base(runDir)

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		a.err("failed to create data directory: %v", err)
		return ExitUsage
	}
	if err := os.WriteFile(noticesPath, []byte{}, 0o644); err != nil {
		a.err("failed to create notices file: %v", err)
		return ExitUsage
	}
	if err := renderInstanceConfig(opt.BaseConfig, configPath, opt.HTTPPort, opt.SocksPort, remoteFilename, region); err != nil {
		a.err("failed to render config: %v", err)
		return ExitUsage
	}

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		a.err("failed to open stdout log: %v", err)
		return ExitUsage
	}
	defer stdoutFile.Close()

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		a.err("failed to open stderr log: %v", err)
		return ExitUsage
	}
	defer stderrFile.Close()

	stdinFile, err := os.Open("/dev/null")
	if err != nil {
		a.err("failed to open /dev/null: %v", err)
		return ExitUsage
	}
	defer stdinFile.Close()

	cmd := exec.Command(binaryPath,
		"-config", configPath,
		"-dataRootDirectory", dataDir,
		"-notices", noticesPath,
	)
	cmd.Stdin = stdinFile
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		a.err("failed to launch region %s: %v", region, err)
		return ExitUsage
	}

	pid := cmd.Process.Pid
	_ = cmd.Process.Release()

	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		a.err("failed to persist pid file: %v", err)
		return ExitUsage
	}

	state := activeState{
		Provider:            installedProviderPsi,
		Region:              region,
		PID:                 pid,
		HTTPPort:            opt.HTTPPort,
		SocksPort:           opt.SocksPort,
		BinaryPath:          binaryPath,
		BaseConfig:          opt.BaseConfig,
		ConfigPath:          configPath,
		DataDir:             dataDir,
		DownloadIfMissing:   opt.DownloadIfMissing,
		DownloadURL:         opt.DownloadURL,
		NoticesPath:         noticesPath,
		ReadyTimeoutSeconds: opt.ReadyTimeoutSecond,
		RunDir:              runDir,
		StartedAt:           time.Now().UTC().Format(time.RFC3339),
		StderrPath:          stderrPath,
		StdoutPath:          stdoutPath,
	}

	if err := writeStateFile(opt.RuntimeRoot, state); err != nil {
		a.err("failed to persist manager state: %v", err)
		return ExitUsage
	}

	if !waitForTunnelReady(pid, noticesPath, opt.ReadyTimeoutSecond) {
		stopPID(pid, DefaultStopTimeout)
		a.err("timed out waiting for tunnel-ready notice for region %s", region)
		_ = removeStateFile(opt.RuntimeRoot)
		return ExitReadyTimeout
	}

	a.log("region %s is ready (pid %d, ports %d/%d)", region, pid, opt.HTTPPort, opt.SocksPort)
	return 0
}

func (a *app) stopActiveState(runtimeRoot string, state activeState) {
	if trackedPIDMatchesState(state) {
		stopPID(state.PID, DefaultStopTimeout)
		a.log("stopped region %s", fallbackRegion(state.Region))
	} else if trackedProcessGroupSurvives(state) {
		stopPID(state.PID, DefaultStopTimeout)
		a.log("stopped surviving process group for region %s", fallbackRegion(state.Region))
	} else if state.PID != 0 {
		a.log("tracked pid %d no longer matches active region %s; clearing state without signaling", state.PID, fallbackRegion(state.Region))
	}
	_ = removeStateFile(runtimeRoot)
}

func (a *app) cleanupStaleState(runtimeRoot string, state activeState) {
	if trackedProcessGroupSurvives(state) {
		stopPID(state.PID, DefaultStopTimeout)
		a.log("stopped surviving stale process group for region %s", fallbackRegion(state.Region))
	}

	if state.PID != 0 {
		a.log("clearing stale manager state for region %s (pid %d)", fallbackRegion(state.Region), state.PID)
	} else {
		a.log("clearing stale manager state")
	}
	_ = removeStateFile(runtimeRoot)
}

func fallbackRegion(region string) string {
	if region == "" {
		return "unknown"
	}
	return region
}
