package mg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVPNGateCSVSelectsBestRegionServer(t *testing.T) {
	jpConfig := "client\nremote jp.example 1194\n"
	usConfig := "client\nremote us.example 1194\n"
	data := []byte(vpngateFixtureCSV([]vpngateFixtureRow{
		{HostName: "jp-low", CountryLong: "Japan", CountryShort: "JP", Score: 10, Throughput: 100, Config: jpConfig},
		{HostName: "us-best", CountryLong: "United States", CountryShort: "US", Score: 900, Throughput: 9000, Config: usConfig},
		{HostName: "us-slow", CountryLong: "United States", CountryShort: "US", Score: 100, Throughput: 10, Config: "client\nremote slow.example 1194\n"},
	}))

	servers, err := parseVPNGateServers(data)
	if err != nil {
		t.Fatalf("parseVPNGateServers() error = %v", err)
	}
	server, err := selectVPNGateServer(servers, []string{"US"})
	if err != nil {
		t.Fatalf("selectVPNGateServer() error = %v", err)
	}
	if server.HostName != "us-best" {
		t.Fatalf("selected host = %q, want us-best", server.HostName)
	}
	if server.OpenVPNConfig != usConfig {
		t.Fatalf("selected config = %q, want %q", server.OpenVPNConfig, usConfig)
	}
	server, err = selectVPNGateServer(servers, []string{"JP", "US"})
	if err != nil {
		t.Fatalf("selectVPNGateServer(JP,US) error = %v", err)
	}
	if server.HostName != "jp-low" {
		t.Fatalf("selected host with ordered regions = %q, want jp-low", server.HostName)
	}
}

func TestVPNGateRejectsDangerousOpenVPNDirectives(t *testing.T) {
	if err := validateOpenVPNConfig("client\nremote safe.example 1194\n<ca>\ninline ca\n</ca>\n"); err != nil {
		t.Fatalf("validateOpenVPNConfig(safe) error = %v", err)
	}
	for _, config := range []string{
		"client\nscript-security 2\n",
		"client\nup /tmp/hook\n",
		"client\n--up /tmp/hook\n",
		"client\nroute-pre-down /tmp/hook\n",
		"client\nmanagement 127.0.0.1 7505\n",
		"client\nplugin /tmp/plugin.so\n",
		"client\nauth-user-pass\n",
		"client\nlog /tmp/openvpn.log\n",
		"client\nstatus /tmp/openvpn.status\n",
		"client\nwritepid /tmp/openvpn.pid\n",
		"client\nca /tmp/ca.crt\n",
		"client\nkey /tmp/client.key\n",
		"client\ntls-auth /tmp/ta.key\n",
		"client\nclient-connect /tmp/hook\n",
		"client\nlearn-address /tmp/hook\n",
	} {
		if err := validateOpenVPNConfig(config); err == nil {
			t.Fatalf("validateOpenVPNConfig(%q) = nil, want error", config)
		}
	}
}

