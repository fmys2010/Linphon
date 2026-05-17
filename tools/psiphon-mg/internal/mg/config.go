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

func readDefaultRegion(repoRoot, configPath string) string {
	content, err := os.ReadFile(configPath)
	if err == nil {
		var raw map[string]any
		if jsonErr := json.Unmarshal(content, &raw); jsonErr == nil {
			if region := strings.TrimSpace(stringFromAny(raw["EgressRegion"])); region != "" {
				return region
			}
		}
	}

	for _, region := range strings.Split(defaultRegionsCSV(repoRoot), ",") {
		if trimmed := strings.TrimSpace(region); trimmed != "" {
			return trimmed
		}
	}

	return "US"
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

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
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
		info, err := os.Lstat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
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
