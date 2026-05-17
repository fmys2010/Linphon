package mg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledProfileDerivationResolvesConflictsAndTruncatesRegions(t *testing.T) {
	layout := buildInstallLayout("/tmp/linph-bin", "/tmp/linph-config")
	profile := installedProfile{
		SlotCount:     3,
		HTTPPortBase:  18080,
		SocksPortBase: 18080,
		Regions:       []string{"US", "CA", "JP", "DE"},
	}

	specs, err := deriveInstalledSlotSpecs(layout, profile)
	if err != nil {
		t.Fatalf("deriveInstalledSlotSpecs() error = %v", err)
	}
	if got, want := len(specs), 3; got != want {
		t.Fatalf("deriveInstalledSlotSpecs() len = %d, want %d", got, want)
	}

	wantPorts := [][2]int{{18080, 18081}, {18082, 18083}, {18084, 18085}}
	for index, spec := range specs {
		if spec.Region != profile.Regions[index] {
			t.Fatalf("spec %d region = %s, want %s", index+1, spec.Region, profile.Regions[index])
		}
		if spec.HTTPPort != wantPorts[index][0] || spec.SocksPort != wantPorts[index][1] {
			t.Fatalf("spec %d ports = %d/%d, want %d/%d", index+1, spec.HTTPPort, spec.SocksPort, wantPorts[index][0], wantPorts[index][1])
		}
	}
}

