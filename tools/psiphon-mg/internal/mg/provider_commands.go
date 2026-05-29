package mg

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (a *linphApp) runProviderCommand(args []string) int {
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
	case "set":
		if len(args) == 2 && isPsiProviderName(args[1]) {
			state, ok, err := loadInstalledProviderState(layout)
			if err != nil {
				fmt.Fprintf(a.stderr, "%v\n", err)
				return ExitUsage
			}
			if !ok {
				fmt.Fprintln(a.stderr, "provider state not found; run linph install first")
				return ExitUsage
			}
			state.ActiveProvider = installedProviderPsi
			if err := writeInstalledProviderState(layout, state); err != nil {
				fmt.Fprintf(a.stderr, "failed to persist provider state: %v\n", err)
				return ExitUsage
			}
			fmt.Fprintln(a.stdout, installedProviderPsi)
			return 0
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

func (a *linphApp) runPsiCommand(args []string) int {
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

func (a *linphApp) runPsiSet(args []string) int {
	if len(args) > 0 && isPsiProviderName(args[0]) {
		args = args[1:]
	}
	layout := activeInstallLayout()
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
`)
}

func isPsiProviderName(name string) bool {
	return name == installedProviderPsi || name == "psiphon"
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

func isPsiProviderActive(layout installLayout) error {
	state, ok, err := loadInstalledProviderState(layout)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("provider state not found at %s", layout.installedProviderProfilePath())
	}
	if state.ActiveProvider != installedProviderPsi {
		return fmt.Errorf("unsupported active provider in phase one: %s", state.ActiveProvider)
	}
	return nil
}

func describeProviderState(state installedProviderState) string {
	profile, err := installedPsiProfileFromState(state)
	if err != nil {
		return state.ActiveProvider
	}
	return strings.Join([]string{state.ActiveProvider, strconv.Itoa(profile.SlotCount)}, " ")
}
