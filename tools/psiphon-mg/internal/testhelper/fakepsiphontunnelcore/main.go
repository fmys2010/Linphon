package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const helperChildModeEnv = "FAKE_PSIPHON_CHILD_MODE"

type cliArgs struct {
	configPath  string
	dataRoot    string
	noticesPath string
	help        bool
}

type configData struct {
	HTTPPort       int
	SocksPort      int
	RemoteFilename string
	EgressRegion   string
}

func main() {
	if os.Getenv(helperChildModeEnv) == "1" {
		os.Exit(runHelperChild())
	}
	os.Exit(run())
}

func run() int {
	args, err := parseArgs(os.Args[1:])
	if err != nil {
		usage(os.Stderr)
		return 64
	}
	if args.help {
		usage(os.Stdout)
		return 0
	}
	if args.dataRoot == "" || args.noticesPath == "" {
		if _, err := readConfig(args.configPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to read config: %v\n", err)
			return 1
		}
		return 0
	}

	if err := os.MkdirAll(args.dataRoot, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create data root: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(args.noticesPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create notices directory: %v\n", err)
		return 1
	}

	config, err := readConfig(args.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read config: %v\n", err)
		return 1
	}
	remoteListPath := filepath.Join(args.dataRoot, config.RemoteFilename)
	if err := os.WriteFile(remoteListPath, []byte(fmt.Sprintf("fake remote list for %s\n", config.RemoteFilename)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write remote list: %v\n", err)
		return 1
	}

	runtimeInfo := fmt.Sprintf("config=%s\ndata_root=%s\nnotices=%s\negress_region=%s\n",
		args.configPath,
		args.dataRoot,
		args.noticesPath,
		config.EgressRegion,
	)
	if err := os.WriteFile(filepath.Join(args.dataRoot, "runtime.info"), []byte(runtimeInfo), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write runtime info: %v\n", err)
		return 1
	}

	if err := appendNotice(args.noticesPath, "ListeningHttpProxyPort", fmt.Sprintf("{\"port\":%d}", config.HTTPPort)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write http notice: %v\n", err)
		return 1
	}
	if err := appendNotice(args.noticesPath, "ListeningSocksProxyPort", fmt.Sprintf("{\"port\":%d}", config.SocksPort)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write socks notice: %v\n", err)
		return 1
	}
	if err := appendNotice(args.noticesPath, "Tunnels", `{"count":1,"state":"connected-for-test"}`); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write tunnels notice: %v\n", err)
		return 1
	}

	helperPIDFile := os.Getenv("FAKE_PSIPHON_HELPER_PID_FILE")
	if err := startHelperChild(helperPIDFile); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start helper child: %v\n", err)
		return 1
	}

	if delay := autoExitDelay(); delay >= 0 {
		time.Sleep(delay)
		return 0
	}

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	for range signals {
		shutdown(args.noticesPath, helperPIDFile)
		return 0
	}

	return 0
}

func autoExitDelay() time.Duration {
	raw := os.Getenv("FAKE_PSIPHON_AUTO_EXIT_DELAY_MS")
	if raw == "" {
		if os.Getenv("FAKE_PSIPHON_AUTO_EXIT") == "1" {
			return 1500 * time.Millisecond
		}
		return -1
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		return -1
	}
	return time.Duration(ms) * time.Millisecond
}

func parseArgs(argv []string) (cliArgs, error) {
	args := cliArgs{}
	for index := 0; index < len(argv); index++ {
		switch argv[index] {
		case "-config":
			if index+1 >= len(argv) {
				return cliArgs{}, fmt.Errorf("missing value for -config")
			}
			args.configPath = argv[index+1]
			index++
		case "-dataRootDirectory":
			if index+1 >= len(argv) {
				return cliArgs{}, fmt.Errorf("missing value for -dataRootDirectory")
			}
			args.dataRoot = argv[index+1]
			index++
		case "-notices":
			if index+1 >= len(argv) {
				return cliArgs{}, fmt.Errorf("missing value for -notices")
			}
			args.noticesPath = argv[index+1]
			index++
		case "--help", "-h":
			args.help = true
		default:
			fmt.Fprintf(os.Stderr, "[fake-psiphon] ignoring extra argument: %s\n", argv[index])
		}
	}

	if args.help {
		return args, nil
	}
	if args.configPath == "" {
		return cliArgs{}, fmt.Errorf("missing required flags")
	}
	return args, nil
}

func usage(file *os.File) {
	fmt.Fprintln(file, "Usage:")
	fmt.Fprintln(file, "  fake-psiphon-tunnel-core-x86_64 -config PATH -dataRootDirectory PATH -notices PATH")
}

func readConfig(configPath string) (configData, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return configData{}, err
	}

	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return configData{}, err
	}

	return configData{
		HTTPPort:       intFromAny(raw["LocalHttpProxyPort"]),
		SocksPort:      intFromAny(raw["LocalSocksProxyPort"]),
		RemoteFilename: stringFromAny(raw["RemoteServerListDownloadFilename"]),
		EgressRegion:   stringFromAny(raw["EgressRegion"]),
	}, nil
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func appendNotice(noticesPath, noticeType, payload string) error {
	file, err := os.OpenFile(noticesPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "{\"noticeType\":\"%s\",\"data\":%s}\n", noticeType, payload)
	return err
}

func shutdown(noticesPath, helperPIDFile string) {
	_ = appendNotice(noticesPath, "Exiting", `{"reason":"signal"}`)
	if helperPIDFile != "" {
		_ = os.Remove(helperPIDFile)
	}
}

func startHelperChild(helperPIDFile string) error {
	if helperPIDFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(helperPIDFile), 0o755); err != nil {
		return err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(executablePath)
	cmd.Env = append(os.Environ(), helperChildModeEnv+"=1")
	if err := cmd.Start(); err != nil {
		return err
	}

	return os.WriteFile(helperPIDFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644)
}

func runHelperChild() int {
	ignoreTerm := os.Getenv("FAKE_PSIPHON_HELPER_IGNORE_TERM") == "1"
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	for signalValue := range signals {
		if signalValue == os.Interrupt {
			return 0
		}
		if signalValue == syscall.SIGTERM && !ignoreTerm {
			return 0
		}
	}

	return 0
}
