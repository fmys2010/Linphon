package mg

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStagedRunnerDownloadFlagsRemainDisabled(t *testing.T) {
	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)

	t.Run("download-if-missing", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		code := RunStaged([]string{"--download-if-missing"}, stdout, stderr)
		if code != ExitDownloadFailed {
			t.Fatalf("expected ExitDownloadFailed=%d, got %d", ExitDownloadFailed, code)
		}
		if !strings.Contains(stderr.String(), "disabled until executable authenticity verification exists") {
			t.Fatalf("missing disabled download error: %s", stderr.String())
		}
	})

	t.Run("download-url", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		code := RunStaged([]string{"--download-url", "https://example.invalid/core"}, stdout, stderr)
		if code != ExitDownloadFailed {
			t.Fatalf("expected ExitDownloadFailed=%d, got %d", ExitDownloadFailed, code)
		}
		if !strings.Contains(stderr.String(), "disabled until executable authenticity verification exists") {
			t.Fatalf("missing disabled download error: %s", stderr.String())
		}
	})
}

func TestStagedRunnerCreatesFixedStagesAndArtifacts(t *testing.T) {
	repoRoot := findRepoRoot(t)
	t.Setenv("PSIPHON_MG_REPO_ROOT", repoRoot)
	fakeBinary := buildFakeTunnelBinary(t, repoRoot)
	runtimeRoot := filepath.Join(t.TempDir(), "staged")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := RunStaged([]string{
		"--binary", fakeBinary,
		"--base-config", filepath.Join(repoRoot, "psiphon.config"),
		"--runtime-root", runtimeRoot,
		"--wait-seconds", "1",
		"--startup-grace-seconds", "1",
	}, stdout, stderr)
	if code != 0 {
		t.Fatalf("staged run failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	resultsPath := filepath.Join(runtimeRoot, "stage-results.tsv")
	requireFile(t, resultsPath)
	rows := readTSV(t, resultsPath)
	if len(rows) != 4 {
		t.Fatalf("expected 4 stage result lines, got %d", len(rows))
	}

	wantStages := []int{3, 8, 28}
	for index, wantStage := range wantStages {
		row := rows[index+1]
		if len(row) != 5 {
			t.Fatalf("expected 5 stage result columns, got %d: %v", len(row), row)
		}
		gotStage, err := strconv.Atoi(row[0])
		if err != nil || gotStage != wantStage {
			t.Fatalf("unexpected stage row %v", row)
		}
		if row[1] != "0" {
			t.Fatalf("expected stage %d exit 0, got %s", wantStage, row[1])
		}
		if !strings.HasSuffix(row[2], filepath.Join("runs", "stage-"+strconv.Itoa(wantStage))) {
			t.Fatalf("unexpected run dir path: %s", row[2])
		}
	}

	stage28Dir := filepath.Join(runtimeRoot, "runs", "stage-28")
	stage28Summary := filepath.Join(stage28Dir, "summary.tsv")
	stage28Metrics := filepath.Join(stage28Dir, "metrics-final.tsv")
	requireFile(t, stage28Summary)
	requireFile(t, stage28Metrics)
	requireFile(t, filepath.Join(stage28Dir, "cgroup-start.snapshot"))
	requireFile(t, filepath.Join(stage28Dir, "cgroup-final.snapshot"))

	stage28SummaryRows := readTSV(t, stage28Summary)
	if len(stage28SummaryRows) != 29 {
		t.Fatalf("expected 29 stage-28 summary lines, got %d", len(stage28SummaryRows))
	}
	stage28MetricsRows := readTSV(t, stage28Metrics)
	if len(stage28MetricsRows) != 29 {
		t.Fatalf("expected 29 stage-28 metrics lines, got %d", len(stage28MetricsRows))
	}

	regions := make([]string, 0, 28)
	for _, row := range stage28SummaryRows[1:] {
		regions = append(regions, row[2])
	}
	if strings.Join(regions, ",") != expectedDefaultRegions {
		t.Fatalf("unexpected stage-28 region order: %s", strings.Join(regions, ","))
	}

	assertStageBasePort(t, filepath.Join(runtimeRoot, "runs", "stage-3", "summary.tsv"), 18110, 11110)
	assertStageBasePort(t, filepath.Join(runtimeRoot, "runs", "stage-8", "summary.tsv"), 18160, 11160)
	assertStageBasePort(t, filepath.Join(runtimeRoot, "runs", "stage-28", "summary.tsv"), 18360, 11360)
}

func assertStageBasePort(t *testing.T, summaryPath string, wantHTTP, wantSocks int) {
	t.Helper()
	rows := readTSV(t, summaryPath)
	if len(rows) < 2 {
		t.Fatalf("summary missing rows: %s", summaryPath)
	}
	row := rows[1]
	if row[3] != strconv.Itoa(wantHTTP) || row[4] != strconv.Itoa(wantSocks) {
		t.Fatalf("unexpected base ports in %s: got http=%s socks=%s want http=%d socks=%d", summaryPath, row[3], row[4], wantHTTP, wantSocks)
	}
}
