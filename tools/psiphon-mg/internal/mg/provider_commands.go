package mg

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (a *app) runProviderCommand(args []string) int {
	if len(args) == 0 {
		providerUsage(a.stderr)
		return ExitUsage
	}
	layout := activeInstallLayout()
	switch args[0] {
	case "get":
		if len(args) != 1 {
			providerUsage(a.stderr)
			return ExitUsage
		}
		return a.withInstalledLock(layout, func() int {
			state, ok, err := loadInstalledProviderState(layout)
			if err != nil {
				fmt.Fprintf(a.stderr, "%v\n", err)
				return ExitUsage
			}
			if !ok {
				fmt.Fprintln(a.stderr, "provider state not found; run linph install first")
				return ExitUsage
			}
			fmt.Fprintln(a.stdout, state.ActiveProvider)
			return 0
		})
	case "set":
		if len(args) == 2 && (isPsiProviderName(args[1]) || isVGProviderName(args[1])) {
			return a.withInstalledLock(layout, func() int {
				state, ok, err := loadInstalledProviderState(layout)
				if err != nil {
					fmt.Fprintf(a.stderr, "%v\n", err)
					return ExitUsage
				}
				if !ok {
					fmt.Fprintln(a.stderr, "provider state not found; run linph install first")
					return ExitUsage
				}
				targetProvider := installedProviderPsi
				if isPsiProviderName(args[1]) {
					targetProvider = installedProviderPsi
				} else {
					if state.Providers.VG == nil {
						fmt.Fprintln(a.stderr, "vg provider state not found; run linph vg set first")
						return ExitUsage
					}
					targetProvider = installedProviderVG
				}
				state.ActiveProvider = targetProvider
				if err := writeInstalledProviderState(layout, state); err != nil {
					fmt.Fprintf(a.stderr, "failed to persist provider state: %v\n", err)
					return ExitUsage
				}
				if code := a.stopInactiveInstalledRuntimes(layout, targetProvider); code != 0 {
					return code
				}
				fmt.Fprintln(a.stdout, state.ActiveProvider)
				return 0
			})
		}
		if len(args) != 2 {
			providerUsage(a.stderr)
			return ExitUsage
		}
		providerUsage(a.stderr)
		return ExitUsage
	case "--help", "-h", "help":
		providerUsage(a.stdout)
		return 0
	default:
		providerUsage(a.stderr)
		return ExitUsage
	}
}

func (a *app) runVGCommand(args []string) int {
	if len(args) == 0 {
		vgUsage(a.stderr)
		return ExitUsage
	}
	switch args[0] {
	case "set":
		return a.runVGSet(args[1:])
	case "refresh":
		return a.runVGRefresh(args[1:])
	case "--help", "-h", "help":
		vgUsage(a.stdout)
		return 0
	default:
		vgUsage(a.stderr)
		return ExitUsage
	}
}

func (a *app) runVGSet(args []string) int {
	if len(args) > 0 && isVGProviderName(args[0]) {
		args = args[1:]
	}
	layout := activeInstallLayout()
	return a.withInstalledLock(layout, func() int {
		state, ok, err := loadInstalledProviderState(layout)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}
		if !ok {
			fmt.Fprintln(a.stderr, "provider state not found; run linph install first")
			return ExitUsage
		}
		profile := defaultInstalledVGProfile(layout)
		if state.Providers.VG != nil {
			profile = *state.Providers.VG
			if profile.CachePath == "" {
				profile.CachePath = layout.installedVGCachePath()
			}
		}
		activate := false
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--regions":
				value, ok := psiSetValue(args, &i, a.stderr, "--regions")
				if !ok {
					return ExitUsage
				}
				regions := normalizeInstalledRegions(value)
				if len(regions) == 0 {
					fmt.Fprintln(a.stderr, "--regions requires at least one region")
					return ExitUsage
				}
				profile.Regions = regions
			case "--openvpn":
				value, ok := psiSetValue(args, &i, a.stderr, "--openvpn")
				if !ok {
					return ExitUsage
				}
				profile.OpenVPNBinaryPath = value
			case "--api-url":
				value, ok := psiSetValue(args, &i, a.stderr, "--api-url")
				if !ok {
					return ExitUsage
				}
				profile.APIURL = value
			case "--cache":
				value, ok := psiSetValue(args, &i, a.stderr, "--cache")
				if !ok {
					return ExitUsage
				}
				profile.CachePath = value
			case "--refresh":
				profile.Refresh = true
			case "--allow-insecure-api-url":
				profile.AllowInsecureAPIURL = true
			case "--allow-local-api-url":
				profile.AllowLocalAPIURL = true
			case "--allow-unsafe-cache-path":
				profile.AllowUnsafeCachePath = true
			case "--activate":
				activate = true
			case "--help", "-h", "help":
				vgSetUsage(a.stdout)
				return 0
			default:
				fmt.Fprintf(a.stderr, "unknown vg set option: %s\n", args[i])
				vgSetUsage(a.stderr)
				return ExitUsage
			}
		}
		var spec installedVGSpec
		profile, spec, err = validateInstalledVGProfile(profile)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}
		profile.CachePath, err = resolveInstalledVGCachePath(layout, profile)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}
		state.Providers.VG = &profile
		if activate || state.ActiveProvider == "" {
			state.ActiveProvider = installedProviderVG
		}
		if err := writeInstalledProviderState(layout, state); err != nil {
			fmt.Fprintf(a.stderr, "failed to persist provider state: %v\n", err)
			return ExitUsage
		}
		fmt.Fprintf(a.stdout, "vg region=%s openvpn=%s api=%s\n", spec.Region, profile.OpenVPNBinaryPath, profile.APIURL)
		return 0
	})
}

