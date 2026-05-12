package mg

import (
	"strings"
	"testing"
)

func TestBuildHarnessInstanceSpecsExplicitMode(t *testing.T) {
	repoRoot := findRepoRoot(t)
	specs, err := buildHarnessInstanceSpecs(repoRoot, harnessOptions{
		InstanceSpecsRaw: []string{"AT:19080:12080", "BE:19081:12081"},
	})
	if err != nil {
		t.Fatalf("build explicit specs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Region != "AT" || specs[0].HTTPPort != 19080 || specs[0].SocksPort != 12080 {
		t.Fatalf("unexpected first spec: %+v", specs[0])
	}
	if specs[1].Region != "BE" || specs[1].HTTPPort != 19081 || specs[1].SocksPort != 12081 {
		t.Fatalf("unexpected second spec: %+v", specs[1])
	}
}

func TestBuildHarnessInstanceSpecsRejectsMixedModes(t *testing.T) {
	repoRoot := findRepoRoot(t)
	_, err := buildHarnessInstanceSpecs(repoRoot, harnessOptions{
		Count:            2,
		CountProvided:    true,
		InstanceSpecsRaw: []string{"AT:19080:12080"},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("expected mixed-mode error, got %v", err)
	}
}

func TestBuildHarnessInstanceSpecsRejectsUnknownExplicitRegion(t *testing.T) {
	repoRoot := findRepoRoot(t)
	_, err := buildHarnessInstanceSpecs(repoRoot, harnessOptions{
		InstanceSpecsRaw: []string{"ZZ:19080:12080"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown region code") {
		t.Fatalf("expected unknown region error, got %v", err)
	}
}

func TestBuildHarnessInstanceSpecsRejectsMalformedExplicitValue(t *testing.T) {
	repoRoot := findRepoRoot(t)
	_, err := buildHarnessInstanceSpecs(repoRoot, harnessOptions{
		InstanceSpecsRaw: []string{"AT:19080:not-a-port"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid SOCKS port") {
		t.Fatalf("expected invalid port error, got %v", err)
	}
}

func TestBuildHarnessInstanceSpecsRejectsLegacyPortOverlap(t *testing.T) {
	repoRoot := findRepoRoot(t)
	_, err := buildHarnessInstanceSpecs(repoRoot, harnessOptions{
		Count:         3,
		HTTPPortBase:  19080,
		SocksPortBase: 19082,
	})
	if err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("expected overlap error, got %v", err)
	}
}
