package mg

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const expectedDefaultRegions = "AT,BE,BG,CA,CH,CZ,DE,DK,EE,ES,FI,FR,GB,HU,IE,IN,IT,JP,LV,NL,NO,PL,RO,RS,SE,SG,SK,US"

func TestDefaultRegionsCatalogMatchesExpectedOrder(t *testing.T) {
	repoRoot := findRepoRoot(t)
	if got := defaultRegionsCSV(repoRoot); got != expectedDefaultRegions {
		t.Fatalf("unexpected default regions order: %s", got)
	}
}

func TestDefaultRegionsCatalogFallsBackWhenMissing(t *testing.T) {
	if got := defaultRegionsCSV(t.TempDir()); got != expectedDefaultRegions {
		t.Fatalf("unexpected fallback default regions: %s", got)
	}
}

func TestTunnelsReadyUsesLatestCount(t *testing.T) {
	dir := t.TempDir()
	noticesPath := filepath.Join(dir, "notices.jsonl")
	content := strings.Join([]string{
		`{"noticeType":"Tunnels","data":{"count":1}}`,
		`{"noticeType":"Tunnels","data":{"count":0}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(noticesPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write notices: %v", err)
	}

	if tunnelsReady(noticesPath) {
		t.Fatalf("expected latest tunnel state to be disconnected")
	}
	if got := tunnelsReadyFlag(noticesPath); got != "no" {
		t.Fatalf("unexpected tunnels flag: %s", got)
	}
}

func TestDownloadIfMissingIsRejectedUntilVerificationExists(t *testing.T) {
	repoRoot := t.TempDir()
	baseConfig := filepath.Join(repoRoot, "psiphon.config")
	if err := os.WriteFile(baseConfig, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write base config: %v", err)
	}

	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)
	runtimeRoot := filepath.Join(repoRoot, ".work", "psiphon-mg-test")

	code, stdout, stderr := runCommand([]string{
		"start", "US",
		"--runtime-root", runtimeRoot,
		"--base-config", baseConfig,
		"--binary", filepath.Join(repoRoot, "missing-binary"),
		"--download-if-missing",
	})

	if code != ExitDownloadFailed {
		t.Fatalf("expected ExitDownloadFailed=%d, got %d (stdout=%s stderr=%s)", ExitDownloadFailed, code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "--download-if-missing is disabled in the Go manager") {
		t.Fatalf("expected disabled-download error, got: %s", stderr)
	}
}

func TestNoticeFlagMatchesExactNoticeType(t *testing.T) {
	dir := t.TempDir()
	noticesPath := filepath.Join(dir, "notices.jsonl")
	content := strings.Join([]string{
		`{"noticeType":"ListeningHttpProxyPortExtra","data":{"port":9999}}`,
		`{"noticeType":"ListeningSocksProxyPort","data":{"port":1081}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(noticesPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write notices: %v", err)
	}

	if got := noticeFlag(noticesPath, "ListeningHttpProxyPort"); got != "no" {
		t.Fatalf("expected exact-match http notice miss, got %s", got)
	}
	if got := noticeFlag(noticesPath, "ListeningSocksProxyPort"); got != "yes" {
		t.Fatalf("expected exact-match socks notice hit, got %s", got)
	}
}

func TestLoadStateTreatsPidReuseMismatchAsStale(t *testing.T) {
	runtimeRoot := t.TempDir()
	state := activeState{
		Region:      "US",
		PID:         os.Getpid(),
		RunDir:      filepath.Join(runtimeRoot, "runs", "run-US-test"),
		DataDir:     filepath.Join(runtimeRoot, "runs", "run-US-test", "data"),
		NoticesPath: filepath.Join(runtimeRoot, "runs", "run-US-test", "notices.jsonl"),
	}
	if err := writeStateFile(runtimeRoot, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	app := &app{}
	loadedState, stateKind := app.loadState(runtimeRoot)
	if stateKind != stateStale {
		t.Fatalf("expected stale state for pid mismatch, got %s", stateKind)
	}
	if loadedState.PID != state.PID {
		t.Fatalf("expected pid %d, got %d", state.PID, loadedState.PID)
	}
}

func TestLoadStateTreatsInvalidNumericFieldsAsStale(t *testing.T) {
	runtimeRoot := t.TempDir()
	statePath := stateFilePath(runtimeRoot)
	content := strings.Join([]string{
		`ACTIVE_REGION="US"`,
		`ACTIVE_PID="not-a-number"`,
		`ACTIVE_HTTP_PORT="8081"`,
		`ACTIVE_SOCKS_PORT="1081"`,
	}, "\n") + "\n"
	if err := os.WriteFile(statePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write invalid state file: %v", err)
	}

	stderr := &bytes.Buffer{}
	app := &app{stderr: stderr}
	loadedState, stateKind := app.loadState(runtimeRoot)

	if stateKind != stateStale {
		t.Fatalf("expected stale state for invalid numeric fields, got %s", stateKind)
	}
	if loadedState.Region != "US" {
		t.Fatalf("expected region US to remain readable, got %q", loadedState.Region)
	}
	if loadedState.PID != 0 {
		t.Fatalf("expected invalid pid to remain unset, got %d", loadedState.PID)
	}
	if !strings.Contains(stderr.String(), "invalid ACTIVE_PID value") {
		t.Fatalf("expected parse warning in stderr, got: %s", stderr.String())
	}
}

func TestPrintStatusConnectivityReflectsState(t *testing.T) {
	readyNoticesPath := filepath.Join(t.TempDir(), "notices-ready.jsonl")
	if err := os.WriteFile(readyNoticesPath, []byte(`{"noticeType":"Tunnels","data":{"count":1}}`+"\n"), 0o644); err != nil {
		t.Fatalf("write ready notices: %v", err)
	}

	tests := []struct {
		name             string
		stateKind        managerState
		state            activeState
		wantConnectivity string
	}{
		{name: "stopped", stateKind: stateNone, wantConnectivity: "stopped"},
		{name: "stale", stateKind: stateStale, wantConnectivity: "stale"},
		{name: "running-not-ready", stateKind: stateRunning, state: activeState{}, wantConnectivity: "not-ready"},
		{name: "running-ready", stateKind: stateRunning, state: activeState{NoticesPath: readyNoticesPath}, wantConnectivity: "ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			app := &app{stdout: stdout}
			app.printStatus("/tmp/runtime", tt.stateKind, tt.state)

			if !strings.Contains(stdout.String(), "connectivity="+tt.wantConnectivity) {
				t.Fatalf("expected connectivity=%s in status output, got: %s", tt.wantConnectivity, stdout.String())
			}
		})
	}
}

func TestTrackedPIDMatchesStateMatchesExactManagerArgs(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run-US-test")
	dataDir := filepath.Join(runDir, "data")
	noticesPath := filepath.Join(runDir, "notices.jsonl")
	configPath := filepath.Join(runDir, "config.json")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	if err := os.WriteFile(noticesPath, []byte{}, 0o644); err != nil {
		t.Fatalf("create notices file: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	cmd := newProcessHelperCommand("sleep",
		"-config", configPath,
		"-dataRootDirectory", dataDir,
		"-notices", noticesPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	defer stopTestProcess(t, cmd)

	state := activeState{
		PID:         cmd.Process.Pid,
		RunDir:      runDir,
		DataDir:     dataDir,
		NoticesPath: noticesPath,
	}
	if !trackedPIDMatchesState(state) {
		t.Fatalf("expected tracked pid %d to match exact manager args", state.PID)
	}
}

func TestTrackedPIDMatchesStateRejectsSubstringOnlyArgvMatch(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run-US-test")
	dataDir := filepath.Join(runDir, "data")
	noticesPath := filepath.Join(runDir, "notices.jsonl")
	configPath := filepath.Join(runDir, "config.json")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	if err := os.WriteFile(noticesPath, []byte{}, 0o644); err != nil {
		t.Fatalf("create notices file: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	cmd := newProcessHelperCommand("sleep",
		"-config", configPath+".stale",
		"-dataRootDirectory", dataDir+"-shadow",
		"-notices", noticesPath+".old",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	defer stopTestProcess(t, cmd)

	state := activeState{
		PID:         cmd.Process.Pid,
		RunDir:      runDir,
		DataDir:     dataDir,
		NoticesPath: noticesPath,
	}
	if trackedPIDMatchesState(state) {
		t.Fatalf("expected tracked pid %d to reject substring-only argv matches", state.PID)
	}
}

func TestProcessLivenessTreatsZombieAsExited(t *testing.T) {
	cmd := newProcessHelperCommand("exit-immediately")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start short-lived process: %v", err)
	}

	pid := cmd.Process.Pid
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Wait()
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		stat, ok := readProcessStat(pid)
		if ok && stat.state == 'Z' {
			if processAlive(pid) {
				t.Fatalf("expected zombie pid %d to be treated as exited", pid)
			}
			if processGroupAlive(pid) {
				t.Fatalf("expected zombie-only process group %d to be treated as exited", pid)
			}
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for pid %d to become zombie", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait on zombie child: %v", err)
	}
	waited = true
}

func TestRunLifecycleWithFakeBinary(t *testing.T) {
	if os.Getenv("PSIPHON_MG_GO_INTEGRATION") != "1" {
		t.Skip("set PSIPHON_MG_GO_INTEGRATION=1 to run the fake-binary lifecycle integration test")
	}

	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)

	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	baseConfig := filepath.Join(repoRoot, "psiphon.config")
	fakeBinary := buildFakeTunnelBinary(t, repoRoot)

	if code, out, errOut := runCommand([]string{"status", "--runtime-root", runtimeRoot}); code != 0 {
		t.Fatalf("status failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	} else if !strings.Contains(out, "state=stopped") {
		t.Fatalf("expected stopped status, got: %s", out)
	}

	defer runCommand([]string{"stop", "--runtime-root", runtimeRoot, "--binary", fakeBinary, "--base-config", baseConfig})

	if code, out, errOut := runCommand([]string{"start", "US", "--runtime-root", runtimeRoot, "--binary", fakeBinary, "--base-config", baseConfig, "--ready-timeout-seconds", "5"}); code != 0 {
		t.Fatalf("start failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	if code, out, errOut := runCommand([]string{"current-region", "--runtime-root", runtimeRoot}); code != 0 || strings.TrimSpace(out) != "US" {
		t.Fatalf("current-region after start failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	code, statusOut, errOut := runCommand([]string{"status", "--runtime-root", runtimeRoot})
	if code != 0 {
		t.Fatalf("status after start failed: code=%d stdout=%s stderr=%s", code, statusOut, errOut)
	}
	if !strings.Contains(statusOut, "tunnels_notice=yes") {
		t.Fatalf("expected tunnels_notice=yes, got: %s", statusOut)
	}

	firstPID := readStatusValue(statusOut, "pid")
	firstNotices := readStatusValue(statusOut, "notices_path")
	if firstPID == "0" || firstPID == "" || firstNotices == "" {
		t.Fatalf("missing pid/notices in status: %s", statusOut)
	}

	if err := os.WriteFile(firstNotices, append(mustReadFile(t, firstNotices), []byte(`{"noticeType":"Tunnels","data":{"count":0}}`+"\n")...), 0o644); err != nil {
		t.Fatalf("append disconnect notice: %v", err)
	}

	if code, out, errOut := runCommand([]string{"switch", "US", "--runtime-root", runtimeRoot}); code != 0 {
		t.Fatalf("same-region switch failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	code, statusOut, errOut = runCommand([]string{"status", "--runtime-root", runtimeRoot})
	if code != 0 {
		t.Fatalf("status after same-region restart failed: code=%d stdout=%s stderr=%s", code, statusOut, errOut)
	}
	if !strings.Contains(statusOut, "region=US") || !strings.Contains(statusOut, "tunnels_notice=yes") {
		t.Fatalf("unexpected status after same-region restart: %s", statusOut)
	}
	if refreshedPID := readStatusValue(statusOut, "pid"); refreshedPID == firstPID {
		t.Fatalf("expected pid refresh after disconnected same-region switch: before=%s after=%s", firstPID, refreshedPID)
	}

	if code, out, errOut := runCommand([]string{"switch", "CA", "--runtime-root", runtimeRoot}); code != 0 {
		t.Fatalf("region switch failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	if code, out, errOut := runCommand([]string{"current-region", "--runtime-root", runtimeRoot}); code != 0 || strings.TrimSpace(out) != "CA" {
		t.Fatalf("current-region after switch failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	if code, out, errOut := runCommand([]string{"stop", "--runtime-root", runtimeRoot}); code != 0 {
		t.Fatalf("stop failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	if code, out, errOut := runCommand([]string{"status", "--runtime-root", runtimeRoot}); code != 0 || !strings.Contains(out, "state=stopped") {
		t.Fatalf("status after stop failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
}

func TestStopKillsHelperChildInProcessGroup(t *testing.T) {
	if os.Getenv("PSIPHON_MG_GO_INTEGRATION") != "1" {
		t.Skip("set PSIPHON_MG_GO_INTEGRATION=1 to run the fake-binary lifecycle integration test")
	}

	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)

	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	baseConfig := filepath.Join(repoRoot, "psiphon.config")
	fakeBinary := buildFakeTunnelBinary(t, repoRoot)
	helperPIDFile := filepath.Join(runtimeRoot, "helper.pid")

	t.Setenv("FAKE_PSIPHON_HELPER_IGNORE_TERM", "1")
	t.Setenv("FAKE_PSIPHON_HELPER_PID_FILE", helperPIDFile)
	defer runCommand([]string{"stop", "--runtime-root", runtimeRoot, "--binary", fakeBinary, "--base-config", baseConfig})

	if code, out, errOut := runCommand([]string{"start", "US", "--runtime-root", runtimeRoot, "--binary", fakeBinary, "--base-config", baseConfig, "--ready-timeout-seconds", "5"}); code != 0 {
		t.Fatalf("start failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	helperPIDText, err := os.ReadFile(helperPIDFile)
	if err != nil {
		t.Fatalf("read helper pid: %v", err)
	}
	helperPID, err := strconv.Atoi(strings.TrimSpace(string(helperPIDText)))
	if err != nil {
		t.Fatalf("parse helper pid: %v", err)
	}

	if !processAlive(helperPID) {
		t.Fatalf("expected helper pid %d to be alive before stop", helperPID)
	}

	if code, out, errOut := runCommand([]string{"stop", "--runtime-root", runtimeRoot}); code != 0 {
		t.Fatalf("stop failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if !processAlive(helperPID) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper pid %d remained alive after stop", helperPID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestStopKillsSurvivingProcessGroupFromStaleState(t *testing.T) {
	if os.Getenv("PSIPHON_MG_GO_INTEGRATION") != "1" {
		t.Skip("set PSIPHON_MG_GO_INTEGRATION=1 to run the fake-binary lifecycle integration test")
	}

	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)

	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	baseConfig := filepath.Join(repoRoot, "psiphon.config")
	fakeBinary := buildFakeTunnelBinary(t, repoRoot)
	helperPIDFile := filepath.Join(runtimeRoot, "helper.pid")

	t.Setenv("FAKE_PSIPHON_HELPER_IGNORE_TERM", "1")
	t.Setenv("FAKE_PSIPHON_HELPER_PID_FILE", helperPIDFile)
	defer runCommand([]string{"stop", "--runtime-root", runtimeRoot, "--binary", fakeBinary, "--base-config", baseConfig})

	if code, out, errOut := runCommand([]string{"start", "US", "--runtime-root", runtimeRoot, "--binary", fakeBinary, "--base-config", baseConfig, "--ready-timeout-seconds", "5"}); code != 0 {
		t.Fatalf("start failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	code, statusOut, errOut := runCommand([]string{"status", "--runtime-root", runtimeRoot})
	if code != 0 {
		t.Fatalf("status after start failed: code=%d stdout=%s stderr=%s", code, statusOut, errOut)
	}

	leaderPID, err := strconv.Atoi(readStatusValue(statusOut, "pid"))
	if err != nil || leaderPID <= 0 {
		t.Fatalf("parse leader pid from status %q: %v", statusOut, err)
	}

	helperPIDText, err := os.ReadFile(helperPIDFile)
	if err != nil {
		t.Fatalf("read helper pid: %v", err)
	}
	helperPID, err := strconv.Atoi(strings.TrimSpace(string(helperPIDText)))
	if err != nil {
		t.Fatalf("parse helper pid: %v", err)
	}
	if !processAlive(helperPID) {
		t.Fatalf("expected helper pid %d to be alive before stale cleanup", helperPID)
	}

	if err := syscall.Kill(leaderPID, syscall.SIGKILL); err != nil {
		t.Fatalf("kill leader pid %d: %v", leaderPID, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		code, statusOut, errOut = runCommand([]string{"status", "--runtime-root", runtimeRoot})
		if code != 0 {
			t.Fatalf("status during stale transition failed: code=%d stdout=%s stderr=%s", code, statusOut, errOut)
		}
		if readStatusValue(statusOut, "state") == "stale" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for stale state, last status=%s", statusOut)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !processGroupAlive(leaderPID) {
		t.Fatalf("expected surviving process group for leader pid %d before stale stop", leaderPID)
	}

	if !processAlive(helperPID) {
		t.Fatalf("expected helper pid %d to remain alive before stale stop", helperPID)
	}

	if code, out, errOut := runCommand([]string{"stop", "--runtime-root", runtimeRoot}); code != 0 {
		t.Fatalf("stop from stale state failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	deadline = time.Now().Add(5 * time.Second)
	for {
		if !processAlive(helperPID) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper pid %d remained alive after stale stop", helperPID)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if code, out, errOut := runCommand([]string{"status", "--runtime-root", runtimeRoot}); code != 0 || !strings.Contains(out, "state=stopped") {
		t.Fatalf("status after stale stop failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
}

func runCommand(args []string) (int, string, string) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run(args, stdout, stderr)
	return code, stdout.String(), stderr.String()
}

func readStatusValue(statusText, key string) string {
	for _, line := range strings.Split(statusText, "\n") {
		if value, ok := strings.CutPrefix(line, key+"="); ok {
			return value
		}
	}
	return ""
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func TestManagerProcessHelper(t *testing.T) {
	if os.Getenv("PSIPHON_MG_TEST_HELPER_PROCESS") != "1" {
		return
	}

	args := helperProcessArgs()
	if len(args) == 0 {
		os.Exit(2)
	}

	switch args[0] {
	case "sleep":
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		for range signals {
			os.Exit(0)
		}
	case "exit-immediately":
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func stopTestProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	_ = cmd.Wait()
}

func newProcessHelperCommand(mode string, extraArgs ...string) *exec.Cmd {
	args := []string{"-test.run=TestManagerProcessHelper", "--", mode}
	args = append(args, extraArgs...)
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "PSIPHON_MG_TEST_HELPER_PROCESS=1")
	return cmd
}

func helperProcessArgs() []string {
	for index, arg := range os.Args {
		if arg == "--" {
			return os.Args[index+1:]
		}
	}
	return nil
}

func buildFakeTunnelBinary(t *testing.T, repoRoot string) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "fake-psiphon-tunnel-core-x86_64")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./internal/testhelper/fakepsiphontunnelcore")
	cmd.Dir = filepath.Join(repoRoot, "tools", "psiphon-mg")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake tunnel binary: %v\n%s", err, output)
	}
	return binaryPath
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	current := wd
	for range 8 {
		if fileExists(filepath.Join(current, "psiphon.config")) && fileExists(filepath.Join(current, "tools", "psiphon-mg", "go.mod")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}
