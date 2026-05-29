package mg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const installedManifestFilename = "linph-install-manifest.json"

const (
	installedRestartScheduleDisabled = 0
	installedRestartScheduleMaxHours = 168
	installedRestartServiceName      = "linph-periodic-restart.service"
	installedRestartTimerName        = "linph-periodic-restart.timer"
)

type installOptions struct {
	BinaryPath          string
	BaseConfig          string
	InstallBinDir       string
	InstallConfigDir    string
	InstalledSlotCount  int
	InstalledHTTPPort   int
	InstalledSocksPort  int
	InstalledRegionsCSV string
	InstalledProfileSet bool
	ForceUnlockSlotCap  bool
	Force               bool
	StartInstalledSlots bool
	RestartEveryHours   int
}

type uninstallOptions struct {
	InstallBinDir       string
	InstallConfigDir    string
	InstallBinDirSet    bool
	InstallConfigDirSet bool
	Purge               bool
}

type installLayout struct {
	BinDir            string
	ConfigDir         string
	LinphPath         string
	CompatPaths       []string
	PsiphonBinaryPath string
	PsiphonConfigPath string
	ManifestPath      string
}

type installManifest struct {
	Version           int      `json:"version"`
	BinDir            string   `json:"bin_dir"`
	ConfigDir         string   `json:"config_dir"`
	LinphPath         string   `json:"linph_path"`
	CompatPaths       []string `json:"compat_paths"`
	PsiphonBinaryPath string   `json:"psiphon_binary_path"`
	PsiphonConfigPath string   `json:"psiphon_config_path"`
	ManifestPath      string   `json:"manifest_path"`
	RestartEveryHours int      `json:"restart_every_hours,omitempty"`
}

