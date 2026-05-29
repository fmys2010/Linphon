package mg

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
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

func TestInstallScriptHelpDocumentsBootstrapDefault(t *testing.T) {
	repoRoot := findRepoRoot(t)
	cmd := exec.Command("bash", filepath.Join(repoRoot, "install.sh"), "--help")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install.sh --help: %v (stderr=%s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Default behavior bootstraps only the linph command") {
		t.Fatalf("install.sh --help stdout = %q, want bootstrap guidance", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--legacy-full-install") {
		t.Fatalf("install.sh --help stdout = %q, want legacy fallback guidance", stdout.String())
	}
	if !strings.Contains(stdout.String(), "golang-go on Debian/Ubuntu") {
		t.Fatalf("install.sh --help stdout = %q, want Go dependency guidance", stdout.String())
	}
}

func TestInstallScriptLegacyHelpRoutesToInstallHelp(t *testing.T) {
	repoRoot := findRepoRoot(t)
	cmd := exec.Command("bash", filepath.Join(repoRoot, "install.sh"), "--legacy-full-install", "--help")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install.sh --legacy-full-install --help: %v (stderr=%s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--start") {
		t.Fatalf("legacy install help stdout = %q, want --start documentation", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("legacy install help stdout = %q, want install usage", stdout.String())
	}
}

func TestInstallScriptBootstrapsLinphByDefault(t *testing.T) {
	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	cmd := exec.Command("bash", filepath.Join(repoRoot, "install.sh"), "--install-bin-dir", binDir)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install.sh bootstrap: %v (stderr=%s)", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(binDir, "linph")); err != nil {
		t.Fatalf("expected bootstrap linph at %s: %v", filepath.Join(binDir, "linph"), err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "linph-install-manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no install manifest in bootstrap mode, err=%v", err)
	}
	if !strings.Contains(stdout.String(), "next step: linph install") {
		t.Fatalf("install.sh bootstrap stdout = %q, want next-step guidance", stdout.String())
	}
}

func TestInstallScriptBootstrapsFromProcessSubstitution(t *testing.T) {
	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	archivePath := filepath.Join(t.TempDir(), "linphon.tar.gz")
	archiveCmd := exec.Command("git", "archive", "--format=tar.gz", "--prefix", "Linphon-main/", "--output", archivePath, "HEAD")
	archiveCmd.Dir = repoRoot
	var archiveStderr bytes.Buffer
	archiveCmd.Stderr = &archiveStderr
	if err := archiveCmd.Run(); err != nil {
		t.Fatalf("git archive: %v (stderr=%s)", err, archiveStderr.String())
	}

	cmd := exec.Command("bash", "-c", "bash <(cat install.sh) --install-bin-dir \"$1\"", "bash", binDir)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "LINPHON_BOOTSTRAP_ARCHIVE_URL=file://"+archivePath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install.sh process substitution bootstrap: %v (stdout=%s stderr=%s)", err, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(binDir, "linph")); err != nil {
		t.Fatalf("expected process substitution bootstrap linph at %s: %v", filepath.Join(binDir, "linph"), err)
	}
	if !strings.Contains(stderr.String(), "fetching Linphon source archive") {
		t.Fatalf("process substitution stderr = %q, want source archive fetch guidance", stderr.String())
	}
}

