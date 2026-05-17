package mg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeInstalledMemorySlotCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		totalMiB uint64
		want     int
	}{
		{name: "fallback", totalMiB: 0, want: 1},
		{name: "small", totalMiB: 99, want: 1},
		{name: "one slot", totalMiB: 100, want: 1},
		{name: "two slots", totalMiB: 256, want: 2},
		{name: "hard limit", totalMiB: 8192, want: 28},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeInstalledMemorySlotCap(tc.totalMiB); got != tc.want {
				t.Fatalf("computeInstalledMemorySlotCap(%d) = %d, want %d", tc.totalMiB, got, tc.want)
			}
		})
	}
}

func TestDetectInstalledSlotCapInfoPrefersLowerCgroupLimit(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

	tempDir := t.TempDir()
	meminfoPath := filepath.Join(tempDir, "meminfo")
	cgroupPath := filepath.Join(tempDir, "memory.max")
	if err := os.WriteFile(meminfoPath, []byte("MemTotal:        1048576 kB\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", meminfoPath, err)
	}
	if err := os.WriteFile(cgroupPath, []byte("268435456\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", cgroupPath, err)
	}
	installedProcMeminfoPath = meminfoPath
	installedCgroupLimitPaths = []string{cgroupPath}

	capInfo := detectInstalledSlotCapInfo(false)
	if got, want := capInfo.Limit, 2; got != want {
		t.Fatalf("detectInstalledSlotCapInfo(false).Limit = %d, want %d", got, want)
	}
	if got, want := capInfo.Source, "cgroup"; got != want {
		t.Fatalf("detectInstalledSlotCapInfo(false).Source = %q, want %q", got, want)
	}
}

func TestValidateInstalledSlotCapacitySuggestsFK(t *testing.T) {
	t.Parallel()

	err := validateInstalledSlotCapacity(installedProfile{SlotCount: 3}, installedSlotCapInfo{Limit: 2, HardLimit: installedSlotCountHardLimit, EffectiveMemoryMiB: 256})
	if err == nil {
		t.Fatalf("validateInstalledSlotCapacity() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "use --fk") {
		t.Fatalf("validateInstalledSlotCapacity() error = %q, want fk guidance", err.Error())
	}
}

func TestRunInstallRejectsSlotCountAboveDetectedMemoryCap(t *testing.T) {
	restore := overrideInstallGlobals(t)
	defer restore()

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
	meminfoPath := filepath.Join(fixtureRoot, "meminfo")
	if err := os.WriteFile(meminfoPath, []byte("MemTotal:         262144 kB\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", meminfoPath, err)
	}
	installedProcMeminfoPath = meminfoPath
	installedCgroupLimitPaths = nil
	currentExecutablePath = func() (string, error) { return sourceLinph, nil }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"--binary", sourceBinary,
		"--base-config", baseConfig,
		"--install-bin-dir", binDir,
		"--install-config-dir", configDir,
		"--installed-slot-count", "3",
		"--installed-http-port", "18080",
		"--installed-socks-port", "18080",
		"--installed-regions", "US,CA,JP",
	}
	if exitCode := runInstall(repoRoot, "linph install", args, &stdout, &stderr); exitCode != ExitUsage {
		t.Fatalf("runInstall() exit = %d, want %d (stderr=%s)", exitCode, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "use --fk") {
		t.Fatalf("runInstall() stderr = %q, want fk guidance", stderr.String())
	}
}