func runInstall(repoRoot, usageName string, args []string, stdout, stderr io.Writer) int {
	opt := installOptions{
		BaseConfig:       filepath.Join(repoRoot, "psiphon.config"),
		InstallBinDir:    filepath.Dir(installedLinphLauncher),
		InstallConfigDir: installedPsiphonConfigDir,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--binary":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--binary requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.BinaryPath = args[i+1]
			i++
		case "--base-config":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--base-config requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.BaseConfig = args[i+1]
			i++
		case "--install-bin-dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--install-bin-dir requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstallBinDir = args[i+1]
			i++
		case "--install-config-dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--install-config-dir requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstallConfigDir = args[i+1]
			i++
		case "--force":
			opt.Force = true
		case "--start":
			opt.StartInstalledSlots = true
		case "--installed-slot-count":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--installed-slot-count requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			count, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(stderr, "--installed-slot-count must be an integer\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstalledSlotCount = count
			opt.InstalledProfileSet = true
			i++
		case "--installed-http-port":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--installed-http-port requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(stderr, "--installed-http-port must be an integer\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstalledHTTPPort = port
			opt.InstalledProfileSet = true
			i++
		case "--installed-socks-port":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--installed-socks-port requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(stderr, "--installed-socks-port must be an integer\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstalledSocksPort = port
			opt.InstalledProfileSet = true
			i++
		case "--installed-regions":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--installed-regions requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstalledRegionsCSV = args[i+1]
			opt.InstalledProfileSet = true
			i++
		case "--restart-every-hours":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--restart-every-hours requires a value\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			hours, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(stderr, "--restart-every-hours must be an integer\n")
				installUsage(stderr, usageName)
				return ExitUsage
			}
			opt.RestartEveryHours = hours
			i++
		case "--fk":
			opt.ForceUnlockSlotCap = true
		case "--help", "-h":
			installUsage(stdout, usageName)
			return 0
		default:
			fmt.Fprintf(stderr, "unknown install option: %s\n", args[i])
			installUsage(stderr, usageName)
			return ExitUsage
		}
	}

	layout := buildInstallLayout(opt.InstallBinDir, opt.InstallConfigDir)
	managedPaths, err := managedInstallPaths(layout.BinDir)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read existing install manifest: %v\n", err)
		return ExitValidationFailed
	}

	runtimeRoot := filepath.Join(repoRoot, ".work", "psiphon-harness")
	if opt.BinaryPath != "" {
		if err := validateInstallBinarySource(opt.BinaryPath); err != nil {
			fmt.Fprintln(stderr, err)
			return ExitUsage
		}
	}
	sourceBinary, ok := resolveBinary(repoRoot, opt.BinaryPath, runtimeRoot)
	if !ok {
		fmt.Fprintf(stderr, "unable to locate psiphon-tunnel-core-x86_64\n")
		return ExitBinaryNotFound
	}
	if err := validateInstallBinarySource(sourceBinary); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	if err := validateInstallBaseConfigSource(opt.BaseConfig); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	if err := validateRestartScheduleHours(opt.RestartEveryHours); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	existingManifest, _, err := readInstallManifest(layout.ManifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read existing install manifest: %v\n", err)
		return ExitValidationFailed
	}

	installedProfile, err := resolveInstalledProfile(repoRoot, opt)
	if err != nil {
		fmt.Fprintf(stderr, "failed to resolve installed profile: %v\n", err)
		return ExitUsage
	}
	if err := validateInstalledSlotCapacity(installedProfile, detectInstalledSlotCapInfo(opt.ForceUnlockSlotCap)); err != nil {
		fmt.Fprintf(stderr, "failed to validate installed slot count: %v\n", err)
		return ExitUsage
	}
	installedSpecs, err := deriveInstalledSlotSpecs(layout, installedProfile)
	if err != nil {
		fmt.Fprintf(stderr, "failed to derive installed slots: %v\n", err)
		return ExitValidationFailed
	}
	sourceLinph, err := currentExecutablePath()
	if err != nil {
		fmt.Fprintf(stderr, "failed to resolve current executable: %v\n", err)
		return ExitValidationFailed
	}

	if err := os.MkdirAll(layout.BinDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "failed to create install bin dir %s: %v\n", layout.BinDir, err)
		return ExitValidationFailed
	}
	if err := os.MkdirAll(layout.ConfigDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "failed to create install config dir %s: %v\n", layout.ConfigDir, err)
		return ExitValidationFailed
	}
	if err := os.MkdirAll(layout.installedRuntimeRoot(), 0o755); err != nil {
		fmt.Fprintf(stderr, "failed to create installed runtime root %s: %v\n", layout.installedRuntimeRoot(), err)
		return ExitValidationFailed
	}

	for _, path := range append(layout.allPaths(), layout.ManifestPath) {
		if sameCleanPath(path, sourceLinph) {
			continue
		}
		if err := prepareManagedDestination(path, managedPaths, opt.Force); err != nil {
			fmt.Fprintf(stderr, "failed to prepare %s: %v\n", path, err)
			return ExitValidationFailed
		}
	}

	if err := copyFileAtomic(sourceBinary, layout.PsiphonBinaryPath, 0o755); err != nil {
		fmt.Fprintf(stderr, "failed to install tunnel core: %v\n", err)
		return ExitValidationFailed
	}
	if err := copyFileAtomic(opt.BaseConfig, layout.PsiphonConfigPath, 0o644); err != nil {
		fmt.Fprintf(stderr, "failed to install config: %v\n", err)
		return ExitValidationFailed
	}
	if !sameCleanPath(sourceLinph, layout.LinphPath) {
		if err := copyFileAtomic(sourceLinph, layout.LinphPath, 0o755); err != nil {
			fmt.Fprintf(stderr, "failed to install linph: %v\n", err)
			return ExitValidationFailed
		}
	}
	for _, aliasPath := range layout.CompatPaths {
		if err := createAlias(aliasPath, layout.LinphPath); err != nil {
			fmt.Fprintf(stderr, "failed to install alias %s: %v\n", aliasPath, err)
			return ExitValidationFailed
		}
	}
	if err := configurePeriodicRestart(layout, opt.RestartEveryHours, existingManifest.RestartEveryHours); err != nil {
		fmt.Fprintf(stderr, "failed to configure periodic restart: %v\n", err)
		return ExitValidationFailed
	}
	if err := writeInstallManifest(layout, opt.RestartEveryHours); err != nil {
		fmt.Fprintf(stderr, "failed to write install manifest: %v\n", err)
		return ExitValidationFailed
	}

	installedApp := &app{stdout: stdout, stderr: stderr, repoRoot: repoRoot, owner: usageName, usageName: usageName}
	if code := installedApp.withInstalledLock(layout, func() int {
		if err := writeInstalledProviderState(layout, installedProviderStateFromPsi(installedProfile)); err != nil {
			fmt.Fprintf(stderr, "failed to write installed profile: %v\n", err)
			return ExitValidationFailed
		}
		if !opt.StartInstalledSlots {
			return 0
		}
		return installedApp.syncInstalledSlots(layout, installedProfile, installedSpecs, false)
	}); code != 0 {
		fmt.Fprintln(stderr, "install artifacts were written, but initial slot startup did not fully succeed")
		fmt.Fprintln(stderr, "review logs with `linph log` and retry with `linph start`")
		fmt.Fprintln(stderr, "configured slot ports:")
		fmt.Fprintln(stderr, installedPortsCSV(installedSpecs))
		return code
	}

	fmt.Fprintf(stdout, "installed linph to %s\n", layout.LinphPath)
	fmt.Fprintf(stdout, "installed tunnel core to %s\n", layout.PsiphonBinaryPath)
	fmt.Fprintf(stdout, "installed config to %s\n", layout.PsiphonConfigPath)
	if !opt.StartInstalledSlots {
		fmt.Fprintln(stdout, "configured Psiphon provider; run `linph start` to start installed slots")
	}
	if opt.RestartEveryHours > 0 {
		fmt.Fprintf(stdout, "configured periodic restart every %d hour(s)\n", opt.RestartEveryHours)
	}
	fmt.Fprintln(stdout, "installed slot ports:")
	fmt.Fprintln(stdout, installedPortsCSV(installedSpecs))
	return 0
}

