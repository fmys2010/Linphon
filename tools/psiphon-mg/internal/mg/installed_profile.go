package mg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	installedProfileFilename     = "linph-installed-profile.json"
	installedRuntimeRootName     = "linph-runtime"
	installedRuntimeSlotsDirName = "slots"
	installedSlotCountLimit      = 5
	installedProfileVersion      = 1
)

type installedProfile struct {
	Version       int      `json:"version"`
	SlotCount     int      `json:"slot_count"`
	HTTPPortBase  int      `json:"http_port_base"`
	SocksPortBase int      `json:"socks_port_base"`
	Regions       []string `json:"regions"`
}

type installedSlotSpec struct {
	Index       int
	Region      string
	HTTPPort    int
	SocksPort   int
	RuntimeRoot string
}

func (layout installLayout) installedRuntimeRoot() string {
	return filepath.Join(layout.ConfigDir, installedRuntimeRootName)
}

func (layout installLayout) installedSlotsRoot() string {
	return filepath.Join(layout.installedRuntimeRoot(), installedRuntimeSlotsDirName)
}

func (layout installLayout) installedProfilePath() string {
	return filepath.Join(layout.installedRuntimeRoot(), installedProfileFilename)
}

func (layout installLayout) installedSlotRoot(index int) string {
	return filepath.Join(layout.installedSlotsRoot(), fmt.Sprintf("slot-%03d", index+1))
}

func (layout installLayout) installedProfilePaths() []string {
	return []string{
		layout.installedRuntimeRoot(),
		layout.installedSlotsRoot(),
		layout.installedProfilePath(),
	}
}

func readInstalledProfile(path string) (installedProfile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return installedProfile{}, false, nil
		}
		return installedProfile{}, false, err
	}
	var profile installedProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return installedProfile{}, false, err
	}
	return profile, true, nil
}

func writeInstalledProfile(path string, profile installedProfile) error {
	profile.Version = installedProfileVersion
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return copyBytesAtomic(append(data, '\n'), path, 0o644)
}

func installedProfileFromBaseConfig(repoRoot, baseConfigPath string) (installedProfile, error) {
	httpPort, socksPort := readDefaultPorts(baseConfigPath)
	region := readDefaultRegion(repoRoot, baseConfigPath)
	return installedProfile{
		Version:       installedProfileVersion,
		SlotCount:     1,
		HTTPPortBase:  httpPort,
		SocksPortBase: socksPort,
		Regions:       []string{region},
	}, nil
}

func normalizeInstalledRegions(raw string) []string {
	parts := strings.Split(raw, ",")
	regions := make([]string, 0, len(parts))
	for _, part := range parts {
		region := strings.ToUpper(strings.TrimSpace(part))
		if region == "" {
			continue
		}
		regions = append(regions, region)
	}
	return regions
}

func deriveInstalledSlotSpecs(layout installLayout, profile installedProfile) ([]installedSlotSpec, error) {
	if profile.SlotCount < 1 || profile.SlotCount > installedSlotCountLimit {
		return nil, fmt.Errorf("slot count must be between 1 and %d", installedSlotCountLimit)
	}
	if profile.HTTPPortBase <= 0 {
		return nil, fmt.Errorf("HTTP port must be greater than zero: %d", profile.HTTPPortBase)
	}
	if profile.SocksPortBase <= 0 {
		return nil, fmt.Errorf("SOCKS port must be greater than zero: %d", profile.SocksPortBase)
	}
	if len(profile.Regions) < profile.SlotCount {
		return nil, fmt.Errorf("need at least %d region(s) for %d slot(s)", profile.SlotCount, profile.SlotCount)
	}

	usedPorts := map[int]struct{}{}
	specs := make([]installedSlotSpec, 0, profile.SlotCount)
	for index := 0; index < profile.SlotCount; index++ {
		httpPort, err := resolveInstalledPort(profile.HTTPPortBase+index, usedPorts)
		if err != nil {
			return nil, fmt.Errorf("slot %d HTTP port: %w", index+1, err)
		}
		socksPort, err := resolveInstalledPort(profile.SocksPortBase+index, usedPorts)
		if err != nil {
			return nil, fmt.Errorf("slot %d SOCKS port: %w", index+1, err)
		}
		specs = append(specs, installedSlotSpec{
			Index:       index,
			Region:      profile.Regions[index],
			HTTPPort:    httpPort,
			SocksPort:   socksPort,
			RuntimeRoot: layout.installedSlotRoot(index),
		})
	}

	if err := validateHarnessInstanceSpecs(installedSpecHarness(specs)); err != nil {
		return nil, err
	}

	return specs, nil
}

func resolveInstalledPort(basePort int, usedPorts map[int]struct{}) (int, error) {
	port := basePort
	if port <= 0 {
		port = 1
	}
	for {
		if port > 65535 {
			return 0, fmt.Errorf("port range exhausted while resolving from %d", basePort)
		}
		if _, ok := usedPorts[port]; !ok {
			usedPorts[port] = struct{}{}
			return port, nil
		}
		port++
	}
}

func installedSpecHarness(specs []installedSlotSpec) []harnessInstanceSpec {
	harnessSpecs := make([]harnessInstanceSpec, 0, len(specs))
	for _, spec := range specs {
		harnessSpecs = append(harnessSpecs, harnessInstanceSpec{
			Region:    spec.Region,
			HTTPPort:  spec.HTTPPort,
			SocksPort: spec.SocksPort,
		})
	}
	return harnessSpecs
}

func installedPortsCSV(specs []installedSlotSpec) string {
	parts := make([]string, 0, len(specs))
	for _, spec := range specs {
		parts = append(parts, fmt.Sprintf("slot-%03d region=%s http=%d socks=%d", spec.Index+1, spec.Region, spec.HTTPPort, spec.SocksPort))
	}
	return strings.Join(parts, "\n")
}

func installedRegionsCSV(specs []installedSlotSpec) string {
	parts := make([]string, 0, len(specs))
	for _, spec := range specs {
		parts = append(parts, spec.Region)
	}
	return strings.Join(parts, ",")
}

func installedSlotPrefix(spec installedSlotSpec, stream string) string {
	return fmt.Sprintf("[slot-%03d %s %s]", spec.Index+1, spec.Region, stream)
}

func installedSlotStateLabel(index int) string {
	return "slot-" + strings.TrimLeft(fmt.Sprintf("%03d", index+1), "0")
}

func installedProfileFromCSV(slotCount, httpPortBase, socksPortBase int, regionsCSV string) (installedProfile, error) {
	regions := normalizeInstalledRegions(regionsCSV)
	if slotCount < 1 || slotCount > installedSlotCountLimit {
		return installedProfile{}, fmt.Errorf("slot count must be between 1 and %d", installedSlotCountLimit)
	}
	if len(regions) < slotCount {
		return installedProfile{}, fmt.Errorf("need at least %d region(s) for %d slot(s)", slotCount, slotCount)
	}
	return installedProfile{
		Version:       installedProfileVersion,
		SlotCount:     slotCount,
		HTTPPortBase:  httpPortBase,
		SocksPortBase: socksPortBase,
		Regions:       append([]string(nil), regions[:slotCount]...),
	}, nil
}

func parseInstalledPort(raw string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("port must be greater than zero: %s", raw)
	}
	return port, nil
}
