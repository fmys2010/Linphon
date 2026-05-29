package mg

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const installedLogPollInterval = 250 * time.Millisecond

func (a *app) runInstalledControlCommand(command string, args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h", "help":
			a.installedUsage(a.stdout)
			return 0
		}
	}

	layout := activeInstallLayout()
	switch command {
	case "start":
		if len(args) != 0 {
			a.installedUsage(a.stderr)
			return ExitUsage
		}
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledStart(layout)
		})
	case "restart":
		if len(args) != 0 {
			a.installedUsage(a.stderr)
			return ExitUsage
		}
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledRestart(layout)
		})
	case "stop":
		if len(args) != 0 {
			a.installedUsage(a.stderr)
			return ExitUsage
		}
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledStop(layout)
		})
	case "port":
		if len(args) != 0 {
			a.installedUsage(a.stderr)
			return ExitUsage
		}
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledPort(layout)
		})
	case "ctry":
		if len(args) != 0 {
			a.installedUsage(a.stderr)
			return ExitUsage
		}
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledCtry(layout)
		})
	case "log":
		if len(args) != 0 {
			a.installedUsage(a.stderr)
			return ExitUsage
		}
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledLog(layout)
		})
	case "switch-port":
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledSwitchPort(layout, args)
		})
	case "switch-ctry":
		return a.withInstalledLock(layout, func() int {
			return a.commandInstalledSwitchCtry(layout, args)
		})
	default:
		a.installedUsage(a.stderr)
		return ExitUsage
	}
}

func (a *app) withInstalledLock(layout installLayout, fn func() int) int {
	release, code := a.acquireLock(layout.installedRuntimeRoot())
	if code != 0 {
		return code
	}
	defer release()
	return fn()
}