func runUninstall(usageName string, args []string, stdout, stderr io.Writer) int {
	opt := uninstallOptions{
		InstallBinDir:    filepath.Dir(installedLinphLauncher),
		InstallConfigDir: installedPsiphonConfigDir,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--install-bin-dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--install-bin-dir requires a value\n")
				uninstallUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstallBinDir = args[i+1]
			opt.InstallBinDirSet = true
			i++
		case "--install-config-dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "--install-config-dir requires a value\n")
				uninstallUsage(stderr, usageName)
				return ExitUsage
			}
			opt.InstallConfigDir = args[i+1]
			opt.InstallConfigDirSet = true
			i++
		case "--purge":
			opt.Purge = true
		case "--help", "-h":
			uninstallUsage(stdout, usageName)
			return 0
		default:
			fmt.Fprintf(stderr, "unknown uninstall option: %s\n", args[i])
			uninstallUsage(stderr, usageName)
			return ExitUsage
		}
	}

	layout, err := uninstallLayout(opt)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read install manifest: %v\n", err)
		return ExitValidationFailed
	}
	installedApp := &app{stdout: stdout, stderr: stderr, owner: usageName, usageName: usageName}
	if code := installedApp.withInstalledLock(layout, func() int {
		return installedApp.stopInstalledSlots(layout)
	}); code != 0 {
		return code
	}
	if err := removePeriodicRestart(); err != nil {
		fmt.Fprintf(stderr, "failed to remove periodic restart: %v\n", err)
		return ExitValidationFailed
	}

	for _, path := range append([]string{layout.LinphPath}, layout.CompatPaths...) {
		if err := removePathIfExists(path); err != nil {
			fmt.Fprintf(stderr, "failed to remove %s: %v\n", path, err)
			return ExitValidationFailed
		}
	}
	if !sameCleanPath(layout.ManifestPath, layout.PsiphonConfigPath) {
		if err := removePathIfExists(layout.ManifestPath); err != nil {
			fmt.Fprintf(stderr, "failed to remove %s: %v\n", layout.ManifestPath, err)
			return ExitValidationFailed
		}
	}
	if err := removePathIfExists(layout.PsiphonBinaryPath); err != nil {
		fmt.Fprintf(stderr, "failed to remove %s: %v\n", layout.PsiphonBinaryPath, err)
		return ExitValidationFailed
	}
	if legacyInstalledPsiphonPath != "" && !containsString(layout.CompatPaths, legacyInstalledPsiphonPath) {
		if err := removePathIfExists(legacyInstalledPsiphonPath); err != nil {
			fmt.Fprintf(stderr, "failed to remove %s: %v\n", legacyInstalledPsiphonPath, err)
			return ExitValidationFailed
		}
	}

	if opt.Purge {
		if err := os.RemoveAll(layout.ConfigDir); err != nil {
			fmt.Fprintf(stderr, "failed to purge %s: %v\n", layout.ConfigDir, err)
			return ExitValidationFailed
		}
		fmt.Fprintf(stdout, "purged %s\n", layout.ConfigDir)
		return 0
	}

	if err := removeDirIfEmpty(layout.ConfigDir); err != nil {
		fmt.Fprintf(stderr, "failed to tidy %s: %v\n", layout.ConfigDir, err)
		return ExitValidationFailed
	}
	fmt.Fprintf(stdout, "uninstalled linph from %s (preserved %s)\n", layout.BinDir, layout.PsiphonConfigPath)
	return 0
}

