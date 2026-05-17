package mg

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	installedSlotCountHardLimit = 28
	installedSlotCapFallback    = 1
	installedSlotMemoryPerMiB   = 50
	installedSlotMemoryShareDiv = 2
	installedSlotMemoryScaleDiv = installedSlotMemoryPerMiB * installedSlotMemoryShareDiv
)

var (
	installedProcMeminfoPath  = "/proc/meminfo"
	installedCgroupLimitPaths = []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
		"/sys/fs/cgroup/memory.limit_in_bytes",
	}
	installedReadFile = os.ReadFile
)

type installedSlotCapInfo struct {
	Limit              int
	HardLimit          int
	EffectiveMemoryMiB uint64
	Override           bool
	Source             string
}

func detectInstalledSlotCapInfo(override bool) installedSlotCapInfo {
	effectiveMemoryMiB, source := detectInstalledEffectiveMemoryMiB()
	limit := computeInstalledMemorySlotCap(effectiveMemoryMiB)
	if override {
		limit = installedSlotCountHardLimit
	}
	return installedSlotCapInfo{
		Limit:              limit,
		HardLimit:          installedSlotCountHardLimit,
		EffectiveMemoryMiB: effectiveMemoryMiB,
		Override:           override,
		Source:             source,
	}
}

func validateInstalledSlotCapacity(profile installedProfile, capInfo installedSlotCapInfo) error {
	if profile.SlotCount <= capInfo.Limit {
		return nil
	}
	if capInfo.EffectiveMemoryMiB > 0 {
		return fmt.Errorf("slot count %d exceeds memory-based cap %d for this host (%d MiB effective, use --fk to unlock up to %d)", profile.SlotCount, capInfo.Limit, capInfo.EffectiveMemoryMiB, capInfo.HardLimit)
	}
	return fmt.Errorf("slot count %d exceeds detected safety cap %d for this host (use --fk to unlock up to %d)", profile.SlotCount, capInfo.Limit, capInfo.HardLimit)
}

func computeInstalledMemorySlotCap(totalMemoryMiB uint64) int {
	if totalMemoryMiB == 0 {
		return installedSlotCapFallback
	}
	limit := int(totalMemoryMiB / installedSlotMemoryScaleDiv)
	if limit < installedSlotCapFallback {
		return installedSlotCapFallback
	}
	if limit > installedSlotCountHardLimit {
		return installedSlotCountHardLimit
	}
	return limit
}

func detectInstalledEffectiveMemoryMiB() (uint64, string) {
	hostMiB, hostErr := readInstalledHostMemoryMiB()
	cgroupMiB, cgroupLimited, cgroupErr := readInstalledCgroupLimitMiB(hostMiB)

	switch {
	case cgroupLimited && cgroupMiB > 0:
		return cgroupMiB, "cgroup"
	case hostMiB > 0:
		return hostMiB, "host"
	case cgroupMiB > 0:
		return cgroupMiB, "cgroup"
	case hostErr == nil && cgroupErr == nil:
		return 0, "fallback"
	default:
		return 0, "fallback"
	}
}

func readInstalledHostMemoryMiB() (uint64, error) {
	data, err := installedReadFile(installedProcMeminfoPath)
	if err != nil {
		return 0, err
	}
	return parseInstalledMemTotalMiB(string(data))
}

func parseInstalledMemTotalMiB(raw string) (uint64, error) {
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kib, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse MemTotal: %w", err)
		}
		if kib == 0 {
			return 0, fmt.Errorf("MemTotal is zero")
		}
		return kib / 1024, nil
	}
	return 0, fmt.Errorf("MemTotal not found")
}

func readInstalledCgroupLimitMiB(hostMiB uint64) (uint64, bool, error) {
	var firstErr error
	for _, path := range installedCgroupLimitPaths {
		data, err := installedReadFile(path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		limitBytes, constrained, err := parseInstalledMemoryLimitBytes(string(data))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !constrained {
			continue
		}
		limitMiB := limitBytes / (1024 * 1024)
		if limitMiB == 0 {
			limitMiB = 1
		}
		if hostMiB > 0 {
			if limitMiB < hostMiB {
				return limitMiB, true, nil
			}
			continue
		}
		return limitMiB, true, nil
	}
	return 0, false, firstErr
}

func parseInstalledMemoryLimitBytes(raw string) (uint64, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "max" {
		return 0, false, nil
	}
	value, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, false, err
	}
	if value == 0 || value >= (1<<50) {
		return 0, false, nil
	}
	return value, true, nil
}