func TestProviderSetStopsInactiveRuntime(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	fixtureRoot, layout := installTestFixture(t)
	currentExecutablePath = func() (string, error) { return layout.LinphPath, nil }
	openVPN := writeFakeOpenVPN(t, filepath.Join(fixtureRoot, "openvpn"))
	csvPath := writeVPNGateCSVFixture(t, fixtureRoot, []vpngateFixtureRow{{HostName: "us-fast", CountryLong: "United States", CountryShort: "US", Score: 1, Throughput: 1, Config: "client\nremote us.example 1194\n"}})

	var startStdout bytes.Buffer
	var startStderr bytes.Buffer
	if exitCode := RunLinph([]string{"start"}, &startStdout, &startStderr); exitCode != 0 {
		t.Fatalf("RunLinph(start psi) exit = %d, stderr = %s", exitCode, startStderr.String())
	}
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil || !ok {
		t.Fatalf("loadInstalledProviderState() ok=%v err=%v", ok, err)
	}
	psiProfile, err := installedPsiProfileFromState(state)
	if err != nil {
		t.Fatalf("installedPsiProfileFromState() error = %v", err)
	}
	psiSpecs, err := deriveInstalledSlotSpecs(layout, psiProfile)
	if err != nil {
		t.Fatalf("deriveInstalledSlotSpecs() error = %v", err)
	}
	for _, spec := range psiSpecs {
		if _, stateKind := (&app{}).loadState(spec.RuntimeRoot); stateKind != stateRunning {
			t.Fatalf("psi state for %s = %s, want running", spec.RuntimeRoot, stateKind)
		}
	}

	var vgStdout bytes.Buffer
	var vgStderr bytes.Buffer
	if exitCode := RunLinph([]string{"vg", "set", "vpngate", "--regions", "US", "--openvpn", openVPN, "--api-url", "file://" + csvPath, "--allow-local-api-url"}, &vgStdout, &vgStderr); exitCode != 0 {
		t.Fatalf("RunLinph(vg set) exit = %d, stderr = %s", exitCode, vgStderr.String())
	}

	var providerStdout bytes.Buffer
	var providerStderr bytes.Buffer
	if exitCode := RunLinph([]string{"provider", "set", "vg"}, &providerStdout, &providerStderr); exitCode != 0 {
		t.Fatalf("RunLinph(provider set vg) exit = %d, stderr = %s", exitCode, providerStderr.String())
	}
	for _, spec := range psiSpecs {
		if _, stateKind := (&app{}).loadState(spec.RuntimeRoot); stateKind != stateNone {
			t.Fatalf("psi state for %s after provider set vg = %s, want none", spec.RuntimeRoot, stateKind)
		}
	}

	startStdout.Reset()
	startStderr.Reset()
	if exitCode := RunLinph([]string{"start"}, &startStdout, &startStderr); exitCode != 0 {
		t.Fatalf("RunLinph(start vg) exit = %d, stderr = %s", exitCode, startStderr.String())
	}
	if _, stateKind := (&app{}).loadState(layout.installedVGRuntimeRoot()); stateKind != stateRunning {
		t.Fatalf("vg state after start = %s, want running", stateKind)
	}

	providerStdout.Reset()
	providerStderr.Reset()
	if exitCode := RunLinph([]string{"provider", "set", "psiphon"}, &providerStdout, &providerStderr); exitCode != 0 {
		t.Fatalf("RunLinph(provider set psiphon) exit = %d, stderr = %s", exitCode, providerStderr.String())
	}
	if _, stateKind := (&app{}).loadState(layout.installedVGRuntimeRoot()); stateKind != stateNone {
		t.Fatalf("vg state after provider set psiphon = %s, want none", stateKind)
	}
}

func TestInstalledVGProviderLifecycleWithFakeOpenVPN(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	fixtureRoot, layout := installTestFixture(t)

	openVPN := writeFakeOpenVPN(t, filepath.Join(fixtureRoot, "openvpn"))
	csvPath := filepath.Join(fixtureRoot, "vpngate.csv")
	config := "client\nremote us.example 1194\n"
	if err := os.WriteFile(csvPath, []byte(vpngateFixtureCSV([]vpngateFixtureRow{{
		HostName: "us-best", CountryLong: "United States", CountryShort: "US", Score: 900, Throughput: 9000, Config: config,
	}})), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", csvPath, err)
	}

	var vgStdout bytes.Buffer
	var vgStderr bytes.Buffer
	if exitCode := RunLinph([]string{"vg", "set", "vpngate", "--regions", "US", "--openvpn", openVPN, "--api-url", "file://" + csvPath, "--allow-local-api-url", "--cache", filepath.Join(layout.installedVGRuntimeRoot(), "cache.csv"), "--refresh", "--activate"}, &vgStdout, &vgStderr); exitCode != 0 {
		t.Fatalf("RunLinph(vg set) exit = %d, stderr = %s", exitCode, vgStderr.String())
	}
	if !strings.Contains(vgStdout.String(), "vg region=US") {
		t.Fatalf("vg set stdout = %q, want region summary", vgStdout.String())
	}

	var providerStdout bytes.Buffer
	var providerStderr bytes.Buffer
	if exitCode := RunLinph([]string{"provider", "get"}, &providerStdout, &providerStderr); exitCode != 0 {
		t.Fatalf("RunLinph(provider get) exit = %d, stderr = %s", exitCode, providerStderr.String())
	}
	if got := strings.TrimSpace(providerStdout.String()); got != installedProviderVG {
		t.Fatalf("provider get = %q, want %q", got, installedProviderVG)
	}

	var startStdout bytes.Buffer
	var startStderr bytes.Buffer
	if exitCode := RunLinph([]string{"start"}, &startStdout, &startStderr); exitCode != 0 {
		t.Fatalf("RunLinph(start vg) exit = %d, stderr = %s", exitCode, startStderr.String())
	}
	stateAfterStart, ok, err := loadInstalledProviderState(layout)
	if err != nil || !ok {
		t.Fatalf("loadInstalledProviderState() after start ok=%v err=%v", ok, err)
	}
	if stateAfterStart.Providers.VG == nil || stateAfterStart.Providers.VG.Refresh {
		t.Fatalf("expected vg refresh flag to be cleared after start, got %#v", stateAfterStart.Providers.VG)
	}

	app := &app{}
	state, stateKind := app.loadState(layout.installedVGRuntimeRoot())
	if stateKind != stateRunning {
		t.Fatalf("vg state = %s, want running; state=%#v", stateKind, state)
	}
	if state.Provider != installedProviderVG || state.Region != "US" || state.ConfigPath == "" {
		t.Fatalf("unexpected vg state: %#v", state)
	}
	writtenConfig, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", state.ConfigPath, err)
	}
	if string(writtenConfig) != config {
		t.Fatalf("written OpenVPN config = %q, want %q", string(writtenConfig), config)
	}

	var portStdout bytes.Buffer
	var portStderr bytes.Buffer
	if exitCode := RunLinph([]string{"port"}, &portStdout, &portStderr); exitCode != 0 {
		t.Fatalf("RunLinph(port vg) exit = %d, stderr = %s", exitCode, portStderr.String())
	}
	if !strings.Contains(portStdout.String(), "no local HTTP/SOCKS") {
		t.Fatalf("vg port output = %q, want no proxy guidance", portStdout.String())
	}

	var ctryStdout bytes.Buffer
	var ctryStderr bytes.Buffer
	if exitCode := RunLinph([]string{"ctry"}, &ctryStdout, &ctryStderr); exitCode != 0 {
		t.Fatalf("RunLinph(ctry vg) exit = %d, stderr = %s", exitCode, ctryStderr.String())
	}
	if got := strings.TrimSpace(ctryStdout.String()); got != "US" {
		t.Fatalf("vg ctry = %q, want US", got)
	}

	var refreshStdout bytes.Buffer
	var refreshStderr bytes.Buffer
	if exitCode := RunLinph([]string{"vg", "refresh"}, &refreshStdout, &refreshStderr); exitCode != 0 {
		t.Fatalf("RunLinph(vg refresh) exit = %d, stderr = %s", exitCode, refreshStderr.String())
	}
	stateAfterRefresh, ok, err := loadInstalledProviderState(layout)
	if err != nil || !ok || stateAfterRefresh.Providers.VG == nil || stateAfterRefresh.Providers.VG.Refresh {
		t.Fatalf("expected vg refresh flag false after refresh, ok=%v err=%v state=%#v", ok, err, stateAfterRefresh)
	}

	var stopStdout bytes.Buffer
	var stopStderr bytes.Buffer
	if exitCode := RunLinph([]string{"stop"}, &stopStdout, &stopStderr); exitCode != 0 {
		t.Fatalf("RunLinph(stop vg) exit = %d, stderr = %s", exitCode, stopStderr.String())
	}
	if _, stateKind := app.loadState(layout.installedVGRuntimeRoot()); stateKind != stateNone {
		t.Fatalf("vg state after stop = %s, want none", stateKind)
	}
}