func installUsage(w io.Writer, usageName string) {
	fmt.Fprintf(w, `Usage:
  %s [options]

Options:
  --binary PATH               Reviewed local tunnel-core binary to install.
  --base-config PATH          Base config to install.
  --install-bin-dir PATH      Install bin dir (default: %s).
  --install-config-dir PATH   Install config dir (default: %s).
	--force                     Overwrite existing unmanaged files.
	--start                     Start installed slots after writing artifacts and provider state.
	--fk                        Ignore the memory-based slot cap and unlock up to 28 slots.
	--installed-slot-count N    Number of installed slots (1-28, further capped by host memory unless --fk).
	--installed-http-port N     Starting HTTP port for installed slots.
	--installed-socks-port N    Starting SOCKS port for installed slots.
	--installed-regions CSV     Comma-separated regions for installed slots.
	--restart-every-hours N     Restart linph every N hours to refresh IP (0 disables, max 168).
	--help                      Show this message.
`, usageName, filepath.Dir(installedLinphLauncher), installedPsiphonConfigDir)
}

func uninstallUsage(w io.Writer, usageName string) {
	fmt.Fprintf(w, `Usage:
  %s [options]

Options:
  --install-bin-dir PATH      Install bin dir (default: %s).
  --install-config-dir PATH   Install config dir (default: %s).
  --purge                     Remove the entire installed config directory.
  --help                      Show this message.
`, usageName, filepath.Dir(installedLinphLauncher), installedPsiphonConfigDir)
}

func fallbackInstallLayout() installLayout {
	return installLayout{
		BinDir:            filepath.Dir(installedLinphLauncher),
		ConfigDir:         installedPsiphonConfigDir,
		LinphPath:         installedLinphLauncher,
		CompatPaths:       []string{installedPsiphonLauncher, installedPlinstallerLauncher, installedPluninstallerPath},
		PsiphonBinaryPath: installedPsiphonBinaryPath,
		PsiphonConfigPath: installedPsiphonConfigPath,
		ManifestPath:      filepath.Join(filepath.Dir(installedLinphLauncher), installedManifestFilename),
	}
}

func buildInstallLayout(binDir, configDir string) installLayout {
	return installLayout{
		BinDir:            binDir,
		ConfigDir:         configDir,
		LinphPath:         filepath.Join(binDir, "linph"),
		CompatPaths:       []string{filepath.Join(binDir, "psiphon"), filepath.Join(binDir, "plinstaller2"), filepath.Join(binDir, "pluninstaller")},
		PsiphonBinaryPath: filepath.Join(configDir, "psiphon-tunnel-core-x86_64"),
		PsiphonConfigPath: filepath.Join(configDir, "psiphon.config"),
		ManifestPath:      filepath.Join(binDir, installedManifestFilename),
	}
}

func (layout installLayout) allPaths() []string {
	paths := []string{layout.LinphPath, layout.PsiphonBinaryPath, layout.PsiphonConfigPath}
	paths = append(paths, layout.CompatPaths...)
	return paths
}

func activeInstallLayout() installLayout {
	manifestPath := filepath.Join(filepath.Dir(installedLinphLauncher), installedManifestFilename)
	if currentPath, err := currentExecutablePath(); err == nil && currentPath != "" {
		manifestPath = filepath.Join(filepath.Dir(currentPath), installedManifestFilename)
	}
	manifest, ok, err := readInstallManifest(manifestPath)
	if err == nil && ok {
		return layoutFromManifest(manifest)
	}
	return fallbackInstallLayout()
}

func uninstallLayout(opt uninstallOptions) (installLayout, error) {
	if opt.InstallBinDirSet {
		manifest, ok, err := readInstallManifest(filepath.Join(opt.InstallBinDir, installedManifestFilename))
		if err != nil {
			return installLayout{}, err
		}
		if ok {
			return layoutFromManifest(manifest), nil
		}
	}
	if !opt.InstallBinDirSet {
		if currentPath, err := currentExecutablePath(); err == nil && currentPath != "" {
			manifest, ok, err := readInstallManifest(filepath.Join(filepath.Dir(currentPath), installedManifestFilename))
			if err != nil {
				return installLayout{}, err
			}
			if ok {
				return layoutFromManifest(manifest), nil
			}
		}
	}

	if opt.InstallBinDir == filepath.Dir(installedLinphLauncher) && opt.InstallConfigDir == installedPsiphonConfigDir {
		return fallbackInstallLayout(), nil
	}
	return buildInstallLayout(opt.InstallBinDir, opt.InstallConfigDir), nil
}