func (a *app) installedUsage(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  linph start
  linph restart
  linph stop
  linph port
  linph ctry
  linph log
  linph switch-port HTTP_PORT SOCKS_PORT
  linph switch-ctry REGION1,REGION2,...

Commands:
  start         Start or reconcile all installed slots.
  restart       Restart all installed slots.
  stop          Stop all installed slots.
  port          Print configured slot port pairs.
  ctry          Print configured regions.
  log           Follow installed logs until Ctrl-C.
  switch-port   Update starting ports and restart if running.
  switch-ctry   Update regions and restart if running.
`)
}

func (a *app) commandInstalledStart(layout installLayout) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		if code := a.stopInstalledSlots(layout); code != 0 {
			return code
		}
		return a.commandVGStart(layout, false)
	}
	profile, specs, err := a.installedProfileAndSpecs(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if code := a.stopInstalledRuntimeRoots([]string{layout.installedVGRuntimeRoot()}); code != 0 {
		return code
	}
	return a.syncInstalledSlots(layout, profile, specs, false)
}

func (a *app) commandInstalledRestart(layout installLayout) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		if code := a.stopInstalledSlots(layout); code != 0 {
			return code
		}
		return a.commandVGStart(layout, true)
	}
	profile, specs, err := a.installedProfileAndSpecs(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if code := a.stopInstalledRuntimeRoots([]string{layout.installedVGRuntimeRoot()}); code != 0 {
		return code
	}
	return a.syncInstalledSlots(layout, profile, specs, true)
}

func (a *app) commandInstalledStop(layout installLayout) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		if _, _, err := a.installedVGProfileAndSpec(layout); err != nil {
			a.err("%v", err)
			return ExitUsage
		}
		return a.stopAllInstalledRuntimes(layout)
	}
	if _, _, err := a.installedProfileAndSpecs(layout); err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	return a.stopAllInstalledRuntimes(layout)
}

func (a *app) commandInstalledPort(layout installLayout) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		return a.commandVGPort(layout)
	}
	_, specs, err := a.installedProfileAndSpecs(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	fmt.Fprintln(a.stdout, installedPortsCSV(specs))
	return 0
}

func (a *app) commandInstalledCtry(layout installLayout) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		return a.commandVGCtry(layout)
	}
	_, specs, err := a.installedProfileAndSpecs(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	fmt.Fprintln(a.stdout, installedRegionsCSV(specs))
	return 0
}

func (a *app) commandInstalledLog(layout installLayout) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		return a.commandVGLog(layout)
	}
	_, specs, err := a.installedProfileAndSpecs(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if !a.anyInstalledSlotRunning(specs) {
		a.err("no installed slots are running")
		return ExitNotRunning
	}
	return a.followInstalledLogs(layout)
}

func (a *app) commandInstalledSwitchPort(layout installLayout, args []string) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		return a.commandVGSwitchPort(layout)
	}
	if len(args) != 2 {
		a.installedUsage(a.stderr)
		return ExitUsage
	}
	httpPort, err := parseInstalledPort(args[0])
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	socksPort, err := parseInstalledPort(args[1])
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}

	profile, specs, err := a.installedProfileAndSpecs(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	profile.HTTPPortBase = httpPort
	profile.SocksPortBase = socksPort
	updatedSpecs, err := deriveInstalledSlotSpecs(layout, profile)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if err := updateInstalledPsiProviderState(layout, profile); err != nil {
		a.err("failed to persist installed profile: %v", err)
		return ExitUsage
	}
	if a.anyInstalledSlotRunning(specs) {
		if code := a.restartInstalledSlots(layout, updatedSpecs); code != 0 {
			return code
		}
	}
	fmt.Fprintln(a.stdout, installedPortsCSV(updatedSpecs))
	return 0
}

func (a *app) commandInstalledSwitchCtry(layout installLayout, args []string) int {
	provider, err := activeProviderName(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if provider == installedProviderVG {
		return a.commandVGSwitchCtry(layout, args)
	}
	if len(args) != 1 {
		a.installedUsage(a.stderr)
		return ExitUsage
	}
	profile, specs, err := a.installedProfileAndSpecs(layout)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	regions := normalizeInstalledRegions(args[0])
	if len(regions) < profile.SlotCount {
		a.err("need at least %d region(s) for %d slot(s)", profile.SlotCount, profile.SlotCount)
		return ExitUsage
	}
	if err := validateInstalledRegions(a.repoRoot, regions[:profile.SlotCount]); err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	profile.Regions = append([]string(nil), regions[:profile.SlotCount]...)
	updatedSpecs, err := deriveInstalledSlotSpecs(layout, profile)
	if err != nil {
		a.err("%v", err)
		return ExitUsage
	}
	if err := updateInstalledPsiProviderState(layout, profile); err != nil {
		a.err("failed to persist installed profile: %v", err)
		return ExitUsage
	}
	if a.anyInstalledSlotRunning(specs) {
		if code := a.restartInstalledSlots(layout, updatedSpecs); code != 0 {
			return code
		}
	}
	fmt.Fprintln(a.stdout, installedRegionsCSV(updatedSpecs))
	return 0
}

func updateInstalledPsiProviderState(layout installLayout, profile installedProfile) error {
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		return err
	}
	if !ok {
		state = installedProviderStateFromPsi(profile)
	} else {
		profile.Version = installedProfileVersion
		state.Providers.Psi = &profile
	}
	return writeInstalledProviderState(layout, state)
}

func (a *app) stopAllInstalledRuntimes(layout installLayout) int {
	runtimeRoots := append([]string{layout.installedVGRuntimeRoot()}, a.installedSlotRuntimeRoots(layout)...)
	return a.stopInstalledRuntimeRoots(runtimeRoots)
}

func (a *app) stopInactiveInstalledRuntimes(layout installLayout, activeProvider string) int {
	if activeProvider == installedProviderVG {
		return a.stopInstalledSlots(layout)
	}
	return a.stopInstalledRuntimeRoots([]string{layout.installedVGRuntimeRoot()})
}

func activeProviderName(layout installLayout) (string, error) {
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("installed profile not found at %s", layout.installedProviderProfilePath())
	}
	return state.ActiveProvider, nil
}

func (a *app) installedProfileAndSpecs(layout installLayout) (installedProfile, []installedSlotSpec, error) {
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		return installedProfile{}, nil, err
	}
	if !ok {
		return installedProfile{}, nil, fmt.Errorf("installed profile not found at %s", layout.installedProviderProfilePath())
	}
	profile, err := installedPsiProfileFromState(state)
	if err != nil {
		return installedProfile{}, nil, err
	}
	return a.installedProfileAndSpecsFromProfile(layout, profile)
}

func (a *app) installedProfileAndSpecsFromProfile(layout installLayout, profile installedProfile) (installedProfile, []installedSlotSpec, error) {
	if profile.SlotCount < 1 || profile.SlotCount > installedSlotCountHardLimit {
		return installedProfile{}, nil, fmt.Errorf("slot count must be between 1 and %d", installedSlotCountHardLimit)
	}
	if len(profile.Regions) < profile.SlotCount {
		return installedProfile{}, nil, fmt.Errorf("need at least %d region(s) for %d slot(s)", profile.SlotCount, profile.SlotCount)
	}
	if err := validateInstalledRegions(a.repoRoot, profile.Regions[:profile.SlotCount]); err != nil {
		return installedProfile{}, nil, err
	}
	specs, err := deriveInstalledSlotSpecs(layout, profile)
	if err != nil {
		return installedProfile{}, nil, err
	}
	return profile, specs, nil
}

func (a *app) syncInstalledSlots(layout installLayout, profile installedProfile, specs []installedSlotSpec, restart bool) int {
	if code := a.stopInstalledOrphanSlots(layout, specs); code != 0 {
		return code
	}
	if restart {
		if code := a.stopInstalledSpecs(specs); code != 0 {
			return code
		}
	}

	started := make([]installedSlotSpec, 0, len(specs))
	for _, spec := range specs {
		state, stateKind := a.loadState(spec.RuntimeRoot)
		if !restart && stateKind == stateRunning && installedSlotMatchesState(state, layout, spec) {
			continue
		}
		if stateKind == stateRunning {
			a.stopActiveState(spec.RuntimeRoot, state)
		} else if stateKind == stateStale {
			a.cleanupStaleState(spec.RuntimeRoot, state)
		}

		code := a.launchRegion(spec.Region, layout.PsiphonBinaryPath, options{
			RuntimeRoot:        spec.RuntimeRoot,
			BaseConfig:         layout.PsiphonConfigPath,
			HTTPPort:           spec.HTTPPort,
			SocksPort:          spec.SocksPort,
			ReadyTimeoutSecond: DefaultReadyTimeout,
		})
		if code != 0 {
			_ = a.stopInstalledSpecs(started)
			return code
		}
		started = append(started, spec)
	}

	return 0
}

func (a *app) restartInstalledSlots(layout installLayout, specs []installedSlotSpec) int {
	if code := a.stopInstalledOrphanSlots(layout, specs); code != 0 {
		return code
	}
	for _, spec := range specs {
		state, stateKind := a.loadState(spec.RuntimeRoot)
		if stateKind == stateRunning {
			a.stopActiveState(spec.RuntimeRoot, state)
		} else if stateKind == stateStale {
			a.cleanupStaleState(spec.RuntimeRoot, state)
		}
	}

	started := make([]installedSlotSpec, 0, len(specs))
	for _, spec := range specs {
		code := a.launchRegion(spec.Region, layout.PsiphonBinaryPath, options{
			RuntimeRoot:        spec.RuntimeRoot,
			BaseConfig:         layout.PsiphonConfigPath,
			HTTPPort:           spec.HTTPPort,
			SocksPort:          spec.SocksPort,
			ReadyTimeoutSecond: DefaultReadyTimeout,
		})
		if code != 0 {
			_ = a.stopInstalledSpecs(started)
			return code
		}
		started = append(started, spec)
	}

	return 0
}

func (a *app) stopInstalledSlots(layout installLayout) int {
	return a.stopInstalledRuntimeRoots(a.installedSlotRuntimeRoots(layout))
}

func (a *app) stopInstalledSpecs(specs []installedSlotSpec) int {
	runtimeRoots := make([]string, 0, len(specs))
	for _, spec := range specs {
		runtimeRoots = append(runtimeRoots, spec.RuntimeRoot)
	}
	return a.stopInstalledRuntimeRoots(runtimeRoots)
}

func (a *app) stopInstalledRuntimeRoots(runtimeRoots []string) int {
	for _, runtimeRoot := range runtimeRoots {
		state, stateKind := a.loadState(runtimeRoot)
		switch stateKind {
		case stateRunning:
			a.stopActiveState(runtimeRoot, state)
		case stateStale:
			a.cleanupStaleState(runtimeRoot, state)
		default:
			_ = removeStateFile(runtimeRoot)
		}
	}
	return 0
}

func (a *app) stopInstalledOrphanSlots(layout installLayout, specs []installedSlotSpec) int {
	expected := map[string]struct{}{}
	for _, spec := range specs {
		expected[filepath.Clean(spec.RuntimeRoot)] = struct{}{}
	}
	orphans := []string{}
	for _, runtimeRoot := range a.installedSlotRuntimeRoots(layout) {
		if _, ok := expected[filepath.Clean(runtimeRoot)]; ok {
			continue
		}
		orphans = append(orphans, runtimeRoot)
	}
	return a.stopInstalledRuntimeRoots(orphans)
}

func (a *app) installedSlotRuntimeRoots(layout installLayout) []string {
	entries, err := os.ReadDir(layout.installedSlotsRoot())
	if err != nil {
		return nil
	}
	runtimeRoots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runtimeRoots = append(runtimeRoots, filepath.Join(layout.installedSlotsRoot(), entry.Name()))
	}
	return runtimeRoots
}

func (a *app) anyInstalledSlotRunning(specs []installedSlotSpec) bool {
	for _, spec := range specs {
		if _, stateKind := a.loadState(spec.RuntimeRoot); stateKind == stateRunning {
			return true
		}
	}
	return false
}

func installedSlotMatchesState(state activeState, layout installLayout, spec installedSlotSpec) bool {
	return state.Region == spec.Region &&
		state.HTTPPort == spec.HTTPPort &&
		state.SocksPort == spec.SocksPort &&
		state.BinaryPath == layout.PsiphonBinaryPath &&
		state.BaseConfig == layout.PsiphonConfigPath
}

func (a *app) followInstalledLogs(layout installLayout) int {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	offsets := map[string]int64{}
	tails := map[string]string{}
	ticker := time.NewTicker(installedLogPollInterval)
	defer ticker.Stop()

	for {
		_, specs, err := a.installedProfileAndSpecs(layout)
		if err != nil {
			a.err("%v", err)
			return ExitUsage
		}
		a.drainInstalledLogFiles(specs, offsets, tails)

		select {
		case <-signalCh:
			return 0
		case <-ticker.C:
		}
	}
}

func (a *app) drainInstalledLogFiles(specs []installedSlotSpec, offsets map[string]int64, tails map[string]string) {
	for _, spec := range specs {
		state, stateKind := a.loadState(spec.RuntimeRoot)
		if stateKind != stateRunning {
			continue
		}
		for _, fileSpec := range []struct {
			stream string
			path   string
		}{
			{stream: "notices", path: state.NoticesPath},
			{stream: "stdout", path: state.StdoutPath},
			{stream: "stderr", path: state.StderrPath},
		} {
			if fileSpec.path == "" {
				continue
			}
			prefix := installedSlotPrefix(spec, fileSpec.stream)
			a.drainInstalledLogFile(prefix, fileSpec.path, offsets, tails)
		}
	}
}

func (a *app) drainInstalledLogFile(prefix, path string, offsets map[string]int64, tails map[string]string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	currentOffset := offsets[path]
	if currentOffset < 0 || currentOffset > int64(len(content)) {
		currentOffset = 0
		tails[path] = ""
	}
	if currentOffset == int64(len(content)) {
		return
	}
	chunk := content[currentOffset:]
	offsets[path] = int64(len(content))
	text := tails[path] + string(chunk)
	lines := strings.Split(text, "\n")
	complete := lines
	if !strings.HasSuffix(text, "\n") {
		complete = lines[:len(lines)-1]
		tails[path] = lines[len(lines)-1]
	} else {
		tails[path] = ""
	}
	for _, line := range complete {
		if line == "" {
			continue
		}
		fmt.Fprintf(a.stdout, "%s %s\n", prefix, line)
	}
}