func (a *app) runVGRefresh(args []string) int {
	if len(args) != 0 {
		vgUsage(a.stderr)
		return ExitUsage
	}
	layout := activeInstallLayout()
	return a.withInstalledLock(layout, func() int {
		state, ok, err := loadInstalledProviderState(layout)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}
		if !ok || state.Providers.VG == nil {
			fmt.Fprintln(a.stderr, "vg provider state not found; run linph vg set first")
			return ExitUsage
		}
		profile := *state.Providers.VG
		profile.CachePath, err = resolveInstalledVGCachePath(layout, profile)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}
		refreshProfile := profile
		refreshProfile.Refresh = true
		data, err := readVPNGateCSV(layout, refreshProfile)
		if err != nil {
			fmt.Fprintf(a.stderr, "failed to refresh VPNGate server list: %v\n", err)
			return ExitUsage
		}
		servers, err := parseVPNGateServers(data)
		if err != nil {
			fmt.Fprintf(a.stderr, "failed to parse VPNGate server list: %v\n", err)
			return ExitUsage
		}
		profile.Refresh = false
		profile.CachePath = refreshProfile.CachePath
		state.Providers.VG = &profile
		if err := writeInstalledProviderState(layout, state); err != nil {
			fmt.Fprintf(a.stderr, "failed to persist provider state: %v\n", err)
			return ExitUsage
		}
		fmt.Fprintf(a.stdout, "cached %d VPNGate OpenVPN servers\n", len(servers))
		return 0
	})
}

func (a *app) runPsiCommand(args []string) int {
	if len(args) == 0 {
		psiUsage(a.stderr)
		return ExitUsage
	}
	switch args[0] {
	case "set":
		return a.runPsiSet(args[1:])
	case "--help", "-h", "help":
		psiUsage(a.stdout)
		return 0
	default:
		psiUsage(a.stderr)
		return ExitUsage
	}
}