func managedInstallPaths(binDir string) (map[string]struct{}, error) {
	manifest, ok, err := readInstallManifest(filepath.Join(binDir, installedManifestFilename))
	if err != nil {
		return nil, err
	}
	if !ok {
		return map[string]struct{}{}, nil
	}
	managed := map[string]struct{}{}
	layout := layoutFromManifest(manifest)
	for _, path := range append(layout.allPaths(), layout.ManifestPath) {
		managed[path] = struct{}{}
	}
	return managed, nil
}

func readInstallManifest(path string) (installManifest, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return installManifest{}, false, nil
		}
		return installManifest{}, false, err
	}
	var manifest installManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return installManifest{}, false, err
	}
	return manifest, true, nil
}

func writeInstallManifest(layout installLayout, restartEveryHours int) error {
	manifest := installManifest{
		Version:           1,
		BinDir:            layout.BinDir,
		ConfigDir:         layout.ConfigDir,
		LinphPath:         layout.LinphPath,
		CompatPaths:       append([]string(nil), layout.CompatPaths...),
		PsiphonBinaryPath: layout.PsiphonBinaryPath,
		PsiphonConfigPath: layout.PsiphonConfigPath,
		ManifestPath:      layout.ManifestPath,
		RestartEveryHours: restartEveryHours,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return copyBytesAtomic(data, layout.ManifestPath, 0o644)
}

func layoutFromManifest(manifest installManifest) installLayout {
	return installLayout{
		BinDir:            manifest.BinDir,
		ConfigDir:         manifest.ConfigDir,
		LinphPath:         manifest.LinphPath,
		CompatPaths:       append([]string(nil), manifest.CompatPaths...),
		PsiphonBinaryPath: manifest.PsiphonBinaryPath,
		PsiphonConfigPath: manifest.PsiphonConfigPath,
		ManifestPath:      manifest.ManifestPath,
	}
}

func validateRestartScheduleHours(hours int) error {
	if hours < installedRestartScheduleDisabled || hours > installedRestartScheduleMaxHours {
		return fmt.Errorf("--restart-every-hours must be between 0 and %d", installedRestartScheduleMaxHours)
	}
	return nil
}

func configurePeriodicRestart(layout installLayout, hours, existingHours int) error {
	if err := validateRestartScheduleHours(hours); err != nil {
		return err
	}
	if hours == installedRestartScheduleDisabled {
		if existingHours == installedRestartScheduleDisabled {
			return nil
		}
		return removePeriodicRestart()
	}
	return writePeriodicRestart(layout, hours)
}

func writePeriodicRestart(layout installLayout, hours int) error {
	servicePath := filepath.Join(installedSystemdSystemDir, installedRestartServiceName)
	timerPath := filepath.Join(installedSystemdSystemDir, installedRestartTimerName)
	if err := validateSystemdExecPath(layout.LinphPath); err != nil {
		return err
	}
	service := fmt.Sprintf(`[Unit]
Description=Restart Linphon to refresh IP

[Service]
Type=oneshot
ExecStart=%s restart
`, layout.LinphPath)
	timer := fmt.Sprintf(`[Unit]
Description=Restart Linphon every %d hour(s) to refresh IP

[Timer]
OnActiveSec=%dh
OnUnitActiveSec=%dh
Persistent=true
Unit=%s

[Install]
WantedBy=timers.target
`, hours, hours, hours, installedRestartServiceName)
	if err := copyBytesAtomic([]byte(service), servicePath, 0o644); err != nil {
		return err
	}
	if err := copyBytesAtomic([]byte(timer), timerPath, 0o644); err != nil {
		return err
	}
	return reloadAndEnablePeriodicRestart()
}

func validateSystemdExecPath(path string) error {
	if strings.ContainsAny(path, " \t\n\r%") {
		return fmt.Errorf("install path contains characters unsupported by periodic restart systemd unit: %s", path)
	}
	return nil
}

func reloadAndEnablePeriodicRestart() error {
	if err := systemctlCommand("daemon-reload"); err != nil {
		return err
	}
	return systemctlCommand("enable", "--now", installedRestartTimerName)
}

func removePeriodicRestart() error {
	timerPath := filepath.Join(installedSystemdSystemDir, installedRestartTimerName)
	servicePath := filepath.Join(installedSystemdSystemDir, installedRestartServiceName)
	changed := false
	if _, err := os.Stat(timerPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	} else if err := systemctlCommand("disable", "--now", installedRestartTimerName); err != nil {
		return err
	} else {
		changed = true
	}
	if _, err := os.Stat(timerPath); err == nil {
		if err := removePathIfExists(timerPath); err != nil {
			return err
		}
		changed = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(servicePath); err == nil {
		if err := removePathIfExists(servicePath); err != nil {
			return err
		}
		changed = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if changed {
		return systemctlCommand("daemon-reload")
	}
	return nil
}

func resolveInstalledProfile(repoRoot string, opt installOptions) (installedProfile, error) {
	if !opt.InstalledProfileSet {
		return installedProfileFromBaseConfig(repoRoot, opt.BaseConfig)
	}
	if opt.InstalledSlotCount == 0 || opt.InstalledHTTPPort == 0 || opt.InstalledSocksPort == 0 || strings.TrimSpace(opt.InstalledRegionsCSV) == "" {
		return installedProfile{}, fmt.Errorf("all installed profile flags must be provided together")
	}
	return installedProfileFromCSV(repoRoot, opt.InstalledSlotCount, opt.InstalledHTTPPort, opt.InstalledSocksPort, opt.InstalledRegionsCSV)
}

func validateInstallBinarySource(path string) error {
	if err := validateRegularInstallFile("binary", path); err != nil {
		return err
	}
	if ok, err := looksLikeInstallExecutable(path); err != nil {
		return fmt.Errorf("failed to inspect binary %s: %w", path, err)
	} else if !ok {
		return fmt.Errorf("binary must be an executable file (ELF or shebang script): %s", path)
	}
	return nil
}

func validateInstallBaseConfigSource(path string) error {
	if err := validateRegularInstallFile("base config", path); err != nil {
		return err
	}
	data, err := readInstallSourceFile(path)
	if err != nil {
		return fmt.Errorf("failed to read base config %s: %w", path, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("base config must be a valid JSON object: %w", err)
	}
	return nil
}

func validateRegularInstallFile(label, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s not found: %s", label, path)
		}
		return fmt.Errorf("failed to stat %s %s: %w", label, path, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file and not a symlink: %s", label, path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file: %s", label, path)
	}
	return nil
}

func looksLikeInstallExecutable(path string) (bool, error) {
	file, err := openInstallSourceFile(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	header := make([]byte, 4)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	header = header[:n]
	if len(header) >= 4 && string(header[:4]) == "\x7fELF" {
		return true, nil
	}
	if len(header) >= 2 && string(header[:2]) == "#!" {
		return true, nil
	}
	return false, nil
}

func prepareManagedDestination(path string, managedPaths map[string]struct{}, force bool) error {
	_, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if _, ok := managedPaths[path]; ok || force {
		return os.RemoveAll(path)
	}
	return fmt.Errorf("path already exists and is not managed by linph (use --force): %s", path)
}

func createAlias(aliasPath, linphPath string) error {
	target := filepath.Base(linphPath)
	if err := os.Symlink(target, aliasPath); err == nil {
		return nil
	}
	return copyFileAtomic(linphPath, aliasPath, 0o755)
}

func copyFileAtomic(sourcePath, destPath string, mode fs.FileMode) error {
	if sameCleanPath(sourcePath, destPath) {
		return nil
	}
	sourceFile, err := openInstallSourceFile(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source must be a regular file: %s", sourcePath)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(destPath), ".linph-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()
	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(mode); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, destPath)
}

func copyBytesAtomic(data []byte, destPath string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(destPath), ".linph-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(mode); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, destPath)
}

func openInstallSourceFile(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("failed to open %s", path)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("source must be a regular file: %s", path)
	}
	return file, nil
}

func readInstallSourceFile(path string) ([]byte, error) {
	file, err := openInstallSourceFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func removePathIfExists(path string) error {
	if path == "" {
		return nil
	}
	if err := os.RemoveAll(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func removeDirIfEmpty(path string) error {
	if path == "" {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(entries) != 0 {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func sameCleanPath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
