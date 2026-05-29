package mg

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const installedVGStartSurvivalCheck = 2 * time.Second

func (a *app) installedVGProfileAndSpec(layout installLayout) (installedVGProfile, installedVGSpec, error) {
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		return installedVGProfile{}, installedVGSpec{}, err
	}
	if !ok {
		return installedVGProfile{}, installedVGSpec{}, fmt.Errorf("installed profile not found at %s", layout.installedProviderProfilePath())
	}
	profile, err := installedVGProfileFromState(state)
	if err != nil {
		return installedVGProfile{}, installedVGSpec{}, err
	}
	profile, spec, err := validateInstalledVGProfile(profile)
	if err != nil {
		return installedVGProfile{}, installedVGSpec{}, err
	}
	profile.CachePath, err = resolveInstalledVGCachePath(layout, profile)
	if err != nil {
		return installedVGProfile{}, installedVGSpec{}, err
	}
	spec.RuntimeRoot = layout.installedVGRuntimeRoot()
	return profile, spec, nil
}

func resolveInstalledVGCachePath(layout installLayout, profile installedVGProfile) (string, error) {
	cachePath := profile.CachePath
	if cachePath == "" {
		cachePath = layout.installedVGCachePath()
	}
	if profile.AllowUnsafeCachePath {
		return cachePath, nil
	}
	cleanRoot := filepath.Clean(layout.installedVGRuntimeRoot())
	cleanPath := filepath.Clean(cachePath)
	if cleanPath != cleanRoot && !strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("vg cache path must stay under %s; pass --allow-unsafe-cache-path for advanced debugging", cleanRoot)
	}
	return cleanPath, nil
}

