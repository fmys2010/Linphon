package mg

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPsiphonReturns127WhenBinaryIsMissing(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	tempDir := t.TempDir()
	installedLinphLauncher = filepath.Join(tempDir, "bin", "linph")
	installedPsiphonLauncher = filepath.Join(tempDir, "bin", "psiphon")
	installedPlinstallerLauncher = filepath.Join(tempDir, "bin", "plinstaller2")
	installedPluninstallerPath = filepath.Join(tempDir, "bin", "pluninstaller")
	legacyInstalledPsiphonPath = filepath.Join(tempDir, "legacy", "psiphon")
	installedPsiphonConfigDir = filepath.Join(tempDir, "etc", "psiphon")
	installedPsiphonBinaryPath = filepath.Join(installedPsiphonConfigDir, "psiphon-tunnel-core-x86_64")
	installedPsiphonConfigPath = filepath.Join(installedPsiphonConfigDir, "psiphon.config")
	currentExecutablePath = func() (string, error) {
		return "", errors.New("not installed")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunPsiphon(&stdout, &stderr); exitCode != 127 {
		t.Fatalf("RunPsiphon() exit = %d, want 127", exitCode)
	}
}

func TestRunInstallAndRunPsiphonViaManifest(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := buildFakeTunnelBinary(t, repoRoot)
	baseConfig := filepath.Join(fixtureRoot, "psiphon.config")
	if err := os.WriteFile(baseConfig, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", baseConfig, err)
	}
	t.Setenv("FAKE_PSIPHON_AUTO_EXIT_DELAY_MS", "1500")
	currentExecutablePath = func() (string, error) {
		return sourceLinph, nil
	}

	var installStdout bytes.Buffer
	var installStderr bytes.Buffer
	installArgs := []string{
		"--binary", sourceBinary,
		"--base-config", baseConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
	}
	if exitCode := runInstall(repoRoot, "linph install", installArgs, &installStdout, &installStderr); exitCode != 0 {
		t.Fatalf("runInstall() exit = %d, stderr = %s", exitCode, installStderr.String())
	}

	layout := buildInstallLayout(binDir, configDir)
	for _, path := range append(layout.allPaths(), layout.ManifestPath) {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("expected installed path %q: %v", path, err)
		}
	}

	manifest, ok, err := readInstallManifest(layout.ManifestPath)
	if err != nil {
		t.Fatalf("readInstallManifest(%q): %v", layout.ManifestPath, err)
	}
	if !ok {
		t.Fatalf("expected install manifest at %q", layout.ManifestPath)
	}
	if manifest.LinphPath != layout.LinphPath {
		t.Fatalf("manifest LinphPath = %q, want %q", manifest.LinphPath, layout.LinphPath)
	}
	if manifest.PsiphonBinaryPath != layout.PsiphonBinaryPath {
		t.Fatalf("manifest PsiphonBinaryPath = %q, want %q", manifest.PsiphonBinaryPath, layout.PsiphonBinaryPath)
	}
	if manifest.PsiphonConfigPath != layout.PsiphonConfigPath {
		t.Fatalf("manifest PsiphonConfigPath = %q, want %q", manifest.PsiphonConfigPath, layout.PsiphonConfigPath)
	}

	currentExecutablePath = func() (string, error) {
		return layout.LinphPath, nil
	}
	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	if exitCode := RunLinphAlias("psiphon", nil, &runStdout, &runStderr); exitCode != 0 {
		t.Fatalf("RunLinphAlias(psiphon) exit = %d, stderr = %s", exitCode, runStderr.String())
	}
}

func TestRunUninstallPreservesConfigByDefault(t *testing.T) {
	binDir, configDir, _ := installFixture(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{"--install-bin-dir", binDir, "--install-config-dir", configDir}
	if exitCode := runUninstall("linph uninstall", args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("runUninstall() exit = %d, stderr = %s", exitCode, stderr.String())
	}

	layout := buildInstallLayout(binDir, configDir)
	for _, path := range append([]string{layout.LinphPath, layout.PsiphonBinaryPath, layout.ManifestPath}, layout.CompatPaths...) {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %q to be removed, err = %v", path, err)
		}
	}
	if _, err := os.Stat(layout.PsiphonConfigPath); err != nil {
		t.Fatalf("expected config to remain at %q: %v", layout.PsiphonConfigPath, err)
	}
	if !strings.Contains(stdout.String(), "preserved") {
		t.Fatalf("uninstall stdout = %q, want preserved message", stdout.String())
	}
}

