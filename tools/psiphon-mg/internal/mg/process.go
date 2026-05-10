package mg

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

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
	if pid == 0 || !processAlive(pid) {
		return
	}

	_ = syscall.Kill(pid, syscall.SIGTERM)
	for elapsed := 0; processAlive(pid); elapsed++ {
		if elapsed >= timeoutSeconds {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			break
		}
		time.Sleep(time.Second)
	}
	for processAlive(pid) {
		time.Sleep(time.Second)
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
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
		Region:              region,
		PID:                 pid,
		HTTPPort:            opt.HTTPPort,
		SocksPort:           opt.SocksPort,
		BinaryPath:          binaryPath,
		BaseConfig:          opt.BaseConfig,
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
	if state.PID != 0 {
		stopPID(state.PID, DefaultStopTimeout)
		a.log("stopped region %s", fallbackRegion(state.Region))
	}
	_ = removeStateFile(runtimeRoot)
}

func (a *app) cleanupStaleState(runtimeRoot string, state activeState) {
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