func TestProviderSetVGRequiresConfiguredState(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	_, layout := installTestFixture(t)
	currentExecutablePath = func() (string, error) { return layout.LinphPath, nil }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunLinph([]string{"provider", "set", "vg"}, &stdout, &stderr); exitCode == 0 {
		t.Fatalf("RunLinph(provider set vg) exit = %d, want failure", exitCode)
	}
	if !strings.Contains(stderr.String(), "linph vg set") {
		t.Fatalf("provider set vg stderr = %q, want vg set guidance", stderr.String())
	}
}

func TestInstalledPSISwitchesPreserveVGState(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	_, layout := installTestFixture(t)
	currentExecutablePath = func() (string, error) { return layout.LinphPath, nil }

	state, ok, err := loadInstalledProviderState(layout)
	if err != nil || !ok {
		t.Fatalf("loadInstalledProviderState() ok=%v err=%v", ok, err)
	}
	vgProfile := defaultInstalledVGProfile(layout)
	vgProfile.APIURL = "https://example.invalid/vpngate.csv"
	state.Providers.VG = &vgProfile
	if err := writeInstalledProviderState(layout, state); err != nil {
		t.Fatalf("writeInstalledProviderState(): %v", err)
	}

	var switchPortStdout bytes.Buffer
	var switchPortStderr bytes.Buffer
	if exitCode := RunLinph([]string{"switch-port", "19000", "19000"}, &switchPortStdout, &switchPortStderr); exitCode != 0 {
		t.Fatalf("RunLinph(switch-port) exit = %d, stderr = %s", exitCode, switchPortStderr.String())
	}
	state, ok, err = loadInstalledProviderState(layout)
	if err != nil || !ok || state.Providers.VG == nil {
		t.Fatalf("expected vg state after switch-port, ok=%v err=%v state=%#v", ok, err, state)
	}

	var switchCtryStdout bytes.Buffer
	var switchCtryStderr bytes.Buffer
	if exitCode := RunLinph([]string{"switch-ctry", "JP"}, &switchCtryStdout, &switchCtryStderr); exitCode != 0 {
		t.Fatalf("RunLinph(switch-ctry) exit = %d, stderr = %s", exitCode, switchCtryStderr.String())
	}
	state, ok, err = loadInstalledProviderState(layout)
	if err != nil || !ok || state.Providers.VG == nil {
		t.Fatalf("expected vg state after switch-ctry, ok=%v err=%v state=%#v", ok, err, state)
	}
}