func TestInstalledLifecycleWithFakeBinary(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	repoRoot := findRepoRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc", "psiphon")
	fixtureRoot := t.TempDir()
	sourceLinph := writeExecutableScript(t, filepath.Join(fixtureRoot, "linph-source.sh"), "#!/bin/sh\nexit 0\n")
	sourceBinary := buildFakeTunnelBinary(t, repoRoot)
	baseConfig := filepath.Join(fixtureRoot, "psiphon.config")
	if err := os.WriteFile(baseConfig, []byte("{\n  \"LocalHttpProxyPort\": 8081,\n  \"LocalSocksProxyPort\": 1081,\n  \"EgressRegion\": \"US\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", baseConfig, err)
	}
	currentExecutablePath = func() (string, error) { return sourceLinph, nil }

	var installStdout bytes.Buffer
	var installStderr bytes.Buffer
	installArgs := []string{
		"--binary", sourceBinary,
		"--base-config", baseConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
		"--installed-slot-count", "3",
		"--installed-http-port", "18080",
		"--installed-socks-port", "18080",
		"--installed-regions", "US,CA,JP,DE",
	}
	if exitCode := runInstall(repoRoot, "linph install", installArgs, &installStdout, &installStderr); exitCode != 0 {
		t.Fatalf("runInstall() exit = %d, stderr = %s", exitCode, installStderr.String())
	}

	layout := buildInstallLayout(binDir, configDir)
	currentExecutablePath = func() (string, error) { return layout.LinphPath, nil }

	profile, ok, err := readInstalledProfile(layout.installedProfilePath())
	if err != nil {
		t.Fatalf("readInstalledProfile() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected installed profile at %s", layout.installedProfilePath())
	}
	if profile.SlotCount != 3 {
		t.Fatalf("profile slot count = %d, want 3", profile.SlotCount)
	}
	if got, want := strings.Join(profile.Regions, ","), "US,CA,JP"; got != want {
		t.Fatalf("profile regions = %s, want %s", got, want)
	}

	app := &app{}
	specs, err := deriveInstalledSlotSpecs(layout, profile)
	if err != nil {
		t.Fatalf("deriveInstalledSlotSpecs() error = %v", err)
	}
	initialPIDs := map[string]int{}
	for _, spec := range specs {
		state, stateKind := app.loadState(spec.RuntimeRoot)
		if stateKind != stateRunning {
			t.Fatalf("expected %s to be running, got %s", spec.RuntimeRoot, stateKind)
		}
		if state.Region != spec.Region || state.HTTPPort != spec.HTTPPort || state.SocksPort != spec.SocksPort {
			t.Fatalf("unexpected state for slot %d: %#v", spec.Index+1, state)
		}
		initialPIDs[spec.RuntimeRoot] = state.PID
	}

	var portStdout bytes.Buffer
	var portStderr bytes.Buffer
	if exitCode := RunLinph([]string{"port"}, &portStdout, &portStderr); exitCode != 0 {
		t.Fatalf("RunLinph(port) exit = %d, stderr = %s", exitCode, portStderr.String())
	}
	for _, want := range []string{
		"slot-001 region=US http=18080 socks=18081",
		"slot-002 region=CA http=18082 socks=18083",
		"slot-003 region=JP http=18084 socks=18085",
	} {
		if !strings.Contains(portStdout.String(), want) {
			t.Fatalf("port output %q missing %q", portStdout.String(), want)
		}
	}

	var ctryStdout bytes.Buffer
	var ctryStderr bytes.Buffer
	if exitCode := RunLinph([]string{"ctry"}, &ctryStdout, &ctryStderr); exitCode != 0 {
		t.Fatalf("RunLinph(ctry) exit = %d, stderr = %s", exitCode, ctryStderr.String())
	}
	if got, want := strings.TrimSpace(ctryStdout.String()), "US,CA,JP"; got != want {
		t.Fatalf("ctry output = %q, want %q", got, want)
	}

	var switchPortStdout bytes.Buffer
	var switchPortStderr bytes.Buffer
	if exitCode := RunLinph([]string{"switch-port", "19000", "19000"}, &switchPortStdout, &switchPortStderr); exitCode != 0 {
		t.Fatalf("RunLinph(switch-port) exit = %d, stderr = %s", exitCode, switchPortStderr.String())
	}
	if !strings.Contains(switchPortStdout.String(), "slot-001 region=US http=19000 socks=19001") {
		t.Fatalf("switch-port output = %q", switchPortStdout.String())
	}

	profileAfterPort, ok, err := readInstalledProfile(layout.installedProfilePath())
	if err != nil || !ok {
		t.Fatalf("readInstalledProfile after switch-port: ok=%v err=%v", ok, err)
	}
	if profileAfterPort.HTTPPortBase != 19000 || profileAfterPort.SocksPortBase != 19000 {
		t.Fatalf("unexpected switched ports: %#v", profileAfterPort)
	}

	switchedSpecs, err := deriveInstalledSlotSpecs(layout, profileAfterPort)
	if err != nil {
		t.Fatalf("deriveInstalledSlotSpecs after switch-port: %v", err)
	}
	for _, spec := range switchedSpecs {
		state, stateKind := app.loadState(spec.RuntimeRoot)
		if stateKind != stateRunning {
			t.Fatalf("expected %s to be running after switch-port, got %s", spec.RuntimeRoot, stateKind)
		}
		if state.PID == initialPIDs[spec.RuntimeRoot] {
			t.Fatalf("expected %s pid to change after switch-port", spec.RuntimeRoot)
		}
	}

	var switchCtryStdout bytes.Buffer
	var switchCtryStderr bytes.Buffer
	if exitCode := RunLinph([]string{"switch-ctry", "JP,DE,FR,AT"}, &switchCtryStdout, &switchCtryStderr); exitCode != 0 {
		t.Fatalf("RunLinph(switch-ctry) exit = %d, stderr = %s", exitCode, switchCtryStderr.String())
	}
	if !strings.Contains(switchCtryStdout.String(), "JP,DE,FR") {
		t.Fatalf("switch-ctry output = %q, want updated regions", switchCtryStdout.String())
	}

	profileAfterCtry, ok, err := readInstalledProfile(layout.installedProfilePath())
	if err != nil || !ok {
		t.Fatalf("readInstalledProfile after switch-ctry: ok=%v err=%v", ok, err)
	}
	if got, want := strings.Join(profileAfterCtry.Regions, ","), "JP,DE,FR"; got != want {
		t.Fatalf("unexpected switched regions: %s", got)
	}

	finalSpecs, err := deriveInstalledSlotSpecs(layout, profileAfterCtry)
	if err != nil {
		t.Fatalf("deriveInstalledSlotSpecs after switch-ctry: %v", err)
	}
	for _, spec := range finalSpecs {
		state, stateKind := app.loadState(spec.RuntimeRoot)
		if stateKind != stateRunning {
			t.Fatalf("expected %s to be running after switch-ctry, got %s", spec.RuntimeRoot, stateKind)
		}
		if state.Region != spec.Region {
			t.Fatalf("expected %s region %s, got %s", spec.RuntimeRoot, spec.Region, state.Region)
		}
	}

	var stopStdout bytes.Buffer
	var stopStderr bytes.Buffer
	if exitCode := RunLinph([]string{"stop"}, &stopStdout, &stopStderr); exitCode != 0 {
		t.Fatalf("RunLinph(stop) exit = %d, stderr = %s", exitCode, stopStderr.String())
	}
	for _, spec := range finalSpecs {
		_, stateKind := app.loadState(spec.RuntimeRoot)
		if stateKind != stateNone {
			t.Fatalf("expected %s to be stopped, got %s", spec.RuntimeRoot, stateKind)
		}
	}

	var restartStdout bytes.Buffer
	var restartStderr bytes.Buffer
	if exitCode := RunLinph([]string{"restart"}, &restartStdout, &restartStderr); exitCode != 0 {
		t.Fatalf("RunLinph(restart) exit = %d, stderr = %s", exitCode, restartStderr.String())
	}
	for _, spec := range finalSpecs {
		state, stateKind := app.loadState(spec.RuntimeRoot)
		if stateKind != stateRunning {
			t.Fatalf("expected %s to be running after restart, got %s", spec.RuntimeRoot, stateKind)
		}
		if state.Region != spec.Region {
			t.Fatalf("expected %s region %s after restart, got %s", spec.RuntimeRoot, spec.Region, state.Region)
		}
	}

	if !strings.Contains(installStdout.String(), "slot-001 region=US http=18080 socks=18081") {
		t.Fatalf("install stdout missing configured port pairs: %s", installStdout.String())
	}
}
