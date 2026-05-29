package mg

import (
	"bufio"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	installedVGProfileVersion = 1
	defaultVPNGateAPIURL      = "https://www.vpngate.net/api/iphone/"
	installedVGRuntimeDirName = "vg"
	installedVGCacheFilename  = "vpngate.csv"
)

type installedVGProfile struct {
	Version              int      `json:"version"`
	Regions              []string `json:"regions"`
	OpenVPNBinaryPath    string   `json:"openvpn_binary_path"`
	APIURL               string   `json:"api_url"`
	CachePath            string   `json:"cache_path"`
	Refresh              bool     `json:"refresh"`
	AllowInsecureAPIURL  bool     `json:"allow_insecure_api_url"`
	AllowLocalAPIURL     bool     `json:"allow_local_api_url"`
	AllowUnsafeCachePath bool     `json:"allow_unsafe_cache_path"`
}

type installedVGSpec struct {
	Region      string
	RuntimeRoot string
}

type vpngateServer struct {
	HostName      string
	IP            string
	CountryLong   string
	CountryShort  string
	Score         int
	Throughput    int64
	OpenVPNConfig string
}

func (layout installLayout) installedVGRuntimeRoot() string {
	return filepath.Join(layout.installedRuntimeRoot(), installedVGRuntimeDirName)
}

func (layout installLayout) installedVGCachePath() string {
	return filepath.Join(layout.installedVGRuntimeRoot(), installedVGCacheFilename)
}

func defaultInstalledVGProfile(layout installLayout) installedVGProfile {
	return installedVGProfile{
		Version:           installedVGProfileVersion,
		Regions:           []string{"US"},
		OpenVPNBinaryPath: "openvpn",
		APIURL:            defaultVPNGateAPIURL,
		CachePath:         layout.installedVGCachePath(),
	}
}

func normalizeInstalledVGProfile(profile *installedVGProfile) {
	if profile.Version == 0 {
		profile.Version = installedVGProfileVersion
	}
	if len(profile.Regions) == 0 {
		profile.Regions = []string{"US"}
	} else {
		profile.Regions = normalizeInstalledRegions(strings.Join(profile.Regions, ","))
	}
	if strings.TrimSpace(profile.OpenVPNBinaryPath) == "" {
		profile.OpenVPNBinaryPath = "openvpn"
	}
	if strings.TrimSpace(profile.APIURL) == "" {
		profile.APIURL = defaultVPNGateAPIURL
	}
}

func validateInstalledVGProfile(profile installedVGProfile) (installedVGProfile, installedVGSpec, error) {
	normalizeInstalledVGProfile(&profile)
	if profile.Version != installedVGProfileVersion {
		return installedVGProfile{}, installedVGSpec{}, fmt.Errorf("unsupported vg profile version: %d", profile.Version)
	}
	if len(profile.Regions) == 0 {
		return installedVGProfile{}, installedVGSpec{}, fmt.Errorf("vg requires at least one region")
	}
	if strings.TrimSpace(profile.OpenVPNBinaryPath) == "" {
		return installedVGProfile{}, installedVGSpec{}, fmt.Errorf("vg openvpn binary is not set")
	}
	if strings.TrimSpace(profile.APIURL) == "" {
		return installedVGProfile{}, installedVGSpec{}, fmt.Errorf("vg API URL is not set")
	}
	if err := validateVPNGateAPIURL(profile); err != nil {
		return installedVGProfile{}, installedVGSpec{}, err
	}
	return profile, installedVGSpec{Region: profile.Regions[0]}, nil
}

func validateVPNGateAPIURL(profile installedVGProfile) error {
	parsed, err := url.Parse(profile.APIURL)
	if err != nil {
		return err
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if profile.AllowInsecureAPIURL {
			return nil
		}
		return fmt.Errorf("vg API URL uses insecure http; use https or pass --allow-insecure-api-url")
	case "file", "":
		if profile.AllowLocalAPIURL {
			return nil
		}
		return fmt.Errorf("vg API URL uses a local file path; pass --allow-local-api-url for offline fixtures")
	default:
		return fmt.Errorf("unsupported VPNGate API URL scheme: %s", parsed.Scheme)
	}
}

func installedVGProfileFromState(state installedProviderState) (installedVGProfile, error) {
	if err := normalizeInstalledProviderState(&state); err != nil {
		return installedVGProfile{}, err
	}
	if state.Providers.VG == nil {
		return installedVGProfile{}, fmt.Errorf("provider vg is missing provider state")
	}
	profile, _, err := validateInstalledVGProfile(*state.Providers.VG)
	return profile, err
}

func parseVPNGateServers(data []byte) ([]vpngateServer, error) {
	lines := strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n")
	csvLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		csvLines = append(csvLines, line)
	}
	if len(csvLines) == 0 {
		return nil, fmt.Errorf("vpngate CSV is empty")
	}

	reader := csv.NewReader(strings.NewReader(strings.Join(csvLines, "\n")))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("vpngate CSV has no server rows")
	}

	headerIndex := -1
	columns := map[string]int{}
	for index, record := range records {
		columns = vpngateHeaderColumns(record)
		if _, ok := columns["OpenVPN_ConfigData_Base64"]; ok {
			headerIndex = index
			break
		}
	}
	if headerIndex < 0 {
		return nil, fmt.Errorf("vpngate CSV missing OpenVPN config column")
	}

	servers := []vpngateServer{}
	for _, record := range records[headerIndex+1:] {
		configBase64 := vpngateField(record, columns, "OpenVPN_ConfigData_Base64")
		if configBase64 == "" {
			continue
		}
		configData, err := base64.StdEncoding.DecodeString(configBase64)
		if err != nil {
			continue
		}
		server := vpngateServer{
			HostName:      vpngateField(record, columns, "HostName"),
			IP:            vpngateField(record, columns, "IP"),
			CountryLong:   vpngateField(record, columns, "CountryLong"),
			CountryShort:  strings.ToUpper(vpngateField(record, columns, "CountryShort")),
			Score:         parseIntDefault(vpngateField(record, columns, "Score"), 0),
			Throughput:    int64(parseIntDefault(vpngateField(record, columns, "Throughput"), 0)),
			OpenVPNConfig: string(configData),
		}
		if server.CountryShort == "" || server.OpenVPNConfig == "" {
			continue
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("vpngate CSV has no usable OpenVPN servers")
	}
	return servers, nil
}

