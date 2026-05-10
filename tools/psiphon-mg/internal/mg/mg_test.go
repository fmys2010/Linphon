package mg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRunLifecycleWithFakeBinary(t *testing.T) {
	if os.Getenv("PSIPHON_MG_GO_INTEGRATION") != "1" {
		t.Skip("set PSIPHON_MG_GO_INTEGRATION=1 to run the fake-binary lifecycle integration test")
	}

	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)

	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	baseConfig := filepath.Join(repoRoot, "psiphon.config")
	fakeBinary := filepath.Join(repoRoot, "tests", "fake-psiphon-tunnel-core-x86_64")

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

func runCommand(args []string) (int, string, string) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run(args, stdout, stderr)
	return code, stdout.String(), stderr.String()
}

func readStatusValue(statusText, key string) string {
	for _, line := range strings.Split(statusText, "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
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

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	current := wd
	for range 8 {
		if fileExists(filepath.Join(current, "psiphon.config")) && fileExists(filepath.Join(current, "tests", "fake-psiphon-tunnel-core-x86_64")) {
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