func (a *app) runPsiSet(args []string) int {
	if len(args) > 0 && isPsiProviderName(args[0]) {
		args = args[1:]
	}
	layout := activeInstallLayout()
	return a.withInstalledLock(layout, func() int {
		state, ok, err := loadInstalledProviderState(layout)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}
		if !ok {
			fmt.Fprintln(a.stderr, "provider state not found; run linph install first")
			return ExitUsage
		}
		profile, err := installedPsiProfileFromState(state)
		if err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}

		activate := false
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--slot-count":
				value, ok := psiSetValue(args, &i, a.stderr, "--slot-count")
				if !ok {
					return ExitUsage
				}
				count, err := strconv.Atoi(value)
				if err != nil || count < 1 || count > installedSlotCountHardLimit {
					fmt.Fprintf(a.stderr, "--slot-count must be between 1 and %d\n", installedSlotCountHardLimit)
					return ExitUsage
				}
				profile.SlotCount = count
			case "--http-port":
				value, ok := psiSetValue(args, &i, a.stderr, "--http-port")
				if !ok {
					return ExitUsage
				}
				port, err := parseInstalledPort(value)
				if err != nil {
					fmt.Fprintf(a.stderr, "%v\n", err)
					return ExitUsage
				}
				profile.HTTPPortBase = port
			case "--socks-port":
				value, ok := psiSetValue(args, &i, a.stderr, "--socks-port")
				if !ok {
					return ExitUsage
				}
				port, err := parseInstalledPort(value)
				if err != nil {
					fmt.Fprintf(a.stderr, "%v\n", err)
					return ExitUsage
				}
				profile.SocksPortBase = port
			case "--regions":
				value, ok := psiSetValue(args, &i, a.stderr, "--regions")
				if !ok {
					return ExitUsage
				}
				regions := normalizeInstalledRegions(value)
				if len(regions) < profile.SlotCount {
					fmt.Fprintf(a.stderr, "need at least %d region(s) for %d slot(s)\n", profile.SlotCount, profile.SlotCount)
					return ExitUsage
				}
				if err := validateInstalledRegions(a.repoRoot, regions[:profile.SlotCount]); err != nil {
					fmt.Fprintf(a.stderr, "%v\n", err)
					return ExitUsage
				}
				profile.Regions = append([]string(nil), regions[:profile.SlotCount]...)
			case "--activate":
				activate = true
			case "--help", "-h", "help":
				psiSetUsage(a.stdout)
				return 0
			default:
				fmt.Fprintf(a.stderr, "unknown psi set option: %s\n", args[i])
				psiSetUsage(a.stderr)
				return ExitUsage
			}
		}
		if len(profile.Regions) < profile.SlotCount {
			fmt.Fprintf(a.stderr, "need at least %d region(s) for %d slot(s)\n", profile.SlotCount, profile.SlotCount)
			return ExitUsage
		}
		if _, err := deriveInstalledSlotSpecs(layout, profile); err != nil {
			fmt.Fprintf(a.stderr, "%v\n", err)
			return ExitUsage
		}
		state.Providers.Psi = &profile
		if activate || state.ActiveProvider == "" {
			state.ActiveProvider = installedProviderPsi
		}
		if err := writeInstalledProviderState(layout, state); err != nil {
			fmt.Fprintf(a.stderr, "failed to persist provider state: %v\n", err)
			return ExitUsage
		}
		if specs, err := deriveInstalledSlotSpecs(layout, profile); err == nil {
			fmt.Fprintln(a.stdout, installedPortsCSV(specs))
		}
		return 0
	})
}

func psiSetValue(args []string, index *int, stderr io.Writer, flag string) (string, bool) {
	if *index+1 >= len(args) {
		fmt.Fprintf(stderr, "%s requires a value\n", flag)
		return "", false
	}
	*index++
	return args[*index], true
}

func providerUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  linph provider get
  linph provider set psi
  linph provider set vg
`)
}

func isPsiProviderName(name string) bool {
	return name == installedProviderPsi || name == "psiphon"
}

func isVGProviderName(name string) bool {
	return name == installedProviderVG || name == "vpngate"
}

func psiUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  linph psi set [options]
  linph psi set psiphon [options]
`)
}

func psiSetUsage(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  linph psi set [options]
  linph psi set psiphon [options]

Options:
  --slot-count N       Number of Psiphon slots (1-%d).
  --http-port N        Starting HTTP port.
  --socks-port N       Starting SOCKS port.
  --regions CSV        Comma-separated region list.
  --activate           Select psi as the active provider.
  --help               Show this message.
`, installedSlotCountHardLimit)
}

func vgUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  linph vg set [options]
  linph vg refresh
`)
}

func vgSetUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  linph vg set [options]
  linph vg set vpngate [options]

Options:
  --regions CSV        Preferred VPNGate country codes, first match is used.
  --openvpn PATH       OpenVPN executable path (default: openvpn).
  --api-url URL        VPNGate CSV API URL.
  --cache PATH         VPNGate CSV cache path.
  --refresh            Refresh the cache on next start.
  --allow-insecure-api-url
                       Allow plaintext http:// VPNGate API URLs.
  --allow-local-api-url
                        Allow file:// or local API paths for offline fixtures.
  --allow-unsafe-cache-path
                       Allow cache paths outside the managed vg runtime.
  --activate           Select vg as the active provider.
  --help               Show this message.
`)
}

func isPsiProviderActive(layout installLayout) error {
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("provider state not found at %s", layout.installedProviderProfilePath())
	}
	if state.ActiveProvider != installedProviderPsi {
		return fmt.Errorf("active provider is %s, not psi", state.ActiveProvider)
	}
	return nil
}

func describeProviderState(state installedProviderState) string {
	if state.ActiveProvider == installedProviderVG && state.Providers.VG != nil {
		return state.ActiveProvider + " " + strings.Join(state.Providers.VG.Regions, ",")
	}
	profile, err := installedPsiProfileFromState(state)
	if err != nil {
		return state.ActiveProvider
	}
	return strings.Join([]string{state.ActiveProvider, strconv.Itoa(profile.SlotCount)}, " ")
}