func TestRunInstallStartPathStartsInstalledSlots(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := buildFakeTunnelBinary(t, repoRoot)
	t.Setenv("FAKE_PSIPHON_AUTO_EXIT_DELAY_MS", "1500")
	baseConfig := filepath.Join(fixtureRoot, "psiphon.config")
	if err := os.WriteFile(baseConfig, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", baseConfig, err)
	}
	currentExecutablePath = func() (string, error) { return sourceLinph, nil }

	layout := buildInstallLayout(binDir, configDir)
	installedLinphLauncher = layout.LinphPath
	installedPsiphonLauncher = filepath.Join(binDir, "psiphon")
	installedPlinstallerLauncher = filepath.Join(binDir, "plinstaller2")
	installedPluninstallerPath = filepath.Join(binDir, "pluninstaller")
	installedPsiphonConfigDir = configDir
	installedPsiphonBinaryPath = layout.PsiphonBinaryPath
	installedPsiphonConfigPath = layout.PsiphonConfigPath

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	installArgs := []string{"--binary", sourceBinary, "--base-config", baseConfig, "--install-bin-dir", binDir, "--install-config-dir", configDir, "--installed-slot-count", "1", "--installed-http-port", "18080", "--installed-socks-port", "18080", "--installed-regions", "US", "--start"}
	if exitCode := runInstall(repoRoot, "linph install", installArgs, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("runInstall(--start) exit = %d, stderr=%s", exitCode, stderr.String())
	}

	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		t.Fatalf("loadInstalledProviderState() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected installed provider state at %s", layout.installedProviderProfilePath())
	}
	profile, err := installedPsiProfileFromState(state)
	if err != nil {
		t.Fatalf("installedPsiProfileFromState() error = %v", err)
	}
	if got, want := profile.SlotCount, 1; got != want {
		t.Fatalf("profile slot count = %d, want %d", got, want)
	}
	app := &app{}
	specs, err := deriveInstalledSlotSpecs(layout, profile)
	if err != nil {
		t.Fatalf("deriveInstalledSlotSpecs() error = %v", err)
	}
	for _, spec := range specs {
		loadedState, stateKind := app.loadState(spec.RuntimeRoot)
		if stateKind != stateRunning {
			t.Fatalf("expected %s to be running after install --start, got %s", spec.RuntimeRoot, stateKind)
		}
		if loadedState.Region != "US" || loadedState.HTTPPort != 18080 || loadedState.SocksPort != 18081 {
			t.Fatalf("unexpected running slot state: %#v", loadedState)
		}
	}

	var stopStdout bytes.Buffer
	var stopStderr bytes.Buffer
	if exitCode := RunLinph([]string{"stop"}, &stopStdout, &stopStderr); exitCode != 0 {
		t.Fatalf("RunLinph(stop) exit = %d, stderr = %s", exitCode, stopStderr.String())
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

func TestRunInstallRejectsUnknownInstalledRegion(t *testing.T) {
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
		"--installed-slot-count", "2",
		"--installed-http-port", "18080",
		"--installed-socks-port", "10880",
		"--installed-regions", "US,ZZ",
	}
	if exitCode := runInstall(repoRoot, "linph install", args, &stdout, &stderr); exitCode != ExitUsage {
		t.Fatalf("runInstall() exit = %d, want %d (stderr=%s)", exitCode, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown region code: ZZ") {
		t.Fatalf("runInstall() stderr = %q, want unknown region guidance", stderr.String())
	}
}

func TestRunInstallRejectsNonRegularBaseConfig(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := buildFakeTunnelBinary(t, repoRoot)
	baseConfigDir := filepath.Join(fixtureRoot, "psiphon.config")
	if err := os.MkdirAll(baseConfigDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", baseConfigDir, err)
	}
	currentExecutablePath = func() (string, error) {
		return sourceLinph, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"--binary", sourceBinary,
		"--base-config", baseConfigDir,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
	}
	if exitCode := runInstall(repoRoot, "linph install", args, &stdout, &stderr); exitCode != ExitUsage {
		t.Fatalf("runInstall() exit = %d, want %d (stderr=%s)", exitCode, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "base config must be a regular file") {
		t.Fatalf("runInstall() stderr = %q, want regular-file guidance", stderr.String())
	}
}

func TestRunInstallRejectsSymlinkedBinary(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	realBinary := buildFakeTunnelBinary(t, repoRoot)
	symlinkBinary := filepath.Join(fixtureRoot, "psiphon-tunnel-core-x86_64")
	if err := os.Symlink(realBinary, symlinkBinary); err != nil {
		t.Fatalf("Symlink(%q, %q): %v", realBinary, symlinkBinary, err)
	}
	baseConfig := filepath.Join(fixtureRoot, "psiphon.config")
	if err := os.WriteFile(baseConfig, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", baseConfig, err)
	}
	currentExecutablePath = func() (string, error) {
		return sourceLinph, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"--binary", symlinkBinary,
		"--base-config", baseConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
	}
	if exitCode := runInstall(repoRoot, "linph install", args, &stdout, &stderr); exitCode != ExitUsage {
		t.Fatalf("runInstall() exit = %d, want %d (stderr=%s)", exitCode, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "binary must be a regular file and not a symlink") {
		t.Fatalf("runInstall() stderr = %q, want symlink guidance", stderr.String())
	}
}

func TestRunInstallRejectsSymlinkedBaseConfig(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := buildFakeTunnelBinary(t, repoRoot)
	realConfig := filepath.Join(fixtureRoot, "real-psiphon.config")
	if err := os.WriteFile(realConfig, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", realConfig, err)
	}
	symlinkConfig := filepath.Join(fixtureRoot, "psiphon.config")
	if err := os.Symlink(realConfig, symlinkConfig); err != nil {
		t.Fatalf("Symlink(%q, %q): %v", realConfig, symlinkConfig, err)
	}
	currentExecutablePath = func() (string, error) {
		return sourceLinph, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"--binary", sourceBinary,
		"--base-config", symlinkConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
	}
	if exitCode := runInstall(repoRoot, "linph install", args, &stdout, &stderr); exitCode != ExitUsage {
		t.Fatalf("runInstall() exit = %d, want %d (stderr=%s)", exitCode, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "base config must be a regular file and not a symlink") {
		t.Fatalf("runInstall() stderr = %q, want symlink guidance", stderr.String())
	}
}

func TestRunInstallPreservesSourceBinaryPermissions(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := buildFakeTunnelBinary(t, repoRoot)
	if err := os.Chmod(sourceBinary, 0o644); err != nil {
		t.Fatalf("Chmod(%q): %v", sourceBinary, err)
	}
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
	info, err := os.Stat(sourceBinary)
	if err != nil {
		t.Fatalf("Stat(%q): %v", sourceBinary, err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("source binary mode = %o, want 644", got)
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
	origInstalledProcMeminfoPath := installedProcMeminfoPath
	origInstalledCgroupLimitPaths := append([]string(nil), installedCgroupLimitPaths...)
	origInstalledReadFile := installedReadFile

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
		installedProcMeminfoPath = origInstalledProcMeminfoPath
		installedCgroupLimitPaths = append([]string(nil), origInstalledCgroupLimitPaths...)
		installedReadFile = origInstalledReadFile
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
