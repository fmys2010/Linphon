package mg

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHarnessLocateBinaryUsesRuntimeBinFallback(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	binaryPath := filepath.Join(runtimeRoot, "bin", "psiphon-tunnel-core-x86_64")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("create runtime bin: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte(""), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := RunMultiInstance([]string{"locate-binary", "--runtime-root", runtimeRoot}, stdout, stderr)
	if code != 0 {
		t.Fatalf("locate-binary failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != binaryPath {
		t.Fatalf("unexpected located binary path: %q", stdout.String())
	}
}

func TestHarnessDownloadCommandsRemainDisabled(t *testing.T) {
	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)

	t.Run("download-binary", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		code := RunMultiInstance([]string{"download-binary"}, stdout, stderr)
		if code != ExitDownloadFailed {
			t.Fatalf("expected ExitDownloadFailed=%d, got %d", ExitDownloadFailed, code)
		}
		if !strings.Contains(stderr.String(), "disabled until executable authenticity verification exists") {
			t.Fatalf("missing disabled download error: %s", stderr.String())
		}
	})

	t.Run("run-download-if-missing", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		code := RunMultiInstance([]string{"run", "--download-if-missing", "--base-config", filepath.Join(repoRoot, "psiphon.config")}, stdout, stderr)
		if code != ExitDownloadFailed {
			t.Fatalf("expected ExitDownloadFailed=%d, got %d", ExitDownloadFailed, code)
		}
		if !strings.Contains(stderr.String(), "disabled until executable authenticity verification exists") {
			t.Fatalf("missing disabled download error: %s", stderr.String())
		}
	})

		t.Run("run-download-url", func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			code := RunMultiInstance([]string{"run", "--binary", filepath.Join(repoRoot, "missing-binary"), "--download-url", "https://example.invalid/core", "--base-config", filepath.Join(repoRoot, "psiphon.config")}, stdout, stderr)
			if code != ExitDownloadFailed {
				t.Fatalf("expected ExitDownloadFailed=%d, got %d", ExitDownloadFailed, code)
			}
		if !strings.Contains(stderr.String(), "disabled until executable authenticity verification exists") {
			t.Fatalf("missing disabled download error: %s", stderr.String())
		}
	})
}

func TestHarnessRunWithFakeBinaryPreservesShellArtifacts(t *testing.T) {
	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)
	fakeBinary := buildFakeTunnelBinary(t, repoRoot)
	runtimeRoot := filepath.Join(t.TempDir(), "single")
	baseConfig := filepath.Join(repoRoot, "psiphon.config")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := RunMultiInstance([]string{
		"run",
		"--binary", fakeBinary,
		"--base-config", baseConfig,
		"--runtime-root", runtimeRoot,
		"--run-name", "smoke-3",
		"--count", "3",
		"--http-port-base", "19080",
		"--socks-port-base", "12080",
		"--wait-seconds", "1",
		"--startup-grace-seconds", "1",
	}, stdout, stderr)
	if code != 0 {
		t.Fatalf("run failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	runDir := filepath.Join(runtimeRoot, "runs", "smoke-3")
	summaryPath := filepath.Join(runDir, "summary.tsv")
	metricsPath := filepath.Join(runDir, "metrics-final.tsv")
	cgroupStartPath := filepath.Join(runDir, "cgroup-start.snapshot")
	cgroupFinalPath := filepath.Join(runDir, "cgroup-final.snapshot")
	runEnvPath := filepath.Join(runDir, "run.env")
	instancesDir := filepath.Join(runDir, "instances")

	requireDir(t, runDir)
	requireDir(t, instancesDir)
	requireFile(t, summaryPath)
	requireFile(t, metricsPath)
	requireFile(t, cgroupStartPath)
	requireFile(t, cgroupFinalPath)
	requireFile(t, runEnvPath)

	summaryRows := readTSV(t, summaryPath)
	if len(summaryRows) != 4 {
		t.Fatalf("expected 4 summary lines, got %d", len(summaryRows))
	}
	metricsRows := readTSV(t, metricsPath)
	if len(metricsRows) != 4 {
		t.Fatalf("expected 4 metrics lines, got %d", len(metricsRows))
	}
	instanceEntries, err := os.ReadDir(instancesDir)
	if err != nil {
		t.Fatalf("read instances dir: %v", err)
	}
	if len(instanceEntries) != 3 {
		t.Fatalf("expected 3 instance directories, got %d", len(instanceEntries))
	}

	regions := []string{}
	remoteNames := map[string]struct{}{}
	for index := 1; index < len(summaryRows); index++ {
		row := summaryRows[index]
		if len(row) != 14 {
			t.Fatalf("expected 14 summary columns, got %d: %v", len(row), row)
		}
		if row[5] != "yes" || row[6] != "yes" || row[7] != "yes" || row[8] != "yes" {
			t.Fatalf("expected running/http/socks/tunnels yes row, got %v", row)
		}
		regions = append(regions, row[2])
		config := readConfigJSON(t, row[9])
		remoteName, _ := config["RemoteServerListDownloadFilename"].(string)
		remoteNames[remoteName] = struct{}{}
	}
	if strings.Join(regions, ",") != "AT,BE,BG" {
		t.Fatalf("unexpected region order: %s", strings.Join(regions, ","))
	}
	if len(remoteNames) != 3 {
		t.Fatalf("expected 3 unique remote list filenames, got %d", len(remoteNames))
	}

	if !strings.Contains(string(mustReadFile(t, cgroupStartPath)), "memory.current\t") && !strings.Contains(string(mustReadFile(t, cgroupStartPath)), "memory.usage_in_bytes\t") {
		t.Fatalf("expected memory cgroup probe in start snapshot")
	}
	if !strings.Contains(string(mustReadFile(t, cgroupFinalPath)), "pids.current\t") && !strings.Contains(string(mustReadFile(t, cgroupFinalPath)), "pids.current.v1\t") {
		t.Fatalf("expected pids cgroup probe in final snapshot")
	}

	if !strings.Contains(string(mustReadFile(t, runEnvPath)), "REGIONS=AT,BE,BG") {
		t.Fatalf("expected run.env to record selected regions")
	}
	assertConfigRegion(t, filepath.Join(runDir, "instances", "instance-001", "config.json"), "AT")
	assertConfigRegion(t, filepath.Join(runDir, "instances", "instance-002", "config.json"), "BE")
	assertConfigRegion(t, filepath.Join(runDir, "instances", "instance-003", "config.json"), "BG")
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected file, got directory: %s", path)
	}
}

func requireDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat dir %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got file: %s", path)
	}
}

func readTSV(t *testing.T, path string) [][]string {
	t.Helper()
	content := strings.TrimSpace(string(mustReadFile(t, path)))
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

func readConfigJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(mustReadFile(t, path), &parsed); err != nil {
		t.Fatalf("parse config %s: %v", path, err)
	}
	return parsed
}

func assertConfigRegion(t *testing.T, path, wantRegion string) {
	t.Helper()
	parsed := readConfigJSON(t, path)
	if got, _ := parsed["EgressRegion"].(string); got != wantRegion {
		t.Fatalf("unexpected EgressRegion in %s: got=%s want=%s", path, got, wantRegion)
	}
}