func (a *app) commandVGStart(layout installLayout, restart bool) int {
	profile, spec, err := a.installedVGProfileAndSpec(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if restart {
		if code := a.stopInstalledRuntimeRoots([]string{spec.RuntimeRoot}); code != 0 {
			return code
		}
	} else {
		state, stateKind := a.loadState(spec.RuntimeRoot)
		if stateKind == stateRunning && installedVGMatchesState(state, profile, spec) {
			return 0
		}
		if stateKind == stateRunning {
			a.stopActiveState(spec.RuntimeRoot, state)
		} else if stateKind == stateStale {
			a.cleanupStaleState(spec.RuntimeRoot, state)
		}
	}
	if code := a.launchVPNGateOpenVPN(profile, spec); code != 0 {
		return code
	}
	if profile.Refresh {
		return a.clearVGRefresh(layout, profile)
	}
	return 0
}

func (a *app) clearVGRefresh(layout installLayout, profile installedVGProfile) int {
	profile.Refresh = false
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if !ok {
		a.err("installed profile not found at %s", layout.installedProviderProfilePath())
		return ExitUsage
	}
	state.Providers.VG = &profile
	if err := writeInstalledProviderState(layout, state); err != nil {
		a.err("failed to persist provider state: %v", err)
		return ExitUsage
	}
	return 0
}

func (a *app) commandVGStop(layout installLayout) int {
	_, spec, err := a.installedVGProfileAndSpec(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	return a.stopInstalledRuntimeRoots([]string{spec.RuntimeRoot})
}

func (a *app) commandVGPort(layout installLayout) int {
	if _, _, err := a.installedVGProfileAndSpec(layout); err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	fmt.Fprintln(a.stdout, "vg uses system VPN routes; no local HTTP/SOCKS proxy ports")
	return 0
}

func (a *app) commandVGCtry(layout installLayout) int {
	profile, _, err := a.installedVGProfileAndSpec(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	fmt.Fprintln(a.stdout, installedVGRegionsCSV(profile))
	return 0
}

func (a *app) commandVGSwitchPort(layout installLayout) int {
	if _, _, err := a.installedVGProfileAndSpec(layout); err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	a.err("vg uses system VPN routes and does not support switch-port")
	return ExitUsage
}

func (a *app) commandVGSwitchCtry(layout installLayout, args []string) int {
	if len(args) != 1 {
		a.installedUsage(a.stderr)
		return ExitUsage
	}
	profile, spec, err := a.installedVGProfileAndSpec(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	originalProfile := profile
	regions := normalizeInstalledRegions(args[0])
	if len(regions) == 0 {
		a.err("vg requires at least one region")
		return ExitUsage
	}
	profile.Regions = regions
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil || !ok {
		a.err("%v", err)
		return ExitUsage
	}
	running := false
	if _, stateKind := a.loadState(spec.RuntimeRoot); stateKind == stateRunning {
		running = true
	}
	state.Providers.VG = &profile
	if err := writeInstalledProviderState(layout, state); err != nil {
		a.err("failed to persist installed profile: %v", err)
		return ExitUsage
	}
	if running {
		if code := a.commandVGStart(layout, true); code != 0 {
			state.Providers.VG = &originalProfile
			if err := writeInstalledProviderState(layout, state); err != nil {
				a.err("failed to restore previous vg profile: %v", err)
				return ExitUsage
			}
			_ = a.commandVGStart(layout, true)
			return code
		}
	}
	fmt.Fprintln(a.stdout, installedVGRegionsCSV(profile))
	return 0
}

func (a *app) commandVGLog(layout installLayout) int {
	_, spec, err := a.installedVGProfileAndSpec(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	_, stateKind := a.loadState(spec.RuntimeRoot)
	if stateKind != stateRunning {
		a.err("no vg session is running")
		return ExitNotRunning
	}
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	offsets := map[string]int64{}
	tails := map[string]string{}
	ticker := time.NewTicker(installedLogPollInterval)
	defer ticker.Stop()
	for {
		state, stateKind := a.loadState(spec.RuntimeRoot)
		if stateKind != stateRunning {
			return 0
		}
		for _, fileSpec := range []struct {
			stream string
			path   string
		}{
			{stream: "stdout", path: state.StdoutPath},
			{stream: "stderr", path: state.StderrPath},
		} {
			if fileSpec.path == "" {
				continue
			}
			a.drainInstalledLogFile("[vg "+fallbackRegion(state.Region)+" "+fileSpec.stream+"]", fileSpec.path, offsets, tails)
		}
		select {
		case <-signalCh:
			return 0
		case <-ticker.C:
		}
	}
}

func (a *app) launchVPNGateOpenVPN(profile installedVGProfile, spec installedVGSpec) int {
	data, err := readVPNGateCSV(activeInstallLayout(), profile)
	if err != nil {
		a.err("failed to read VPNGate server list: %v", err)
		return ExitUsage
	}
	servers, err := parseVPNGateServers(data)
	if err != nil {
		a.err("failed to parse VPNGate server list: %v", err)
		return ExitUsage
	}
	server, err := selectVPNGateServer(servers, profile.Regions)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	return a.launchOpenVPNServer(profile, spec, server)
}

func (a *app) launchOpenVPNServer(profile installedVGProfile, spec installedVGSpec, server vpngateServer) int {
	if err := validateOpenVPNConfig(server.OpenVPNConfig); err != nil {
		a.err("rejected unsafe OpenVPN config for %s: %v", server.CountryShort, err)
		return ExitValidationFailed
	}
	binaryPath, err := resolveOpenVPNBinaryPath(profile.OpenVPNBinaryPath)
	if err != nil {
		a.err("invalid OpenVPN executable: %v", err)
		return ExitUsage
	}
	runDir, err := makeRunDir(spec.RuntimeRoot, server.CountryShort)
	if err != nil {
		a.err("failed to create vg run directory: %v", err)
		return ExitUsage
	}
	configPath := filepath.Join(runDir, "vpngate.ovpn")
	stdoutPath := filepath.Join(runDir, "stdout.log")
	stderrPath := filepath.Join(runDir, "stderr.log")
	pidPath := filepath.Join(runDir, "pid")
	if err := os.WriteFile(configPath, []byte(server.OpenVPNConfig), 0o600); err != nil {
		a.err("failed to write OpenVPN config: %v", err)
		return ExitUsage
	}
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		a.err("failed to open vg stdout log: %v", err)
		return ExitUsage
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		a.err("failed to open vg stderr log: %v", err)
		return ExitUsage
	}
	defer stderrFile.Close()
	stdinFile, err := os.Open("/dev/null")
	if err != nil {
		a.err("failed to open /dev/null: %v", err)
		return ExitUsage
	}
	defer stdinFile.Close()

	cmd := exec.Command(binaryPath, "--config", configPath)
	cmd.Stdin = stdinFile
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		a.err("failed to launch VPNGate OpenVPN for %s: %v", server.CountryShort, err)
		return ExitUsage
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	if !processSurvives(pid, installedVGStartSurvivalCheck) {
		stopPID(pid, DefaultStopTimeout)
		_ = os.RemoveAll(runDir)
		_ = removeStateFile(spec.RuntimeRoot)
		a.err("VPNGate OpenVPN for %s exited before startup stabilized", server.CountryShort)
		return ExitInstanceFailed
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		a.err("failed to persist vg pid file: %v", err)
		stopPID(pid, DefaultStopTimeout)
		_ = os.RemoveAll(runDir)
		_ = removeStateFile(spec.RuntimeRoot)
		return ExitUsage
	}

	state := activeState{
		Provider:   installedProviderVG,
		Region:     server.CountryShort,
		PID:        pid,
		BinaryPath: binaryPath,
		BaseConfig: profile.APIURL,
		ConfigPath: configPath,
		RunDir:     runDir,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		StderrPath: stderrPath,
		StdoutPath: stdoutPath,
	}
	if err := writeStateFile(spec.RuntimeRoot, state); err != nil {
		a.err("failed to persist vg state: %v", err)
		stopPID(pid, DefaultStopTimeout)
		_ = os.RemoveAll(runDir)
		_ = removeStateFile(spec.RuntimeRoot)
		return ExitUsage
	}
	a.log("vg region %s is running via %s (pid %d)", server.CountryShort, binaryPath, pid)
	return 0
}

func processSurvives(pid int, duration time.Duration) bool {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return processAlive(pid)
}

func installedVGMatchesState(state activeState, profile installedVGProfile, spec installedVGSpec) bool {
	binaryPath, err := resolveOpenVPNBinaryPath(profile.OpenVPNBinaryPath)
	if err != nil {
		return false
	}
	return state.Provider == installedProviderVG &&
		state.Region == spec.Region &&
		state.BinaryPath == binaryPath &&
		state.BaseConfig == profile.APIURL &&
		state.ConfigPath != ""
}

func installedVGRegionsCSV(profile installedVGProfile) string {
	return strings.Join(profile.Regions, ",")
}
