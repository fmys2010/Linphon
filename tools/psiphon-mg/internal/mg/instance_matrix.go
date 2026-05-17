package mg

import (
	"fmt"
	"strconv"
	"strings"
)

type harnessInstanceSpec struct {
	Region    string
	HTTPPort  int
	SocksPort int
}

func buildHarnessInstanceSpecs(repoRoot string, opt harnessOptions) ([]harnessInstanceSpec, error) {
	if len(opt.InstanceSpecsRaw) > 0 {
		if opt.CountProvided || opt.RegionsProvided || opt.HTTPPortBaseProvided || opt.SocksPortBaseProvided {
			return nil, fmt.Errorf("--instance cannot be combined with --count, --regions, --http-port-base, or --socks-port-base")
		}
		return buildExplicitHarnessInstanceSpecs(repoRoot, opt.InstanceSpecsRaw)
	}

	return buildLegacyHarnessInstanceSpecs(repoRoot, opt.Count, opt.RegionsCSV, opt.HTTPPortBase, opt.SocksPortBase)
}

func buildExplicitHarnessInstanceSpecs(repoRoot string, rawSpecs []string) ([]harnessInstanceSpec, error) {
	specs := make([]harnessInstanceSpec, 0, len(rawSpecs))
	for _, rawSpec := range rawSpecs {
		spec, err := parseHarnessInstanceSpec(rawSpec)
		if err != nil {
			return nil, err
		}
		if !isKnownRegion(repoRoot, spec.Region) {
			return nil, fmt.Errorf("unknown region code: %s", spec.Region)
		}
		specs = append(specs, spec)
	}

	if err := validateHarnessInstanceSpecs(specs); err != nil {
		return nil, err
	}
	return specs, nil
}

func buildLegacyHarnessInstanceSpecs(repoRoot string, count int, overrideCSV string, httpPortBase, socksPortBase int) ([]harnessInstanceSpec, error) {
	regions, err := buildHarnessRegionList(repoRoot, count, overrideCSV)
	if err != nil {
		return nil, err
	}

	specs := make([]harnessInstanceSpec, 0, count)
	for index, region := range regions {
		specs = append(specs, harnessInstanceSpec{
			Region:    region,
			HTTPPort:  httpPortBase + index,
			SocksPort: socksPortBase + index,
		})
	}

	if err := validateHarnessInstanceSpecs(specs); err != nil {
		return nil, err
	}
	return specs, nil
}

func parseHarnessInstanceSpec(raw string) (harnessInstanceSpec, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return harnessInstanceSpec{}, fmt.Errorf("invalid --instance value %q: want REGION:HTTP_PORT:SOCKS_PORT", raw)
	}

	parts := strings.Split(trimmed, ":")
	if len(parts) != 3 {
		return harnessInstanceSpec{}, fmt.Errorf("invalid --instance value %q: want REGION:HTTP_PORT:SOCKS_PORT", raw)
	}

	region := strings.TrimSpace(parts[0])
	if region == "" {
		return harnessInstanceSpec{}, fmt.Errorf("invalid --instance value %q: region is required", raw)
	}

	httpPort, err := parseHarnessInstancePort(parts[1], "HTTP", raw)
	if err != nil {
		return harnessInstanceSpec{}, err
	}
	socksPort, err := parseHarnessInstancePort(parts[2], "SOCKS", raw)
	if err != nil {
		return harnessInstanceSpec{}, err
	}

	return harnessInstanceSpec{
		Region:    region,
		HTTPPort:  httpPort,
		SocksPort: socksPort,
	}, nil
}

func parseHarnessInstancePort(raw, label, source string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > 65535 {
		return 0, fmt.Errorf("invalid %s port in --instance value %q: must be between 1 and 65535", label, source)
	}
	return value, nil
}

func validateHarnessInstanceSpecs(specs []harnessInstanceSpec) error {
	seenHTTP := map[int]int{}
	seenSocks := map[int]int{}
	seenAny := map[int]string{}

	for index, spec := range specs {
		instanceNumber := index + 1

		if err := validateHarnessInstancePortRange("HTTP", instanceNumber, spec.HTTPPort); err != nil {
			return err
		}
		if err := validateHarnessInstancePortRange("SOCKS", instanceNumber, spec.SocksPort); err != nil {
			return err
		}

		if previous, ok := seenHTTP[spec.HTTPPort]; ok {
			return fmt.Errorf("duplicate HTTP port %d for instance %d and %d", spec.HTTPPort, previous, instanceNumber)
		}
		if previous, ok := seenAny[spec.HTTPPort]; ok {
			return fmt.Errorf("HTTP port %d for instance %d overlaps %s", spec.HTTPPort, instanceNumber, previous)
		}
		seenHTTP[spec.HTTPPort] = instanceNumber
		seenAny[spec.HTTPPort] = fmt.Sprintf("HTTP port for instance %d", instanceNumber)

		if previous, ok := seenSocks[spec.SocksPort]; ok {
			return fmt.Errorf("duplicate SOCKS port %d for instance %d and %d", spec.SocksPort, previous, instanceNumber)
		}
		if previous, ok := seenAny[spec.SocksPort]; ok {
			return fmt.Errorf("SOCKS port %d for instance %d overlaps %s", spec.SocksPort, instanceNumber, previous)
		}
		seenSocks[spec.SocksPort] = instanceNumber
		seenAny[spec.SocksPort] = fmt.Sprintf("SOCKS port for instance %d", instanceNumber)
	}

	return nil
}

func validateHarnessInstancePortRange(label string, instanceNumber, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s port for instance %d must be between 1 and 65535: %d", label, instanceNumber, port)
	}
	return nil
}

func harnessRegions(specs []harnessInstanceSpec) []string {
	regions := make([]string, 0, len(specs))
	for _, spec := range specs {
		regions = append(regions, spec.Region)
	}
	return regions
}