func vpngateHeaderColumns(header []string) map[string]int {
	columns := map[string]int{}
	for index, name := range header {
		columns[strings.TrimSpace(name)] = index
	}
	return columns
}

func vpngateField(record []string, columns map[string]int, name string) string {
	index, ok := columns[name]
	if !ok || index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func parseIntDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func selectVPNGateServer(servers []vpngateServer, regions []string) (vpngateServer, error) {
	preferredRegions := make([]string, 0, len(regions))
	for _, region := range regions {
		if region = strings.ToUpper(strings.TrimSpace(region)); region != "" {
			preferredRegions = append(preferredRegions, region)
		}
	}
	for _, region := range preferredRegions {
		filtered := make([]vpngateServer, 0, len(servers))
		for _, server := range servers {
			if server.CountryShort == region {
				filtered = append(filtered, server)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].Score != filtered[j].Score {
				return filtered[i].Score > filtered[j].Score
			}
			return filtered[i].Throughput > filtered[j].Throughput
		})
		return filtered[0], nil
	}
	return vpngateServer{}, fmt.Errorf("no VPNGate OpenVPN servers found for regions %s", strings.Join(regions, ","))
}

func readVPNGateCSV(layout installLayout, profile installedVGProfile) ([]byte, error) {
	if err := validateVPNGateAPIURL(profile); err != nil {
		return nil, err
	}
	cachePath, err := resolveInstalledVGCachePath(layout, profile)
	if err != nil {
		return nil, err
	}
	if !profile.Refresh && cachePath != "" {
		data, err := os.ReadFile(cachePath)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}

	data, err := fetchVPNGateCSV(profile.APIURL)
	if err != nil {
		return nil, err
	}
	if cachePath != "" {
		if writeErr := copyBytesAtomic(data, cachePath, 0o600); writeErr != nil {
			return nil, writeErr
		}
	}
	return data, nil
}

func fetchVPNGateCSV(rawURL string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	switch parsed.Scheme {
	case "file":
		return os.ReadFile(parsed.Path)
	case "http", "https":
		client := &http.Client{Timeout: 20 * time.Second}
		response, err := client.Get(rawURL)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("VPNGate API returned %s", response.Status)
		}
		return io.ReadAll(response.Body)
	case "":
		return os.ReadFile(rawURL)
	default:
		return nil, fmt.Errorf("unsupported VPNGate API URL scheme: %s", parsed.Scheme)
	}
}

func validateOpenVPNConfig(config string) error {
	unsafeDirectives := map[string]struct{}{
		"askpass":                    {},
		"auth-user-pass":             {},
		"ca":                         {},
		"capath":                     {},
		"cd":                         {},
		"cert":                       {},
		"chroot":                     {},
		"client-connect":             {},
		"client-disconnect":          {},
		"crl-verify":                 {},
		"daemon":                     {},
		"down":                       {},
		"down-pre":                   {},
		"http-proxy-user-pass":       {},
		"ipchange":                   {},
		"key":                        {},
		"learn-address":              {},
		"log":                        {},
		"log-append":                 {},
		"management":                 {},
		"management-client":          {},
		"management-client-auth":     {},
		"management-external-key":    {},
		"management-hold":            {},
		"management-query-passwords": {},
		"management-query-proxy":     {},
		"management-signal":          {},
		"management-up-down":         {},
		"pkcs12":                     {},
		"plugin":                     {},
		"route-pre-down":             {},
		"route-up":                   {},
		"script-security":            {},
		"secret":                     {},
		"setenv":                     {},
		"status":                     {},
		"status-version":             {},
		"tmp-dir":                    {},
		"tls-auth":                   {},
		"tls-crypt":                  {},
		"tls-crypt-v2":               {},
		"tls-verify":                 {},
		"up":                         {},
		"writepid":                   {},
	}
	scanner := bufio.NewScanner(strings.NewReader(config))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		directive := strings.ToLower(strings.TrimLeft(fields[0], "-"))
		if _, unsafe := unsafeDirectives[directive]; unsafe {
			return fmt.Errorf("unsafe OpenVPN directive %q on line %d", fields[0], lineNumber)
		}
	}
	return scanner.Err()
}

func resolveOpenVPNBinaryPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "openvpn"
	}
	if !filepath.IsAbs(path) {
		if strings.ContainsRune(path, os.PathSeparator) {
			return "", fmt.Errorf("OpenVPN executable path must be absolute: %s", path)
		}
		resolved, err := exec.LookPath(path)
		if err != nil {
			return "", err
		}
		path = resolved
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("OpenVPN executable must not be a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("OpenVPN executable must be a regular file: %s", path)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("OpenVPN executable is not executable: %s", path)
	}
	if info.Mode()&0o022 != 0 {
		return "", fmt.Errorf("OpenVPN executable must not be group/world-writable: %s", path)
	}
	return path, nil
}