func TestInstalledVGStartFailsWhenOpenVPNExitsImmediately(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	fixtureRoot, layout := installTestFixture(t)
	currentExecutablePath = func() (string, error) { return layout.LinphPath, nil }
	openVPN := writeExecutableScript(t, filepath.Join(fixtureRoot, "openvpn-exit"), "#!/bin/sh\nexit 42\n")
	csvPath := writeVPNGateCSVFixture(t, fixtureRoot, []vpngateFixtureRow{{HostName: "us-fast", CountryLong: "United States", CountryShort: "US", Score: 1, Throughput: 1, Config: "client\nremote us.example 1194\n"}})

	var vgStdout bytes.Buffer
	var vgStderr bytes.Buffer
	if exitCode := RunLinph([]string{"vg", "set", "vpngate", "--regions", "US", "--openvpn", openVPN, "--api-url", "file://" + csvPath, "--allow-local-api-url", "--activate"}, &vgStdout, &vgStderr); exitCode != 0 {
		t.Fatalf("RunLinph(vg set) exit = %d, stderr = %s", exitCode, vgStderr.String())
	}

	var startStdout bytes.Buffer
	var startStderr bytes.Buffer
	if exitCode := RunLinph([]string{"start"}, &startStdout, &startStderr); exitCode == 0 {
		t.Fatalf("RunLinph(start) exit = %d, want failure", exitCode)
	}
	if _, stateKind := (&app{}).loadState(layout.installedVGRuntimeRoot()); stateKind == stateRunning {
		t.Fatalf("vg state after failed start = running, want not running")
	}
}

func installTestFixture(t *testing.T) (string, installLayout) {
	t.Helper()
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
	currentExecutablePath = func() (string, error) { return sourceLinph, nil }

	var installStdout bytes.Buffer
	var installStderr bytes.Buffer
	if exitCode := runInstall(repoRoot, "linph install", []string{
		"--binary", sourceBinary,
		"--base-config", baseConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
	}, &installStdout, &installStderr); exitCode != 0 {
		t.Fatalf("runInstall() exit = %d, stderr = %s", exitCode, installStderr.String())
	}

	layout := buildInstallLayout(binDir, configDir)
	installedLinphLauncher = layout.LinphPath
	installedPsiphonLauncher = filepath.Join(binDir, "psiphon")
	installedPlinstallerLauncher = filepath.Join(binDir, "plinstaller2")
	installedPluninstallerPath = filepath.Join(binDir, "pluninstaller")
	legacyInstalledPsiphonPath = filepath.Join(t.TempDir(), "legacy", "psiphon")
	installedPsiphonConfigDir = configDir
	installedPsiphonBinaryPath = layout.PsiphonBinaryPath
	installedPsiphonConfigPath = layout.PsiphonConfigPath
	return fixtureRoot, layout
}

func writeVPNGateCSVFixture(t *testing.T, dir string, rows []vpngateFixtureRow) string {
	t.Helper()
	csvPath := filepath.Join(dir, "vpngate.csv")
	if err := os.WriteFile(csvPath, []byte(vpngateFixtureCSV(rows)), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", csvPath, err)
	}
	return csvPath
}

type vpngateFixtureRow struct {
	HostName     string
	CountryLong  string
	CountryShort string
	Score        int
	Throughput   int
	Config       string
}

func vpngateFixtureCSV(rows []vpngateFixtureRow) string {
	var builder strings.Builder
	builder.WriteString("*vpn_servers\n")
	builder.WriteString("HostName,IP,Score,Throughput,CountryLong,CountryShort,OpenVPN_ConfigData_Base64\n")
	for _, row := range rows {
		builder.WriteString(fmt.Sprintf("%s,203.0.113.1,%d,%d,%s,%s,%s\n", row.HostName, row.Score, row.Throughput, row.CountryLong, row.CountryShort, base64.StdEncoding.EncodeToString([]byte(row.Config))))
	}
	builder.WriteString("*\n")
	return builder.String()
}

func writeFakeOpenVPN(t *testing.T, path string) string {
	t.Helper()
	return writeExecutableScript(t, path, `#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    --config)
      CONFIG="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
echo "fake-openvpn config=$CONFIG"
trap 'exit 0' TERM INT
while :; do sleep 1; done
`)
}
