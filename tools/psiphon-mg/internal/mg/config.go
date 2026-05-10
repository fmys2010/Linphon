package mg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func readDefaultPorts(configPath string) (int, int) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return 8081, 1081
	}
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return 8081, 1081
	}
	httpPort := intFromAny(raw["LocalHttpProxyPort"], 8081)
	socksPort := intFromAny(raw["LocalSocksProxyPort"], 1081)
	return httpPort, socksPort
}

func intFromAny(value any, fallback int) int {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case int:
		if typed > 0 {
			return typed
		}
	}
	return fallback
}

func resolveBinary(repoRoot, explicitBinary, runtimeRoot string) (string, bool) {
	candidates := []string{}
	if explicitBinary != "" {
		candidates = append(candidates, explicitBinary)
	}
	candidates = append(candidates,
		filepath.Join(repoRoot, "psiphon-tunnel-core-x86_64"),
		filepath.Join(repoRoot, "archive", "psiphon-tunnel-core-x86_64"),
		filepath.Join(runtimeRoot, "bin", "psiphon-tunnel-core-x86_64"),
	)

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			_ = os.Chmod(candidate, 0o755)
			return candidate, true
		}
	}

	return "", false
}

func renderInstanceConfig(baseConfigPath, outputPath string, httpPort, socksPort int, remoteFilename, egressRegion string) error {
	content, err := os.ReadFile(baseConfigPath)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return err
	}
	raw["LocalHttpProxyPort"] = httpPort
	raw["LocalSocksProxyPort"] = socksPort
	raw["RemoteServerListDownloadFilename"] = remoteFilename
	raw["EgressRegion"] = egressRegion
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, append(encoded, '\n'), 0o644)
}

func isKnownRegion(region string) bool {
	for _, known := range strings.Split(DefaultRegions, ",") {
		if region == strings.TrimSpace(known) {
			return true
		}
	}
	return false
}