func TestRunUninstallPurgeRemovesConfigDir(t *testing.T) {
	binDir, configDir, _ := installFixture(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{"--install-bin-dir", binDir, "--install-config-dir", configDir, "--purge"}
	if exitCode := runUninstall("linph uninstall", args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("runUninstall(--purge) exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if _, err := os.Stat(configDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected config dir %q to be removed, err = %v", configDir, err)
	}
	if !strings.Contains(stdout.String(), "purged") {
		t.Fatalf("uninstall stdout = %q, want purge message", stdout.String())
	}
}

func TestRunInstallRejectsUnmanagedExistingFile(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", binDir, err)
	}
	blockingPath := filepath.Join(binDir, "linph")
	if err := os.WriteFile(blockingPath, []byte("unmanaged\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", blockingPath, err)
	}
	sourceLinph := writeExecutableScript(t, filepath.Join(repoRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := writeExecutableScript(t, filepath.Join(repoRoot, "psiphon-tunnel-core-x86_64"), "#!/bin/sh\nexit 0\n")
	baseConfig := filepath.Join(repoRoot, "psiphon.config")
	if err := os.WriteFile(baseConfig, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", baseConfig, err)
	}
	currentExecutablePath = func() (string, error) {
		return sourceLinph, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"--binary", sourceBinary,
		"--base-config", baseConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
	}
	exitCode := runInstall(repoRoot, "linph install", args, &stdout, &stderr)
	if exitCode != ExitValidationFailed {
		t.Fatalf("runInstall() exit = %d, want %d", exitCode, ExitValidationFailed)
	}
	if !strings.Contains(stderr.String(), "use --force") {
		t.Fatalf("runInstall() stderr = %q, want unmanaged path guidance", stderr.String())
	}
}

func installFixture(t *testing.T) (binDir, configDir string, layout installLayout) {
	t.Helper()

	restore := overrideInstallGlobals(t)
	t.Cleanup(restore)

	repoRoot := findRepoRoot(t)
	binDir = filepath.Join(t.TempDir(), "bin")
	configDir = filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := buildFakeTunnelBinary(t, repoRoot)
	baseConfig := filepath.Join(fixtureRoot, "psiphon.config")
	if err := os.WriteFile(baseConfig, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", baseConfig, err)
	}
	t.Setenv("FAKE_PSIPHON_AUTO_EXIT_DELAY_MS", "1500")
	currentExecutablePath = func() (string, error) {
		return sourceLinph, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"--binary", sourceBinary,
		"--base-config", baseConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
	}
	if exitCode := runInstall(repoRoot, "linph install", args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("runInstall() exit = %d, stderr = %s", exitCode, stderr.String())
	}
	layout = buildInstallLayout(binDir, configDir)
	return binDir, configDir, layout
}

func overrideInstallGlobals(t *testing.T) func() {
	t.Helper()

	origConfigDir := installedPsiphonConfigDir
	origBinaryPath := installedPsiphonBinaryPath
	origConfigPath := installedPsiphonConfigPath
	origLinphLauncher := installedLinphLauncher
	origPsiphonLauncher := installedPsiphonLauncher
	origPlinstallerLauncher := installedPlinstallerLauncher
	origPluninstallerPath := installedPluninstallerPath
	origLegacyPsiphonPath := legacyInstalledPsiphonPath
	origCurrentExecutablePath := currentExecutablePath

	return func() {
		installedPsiphonConfigDir = origConfigDir
		installedPsiphonBinaryPath = origBinaryPath
		installedPsiphonConfigPath = origConfigPath
		installedLinphLauncher = origLinphLauncher
		installedPsiphonLauncher = origPsiphonLauncher
		installedPlinstallerLauncher = origPlinstallerLauncher
		installedPluninstallerPath = origPluninstallerPath
		legacyInstalledPsiphonPath = origLegacyPsiphonPath
		currentExecutablePath = origCurrentExecutablePath
	}
}

func writeExecutableScript(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}
