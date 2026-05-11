package mg

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func extractRuntimeRoot(fallback string, args []string) string {
	runtimeRoot := fallback
	for i := 0; i < len(args); i++ {
		if args[i] == "--runtime-root" && i+1 < len(args) {
			runtimeRoot = args[i+1]
			i++
		}
	}
	return runtimeRoot
}

func stateFilePath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "active.env")
}

func removeStateFile(runtimeRoot string) error {
	if err := os.Remove(stateFilePath(runtimeRoot)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeStateFile(runtimeRoot string, state activeState) error {
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		return err
	}

	var builder strings.Builder
	writeStateLine(&builder, "ACTIVE_REGION", state.Region)
	writeStateLine(&builder, "ACTIVE_PID", strconv.Itoa(state.PID))
	writeStateLine(&builder, "ACTIVE_HTTP_PORT", strconv.Itoa(state.HTTPPort))
	writeStateLine(&builder, "ACTIVE_SOCKS_PORT", strconv.Itoa(state.SocksPort))
	writeStateLine(&builder, "ACTIVE_BINARY_PATH", state.BinaryPath)
	writeStateLine(&builder, "ACTIVE_BASE_CONFIG", state.BaseConfig)
	writeStateLine(&builder, "ACTIVE_DATA_DIR", state.DataDir)
	writeStateLine(&builder, "ACTIVE_DOWNLOAD_IF_MISSING", boolString(state.DownloadIfMissing))
	writeStateLine(&builder, "ACTIVE_DOWNLOAD_URL", state.DownloadURL)
	writeStateLine(&builder, "ACTIVE_NOTICES_PATH", state.NoticesPath)
	writeStateLine(&builder, "ACTIVE_READY_TIMEOUT_SECONDS", strconv.Itoa(state.ReadyTimeoutSeconds))
	writeStateLine(&builder, "ACTIVE_RUN_DIR", state.RunDir)
	writeStateLine(&builder, "ACTIVE_STARTED_AT", state.StartedAt)
	writeStateLine(&builder, "ACTIVE_STDERR_PATH", state.StderrPath)
	writeStateLine(&builder, "ACTIVE_STDOUT_PATH", state.StdoutPath)

	tmpPath := stateFilePath(runtimeRoot) + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(builder.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, stateFilePath(runtimeRoot))
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func writeStateLine(builder *strings.Builder, key, value string) {
	builder.WriteString(key)
	builder.WriteString("=")
	builder.WriteString(strconv.Quote(value))
	builder.WriteString("\n")
}

func (a *app) loadState(runtimeRoot string) (activeState, managerState) {
	path := stateFilePath(runtimeRoot)
	content, err := os.ReadFile(path)
	if err != nil {
		return activeState{}, stateNone
	}

	state := activeState{}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		value := parseStateValue(parts[1])
		switch key {
		case "ACTIVE_REGION":
			state.Region = value
		case "ACTIVE_PID":
			state.PID, _ = strconv.Atoi(value)
		case "ACTIVE_HTTP_PORT":
			state.HTTPPort, _ = strconv.Atoi(value)
		case "ACTIVE_SOCKS_PORT":
			state.SocksPort, _ = strconv.Atoi(value)
		case "ACTIVE_BINARY_PATH":
			state.BinaryPath = value
		case "ACTIVE_BASE_CONFIG":
			state.BaseConfig = value
		case "ACTIVE_DATA_DIR":
			state.DataDir = value
		case "ACTIVE_DOWNLOAD_IF_MISSING":
			state.DownloadIfMissing = value == "1" || strings.EqualFold(value, "true")
		case "ACTIVE_DOWNLOAD_URL":
			state.DownloadURL = value
		case "ACTIVE_NOTICES_PATH":
			state.NoticesPath = value
		case "ACTIVE_READY_TIMEOUT_SECONDS":
			state.ReadyTimeoutSeconds, _ = strconv.Atoi(value)
		case "ACTIVE_RUN_DIR":
			state.RunDir = value
		case "ACTIVE_STARTED_AT":
			state.StartedAt = value
		case "ACTIVE_STDERR_PATH":
			state.StderrPath = value
		case "ACTIVE_STDOUT_PATH":
			state.StdoutPath = value
		}
	}

	if trackedPIDMatchesState(state) {
		return state, stateRunning
	}
	return state, stateStale
}

func parseStateValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "\"") {
		if unquoted, err := strconv.Unquote(raw); err == nil {
			return unquoted
		}
	}
	if strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'") && len(raw) >= 2 {
		return strings.Trim(raw, "'")
	}
	return raw
}

func readTrimmedInt(path string) (int, error) {
	text, err := readTrimmedString(path)
	if err != nil {
		return 0, err
	}
	if text == "" {
		return 0, nil
	}
	return strconv.Atoi(text)
}

func readTrimmedString(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
