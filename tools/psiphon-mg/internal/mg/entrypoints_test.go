package mg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPsiphonReturns127WhenBinaryIsMissing(t *testing.T) {
	prevBinary := installedPsiphonBinaryPath
	installedPsiphonBinaryPath = filepath.Join(t.TempDir(), "missing-psiphon-binary")
	defer func() {
		installedPsiphonBinaryPath = prevBinary
	}()

	if code := RunPsiphon(&bytes.Buffer{}, &bytes.Buffer{}); code != 127 {
		t.Fatalf("expected missing-binary exit 127, got %d", code)
	}
}

func TestPluninstallerRemovesConfiguredPaths(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	launcherPath := filepath.Join(t.TempDir(), "usr", "bin", "psiphon")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		t.Fatalf("create launcher dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed config dir: %v", err)
	}
	if err := os.WriteFile(launcherPath, []byte("launcher\n"), 0o755); err != nil {
		t.Fatalf("seed launcher: %v", err)
	}

	prevConfigDir := installedPsiphonConfigDir
	prevLauncher := installedPsiphonLauncher
	installedPsiphonConfigDir = configDir
	installedPsiphonLauncher = launcherPath
	defer func() {
		installedPsiphonConfigDir = prevConfigDir
		installedPsiphonLauncher = prevLauncher
	}()

	if code := RunPluninstaller(&bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("expected config dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(launcherPath); !os.IsNotExist(err) {
		t.Fatalf("expected launcher removed, stat err=%v", err)
	}
}

func TestPluninstallerIgnoresMissingLauncher(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	launcherPath := filepath.Join(t.TempDir(), "usr", "bin", "psiphon")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed config dir: %v", err)
	}

	prevConfigDir := installedPsiphonConfigDir
	prevLauncher := installedPsiphonLauncher
	installedPsiphonConfigDir = configDir
	installedPsiphonLauncher = launcherPath
	defer func() {
		installedPsiphonConfigDir = prevConfigDir
		installedPsiphonLauncher = prevLauncher
	}()

	if code := RunPluninstaller(&bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("unexpected exit code when launcher is absent: %d", code)
	}
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("expected config dir removed, stat err=%v", err)
	}
}

func TestPlinstaller2ReportsDisabledDownload(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := RunPlinstaller2(stdout, stderr); code != ExitDownloadFailed {
		t.Fatalf("expected ExitDownloadFailed=%d, got %d", ExitDownloadFailed, code)
	}
	output := stderr.String()
	if !strings.Contains(output, "Automatic remote download/install is disabled until executable authenticity verification exists.") {
		t.Fatalf("missing primary disabled-download message: %s", output)
	}
	if !strings.Contains(output, "psiphon-mg with an explicit --binary path") {
		t.Fatalf("missing follow-up guidance: %s", output)
	}
}
