package mg

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type harnessApp struct {
	stdout   io.Writer
	stderr   io.Writer
	repoRoot string
}

type harnessOptions struct {
	BinaryPath          string
	DownloadIfMissing   bool
	DownloadURL         string
	DownloadURLProvided bool
	BaseConfig          string
	RuntimeRoot         string
	RunName             string
	Count               int
	RegionsCSV          string
	HTTPPortBase        int
	SocksPortBase       int
	WaitSeconds         int
	StartupGraceSeconds int
	KeepRunning         bool
}

type harnessInstance struct {
	InstanceID  string
	Dir         string
	ConfigPath  string
	DataDir     string
	NoticesPath string
	StdoutPath  string
	StderrPath  string
	PIDPath     string
	Region      string
	HTTPPort    int
	SocksPort   int
	PID         int
	Command     *exec.Cmd
}

type metricsSnapshot struct {
	PPID    string
	State   string
	RSSKB   string
	VSZKB   string
	Command string
}

func RunMultiInstance(args []string, stdout, stderr io.Writer) int {
	app := &harnessApp{
		stdout:   stdout,
		stderr:   stderr,
		repoRoot: resolveRepoRoot(),
	}
	return app.run(args)
}

func (a *harnessApp) run(args []string) int {
	if len(args) == 0 {
		a.usage(a.stderr)
		return ExitUsage
	}

	switch args[0] {
	case "locate-binary":
		return a.commandLocateBinary(args[1:])
	case "download-binary":
		return a.commandDownloadBinary(args[1:])
	case "run":
		return a.commandRun(args[1:])
	case "--help", "-h", "help":
		a.usage(a.stdout)
		return 0
	default:
		a.err("unknown command: %s", args[0])
		a.usage(a.stderr)
		return ExitUsage
	}
}

func (a *harnessApp) usage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  psiphon-multi-instance locate-binary [--binary PATH] [--runtime-root PATH]
  psiphon-multi-instance download-binary [--output PATH] [--url URL]
  psiphon-multi-instance run [options]

Commands:
  locate-binary     Resolve a repo-local psiphon-tunnel-core-x86_64 path.
  download-binary   Disabled until executable authenticity verification exists.
  run               Generate isolated configs and launch N instances.

Run options:
  --binary PATH                 Explicit binary path.
  --download-if-missing         Disabled until executable authenticity verification exists.
  --download-url URL            Disabled until executable authenticity verification exists.
  --base-config PATH            Base config template (default: ./psiphon.config).
  --runtime-root PATH           Runtime root (default: ./.work/psiphon-harness).
  --run-name NAME               Stable run directory name under runtime root.
  --count N                     Number of instances to launch (default: 1).
  --regions CSV                 Comma-separated EgressRegion values.
  --http-port-base N            First LocalHttpProxyPort (default: 18080).
  --socks-port-base N           First LocalSocksProxyPort (default: 11080).
  --wait-seconds N              Seconds to wait before final metrics (default: 5).
  --startup-grace-seconds N     Seconds to allow processes to initialize (default: 2).
  --keep-running                Leave processes running on exit.
  --help                        Show this message.

Artifacts:
  runtime-root/
    bin/
    runs/<run-name>/
      instances/instance-XXX/{config.json,data/,notices.jsonl,stdout.log,stderr.log,pid}
      summary.tsv
      metrics-start.tsv
      metrics-final.tsv
      cgroup-start.snapshot
      cgroup-final.snapshot
`)
}

func (a *harnessApp) commandLocateBinary(args []string) int {
	runtimeRoot := filepath.Join(a.repoRoot, ".work", "psiphon-harness")
	binaryPath := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--binary":
			if i+1 >= len(args) {
				a.err("--binary requires a value")
				return ExitUsage
			}
			binaryPath = args[i+1]
			i++
		case "--runtime-root":
			if i+1 >= len(args) {
				a.err("--runtime-root requires a value")
				return ExitUsage
			}
			runtimeRoot = args[i+1]
			i++
		case "--help":
			a.usage(a.stdout)
			return 0
		default:
			a.err("unknown locate-binary option: %s", args[i])
			return ExitUsage
		}
	}

	if resolved, ok := resolveBinary(a.repoRoot, binaryPath, runtimeRoot); ok {
		fmt.Fprintln(a.stdout, resolved)
		return 0
	}

	a.err("unable to locate psiphon-tunnel-core-x86_64")
	return ExitBinaryNotFound
}

func (a *harnessApp) commandDownloadBinary(args []string) int {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "--url":
			if i+1 >= len(args) {
				a.err("%s requires a value", args[i])
				return ExitUsage
			}
			i++
		case "--help":
			a.usage(a.stdout)
			return 0
		default:
			a.err("unknown download-binary option: %s", args[i])
			return ExitUsage
		}
	}

	a.downloadDisabled()
	return ExitDownloadFailed
}

func (a *harnessApp) commandRun(args []string) int {
	opt := harnessOptions{
		DownloadURL:         DefaultBinaryDownloadURL,
		BaseConfig:          filepath.Join(a.repoRoot, "psiphon.config"),
		RuntimeRoot:         filepath.Join(a.repoRoot, ".work", "psiphon-harness"),
		Count:               1,
		HTTPPortBase:        18080,
		SocksPortBase:       11080,
		WaitSeconds:         5,
		StartupGraceSeconds: 2,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--binary":
			if i+1 >= len(args) {
				a.err("--binary requires a value")
				return ExitUsage
			}
			opt.BinaryPath = args[i+1]
			i++
		case "--download-if-missing":
			opt.DownloadIfMissing = true
		case "--download-url":
			if i+1 >= len(args) {
				a.err("--download-url requires a value")
				return ExitUsage
			}
			opt.DownloadURL = args[i+1]
			opt.DownloadURLProvided = true
			i++
		case "--base-config":
			if i+1 >= len(args) {
				a.err("--base-config requires a value")
				return ExitUsage
			}
			opt.BaseConfig = args[i+1]
			i++
		case "--runtime-root":
			if i+1 >= len(args) {
				a.err("--runtime-root requires a value")
				return ExitUsage
			}
			opt.RuntimeRoot = args[i+1]
			i++
		case "--run-name":
			if i+1 >= len(args) {
				a.err("--run-name requires a value")
				return ExitUsage
			}
			opt.RunName = args[i+1]
			i++
		case "--count":
			if i+1 >= len(args) {
				a.err("--count requires a value")
				return ExitUsage
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value <= 0 {
				a.err("instance count must be greater than zero: %s", args[i+1])
				return ExitUsage
			}
			opt.Count = value
			i++
		case "--regions":
			if i+1 >= len(args) {
				a.err("--regions requires a value")
				return ExitUsage
			}
			opt.RegionsCSV = args[i+1]
			i++
		case "--http-port-base":
			if i+1 >= len(args) {
				a.err("--http-port-base requires a value")
				return ExitUsage
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value <= 0 {
				a.err("HTTP port base must be greater than zero: %s", args[i+1])
				return ExitUsage
			}
			opt.HTTPPortBase = value
			i++
		case "--socks-port-base":
			if i+1 >= len(args) {
				a.err("--socks-port-base requires a value")
				return ExitUsage
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value <= 0 {
				a.err("SOCKS port base must be greater than zero: %s", args[i+1])
				return ExitUsage
			}
			opt.SocksPortBase = value
			i++
		case "--wait-seconds":
			if i+1 >= len(args) {
				a.err("--wait-seconds requires a value")
				return ExitUsage
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value < 0 {
				a.err("wait seconds must be a non-negative integer: %s", args[i+1])
				return ExitUsage
			}
			opt.WaitSeconds = value
			i++
		case "--startup-grace-seconds":
			if i+1 >= len(args) {
				a.err("--startup-grace-seconds requires a value")
				return ExitUsage
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value < 0 {
				a.err("startup grace seconds must be a non-negative integer: %s", args[i+1])
				return ExitUsage
			}
			opt.StartupGraceSeconds = value
			i++
		case "--keep-running":
			opt.KeepRunning = true
		case "--help":
			a.usage(a.stdout)
			return 0
		default:
			a.err("unknown run option: %s", args[i])
			a.usage(a.stderr)
			return ExitUsage
		}
	}

	if _, err := os.Stat(opt.BaseConfig); err != nil {
		a.err("base config not found: %s", opt.BaseConfig)
		return ExitUsage
	}
	if opt.DownloadIfMissing || opt.DownloadURLProvided {
		a.downloadDisabled()
		return ExitDownloadFailed
	}

	binaryPath, ok := resolveBinary(a.repoRoot, opt.BinaryPath, opt.RuntimeRoot)
	if !ok {
		a.err("unable to locate psiphon-tunnel-core-x86_64")
		return ExitBinaryNotFound
	}

	if err := os.MkdirAll(filepath.Join(opt.RuntimeRoot, "runs"), 0o755); err != nil {
		a.err("failed to create runtime root: %v", err)
		return ExitValidationFailed
	}

	runName := opt.RunName
	if runName == "" {
		runName = fmt.Sprintf("run-%s-%di", time.Now().Format("20060102-150405"), opt.Count)
	}

	runDir := filepath.Join(opt.RuntimeRoot, "runs", runName)
	if _, err := os.Stat(runDir); err == nil {
		a.err("run directory already exists: %s", runDir)
		return ExitValidationFailed
	}

	regions, regionErr := buildHarnessRegionList(a.repoRoot, opt.Count, opt.RegionsCSV)
	if regionErr != nil {
		a.err(regionErr.Error())
		return ExitUsage
	}

	instancesDir := filepath.Join(runDir, "instances")
	summaryPath := filepath.Join(runDir, "summary.tsv")
	metricsStartPath := filepath.Join(runDir, "metrics-start.tsv")
	metricsFinalPath := filepath.Join(runDir, "metrics-final.tsv")
	cgroupStartPath := filepath.Join(runDir, "cgroup-start.snapshot")
	cgroupFinalPath := filepath.Join(runDir, "cgroup-final.snapshot")
	manifestPath := filepath.Join(runDir, "run.env")
	regionsPath := filepath.Join(runDir, "regions.txt")

	if err := os.MkdirAll(instancesDir, 0o755); err != nil {
		a.err("failed to create run directory: %v", err)
		return ExitValidationFailed
	}
	if err := os.WriteFile(regionsPath, []byte(strings.Join(regions, "\n")+"\n"), 0o644); err != nil {
		a.err("failed to write region list: %v", err)
		return ExitValidationFailed
	}
	manifest := strings.Join([]string{
		"RUN_DIR=" + runDir,
		"SUMMARY_PATH=" + summaryPath,
		"METRICS_START_PATH=" + metricsStartPath,
		"METRICS_FINAL_PATH=" + metricsFinalPath,
		"CGROUP_START_PATH=" + cgroupStartPath,
		"CGROUP_FINAL_PATH=" + cgroupFinalPath,
		"BINARY_PATH=" + binaryPath,
		"BASE_CONFIG=" + opt.BaseConfig,
		"INSTANCE_COUNT=" + strconv.Itoa(opt.Count),
		"REGIONS=" + strings.Join(regions, ","),
	}, "\n") + "\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		a.err("failed to write run manifest: %v", err)
		return ExitValidationFailed
	}
	if err := os.WriteFile(summaryPath, []byte("instance\tpid\tregion\thttp_port\tsocks_port\trunning_after_startup\thttp_notice\tsocks_notice\ttunnels_notice\tconfig_path\tdata_dir\tnotices_path\tstdout_path\tstderr_path\n"), 0o644); err != nil {
		a.err("failed to create summary: %v", err)
		return ExitValidationFailed
	}

	a.log("starting %d instance(s) using %s", opt.Count, binaryPath)
	a.log("artifacts will be written to %s", runDir)

	instances := make([]harnessInstance, 0, opt.Count)
	defer func() {
		cleanupHarnessProcesses(opt.KeepRunning, instances)
	}()

	for index := 0; index < opt.Count; index++ {
		instanceID := fmt.Sprintf("instance-%03d", index+1)
		instanceDir := filepath.Join(instancesDir, instanceID)
		dataDir := filepath.Join(instanceDir, "data")
		configPath := filepath.Join(instanceDir, "config.json")
		noticesPath := filepath.Join(instanceDir, "notices.jsonl")
		stdoutPath := filepath.Join(instanceDir, "stdout.log")
		stderrPath := filepath.Join(instanceDir, "stderr.log")
		pidPath := filepath.Join(instanceDir, "pid")
		httpPort := opt.HTTPPortBase + index
		socksPort := opt.SocksPortBase + index
		remoteFilename := "remote_server_list_" + instanceID
		region := regions[index]

		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			a.err("failed to create instance directory: %v", err)
			return ExitValidationFailed
		}
		if err := renderInstanceConfig(opt.BaseConfig, configPath, httpPort, socksPort, remoteFilename, region); err != nil {
			a.err("failed to render instance config: %v", err)
			return ExitValidationFailed
		}

		stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			a.err("failed to open stdout log: %v", err)
			return ExitValidationFailed
		}
		stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			stdoutFile.Close()
			a.err("failed to open stderr log: %v", err)
			return ExitValidationFailed
		}

		cmd := exec.Command(binaryPath,
			"-config", configPath,
			"-dataRootDirectory", dataDir,
			"-notices", noticesPath,
		)
		cmd.Stdout = stdoutFile
		cmd.Stderr = stderrFile
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			stdoutFile.Close()
			stderrFile.Close()
			a.err("failed to launch %s: %v", instanceID, err)
			return ExitInstanceFailed
		}
		_ = stdoutFile.Close()
		_ = stderrFile.Close()

		instance := harnessInstance{
			InstanceID:  instanceID,
			Dir:         instanceDir,
			ConfigPath:  configPath,
			DataDir:     dataDir,
			NoticesPath: noticesPath,
			StdoutPath:  stdoutPath,
			StderrPath:  stderrPath,
			PIDPath:     pidPath,
			Region:      region,
			HTTPPort:    httpPort,
			SocksPort:   socksPort,
			PID:         cmd.Process.Pid,
			Command:     cmd,
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(instance.PID)+"\n"), 0o644); err != nil {
			a.err("failed to persist pid: %v", err)
			return ExitValidationFailed
		}
		instances = append(instances, instance)
	}

	time.Sleep(time.Duration(opt.StartupGraceSeconds) * time.Second)
	if err := snapshotCGroupState(cgroupStartPath); err != nil {
		a.err("failed to write start cgroup snapshot: %v", err)
		return ExitValidationFailed
	}
	if err := collectHarnessMetrics(metricsStartPath, instances); err != nil {
		a.err("failed to write start metrics: %v", err)
		return ExitValidationFailed
	}

	aliveCount := 0
	summaryFile, err := os.OpenFile(summaryPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		a.err("failed to open summary: %v", err)
		return ExitValidationFailed
	}
	for _, instance := range instances {
		running := "no"
		if signalableProcess(instance.PID) {
			running = "yes"
			aliveCount++
		}
		_, err := fmt.Fprintf(summaryFile, "%s\t%d\t%s\t%d\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			instance.InstanceID,
			instance.PID,
			instance.Region,
			instance.HTTPPort,
			instance.SocksPort,
			running,
			noticeFlag(instance.NoticesPath, "ListeningHttpProxyPort"),
			noticeFlag(instance.NoticesPath, "ListeningSocksProxyPort"),
			noticeFlag(instance.NoticesPath, "Tunnels"),
			instance.ConfigPath,
			instance.DataDir,
			instance.NoticesPath,
			instance.StdoutPath,
			instance.StderrPath,
		)
		if err != nil {
			_ = summaryFile.Close()
			a.err("failed to append summary: %v", err)
			return ExitValidationFailed
		}
	}
	_ = summaryFile.Close()

	if opt.WaitSeconds > 0 {
		time.Sleep(time.Duration(opt.WaitSeconds) * time.Second)
	}
	if err := snapshotCGroupState(cgroupFinalPath); err != nil {
		a.err("failed to write final cgroup snapshot: %v", err)
		return ExitValidationFailed
	}
	if err := collectHarnessMetrics(metricsFinalPath, instances); err != nil {
		a.err("failed to write final metrics: %v", err)
		return ExitValidationFailed
	}

	a.log("summary: %s", summaryPath)
	a.log("metrics: %s", metricsFinalPath)
	a.log("cgroup snapshots: %s %s", cgroupStartPath, cgroupFinalPath)

	if aliveCount != opt.Count {
		a.err("%d of %d instance(s) remained alive through startup", aliveCount, opt.Count)
		return ExitInstanceFailed
	}

	a.log("all %d instance(s) remained alive through startup", opt.Count)
	a.log("network/tunnel success is reported separately via notices and is not required for harness success")
	return 0
}

func buildHarnessRegionList(repoRoot string, count int, overrideCSV string) ([]string, error) {
	sourceCSV := defaultRegionsCSV(repoRoot)
	if strings.TrimSpace(overrideCSV) != "" {
		sourceCSV = overrideCSV
	}

	selected := make([]string, 0, count)
	for _, raw := range strings.Split(sourceCSV, ",") {
		region := strings.TrimSpace(raw)
		if region == "" {
			continue
		}
		if !isKnownRegion(repoRoot, region) {
			return nil, fmt.Errorf("unknown region code: %s", region)
		}
		selected = append(selected, region)
		if len(selected) == count {
			break
		}
	}

	if len(selected) != count {
		return nil, fmt.Errorf("need at least %d region value(s); only found %d", count, len(selected))
	}
	return selected, nil
}

func cleanupHarnessProcesses(keepRunning bool, instances []harnessInstance) {
	if keepRunning {
		return
	}
	for _, instance := range instances {
		if instance.PID > 0 {
			stopPID(instance.PID, DefaultStopTimeout)
		}
	}
	for _, instance := range instances {
		if instance.Command != nil {
			_ = instance.Command.Wait()
		}
	}
}

func signalableProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func snapshotCGroupState(outputPath string) error {
	type probe struct {
		path  string
		label string
	}
	probes := []probe{
		{path: "/sys/fs/cgroup/memory.current", label: "memory.current"},
		{path: "/sys/fs/cgroup/memory.max", label: "memory.max"},
		{path: "/sys/fs/cgroup/pids.current", label: "pids.current"},
		{path: "/sys/fs/cgroup/pids.max", label: "pids.max"},
		{path: "/sys/fs/cgroup/memory/memory.usage_in_bytes", label: "memory.usage_in_bytes"},
		{path: "/sys/fs/cgroup/memory/memory.limit_in_bytes", label: "memory.limit_in_bytes"},
		{path: "/sys/fs/cgroup/pids/pids.current", label: "pids.current.v1"},
		{path: "/sys/fs/cgroup/pids/pids.max", label: "pids.max.v1"},
	}

	var builder strings.Builder
	builder.WriteString("timestamp\t")
	builder.WriteString(time.Now().UTC().Format(time.RFC3339))
	builder.WriteString("\n")
	builder.WriteString("dockerenv_present\t")
	if _, err := os.Stat("/.dockerenv"); err == nil {
		builder.WriteString("yes\n")
	} else {
		builder.WriteString("no\n")
	}

	for _, path := range []string{"/proc/1/cgroup", "/proc/self/cgroup", "/proc/1/cpuset"} {
		if content, err := os.ReadFile(path); err == nil {
			builder.WriteString(filepath.Base(path))
			builder.WriteString("\t")
			builder.WriteString(strings.ReplaceAll(strings.TrimRight(string(content), "\n"), "\n", ";"))
			builder.WriteString("\n")
		}
	}

	for _, probe := range probes {
		builder.WriteString(probe.label)
		builder.WriteString("\t")
		content, err := os.ReadFile(probe.path)
		if err != nil {
			builder.WriteString("unavailable\n")
			continue
		}
		builder.WriteString(strings.TrimSpace(string(content)))
		builder.WriteString("\n")
	}

	return os.WriteFile(outputPath, []byte(builder.String()), 0o644)
}

func collectHarnessMetrics(outputPath string, instances []harnessInstance) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, "pid\tppid\tstate\tcpu_percent\trss_kb\tvsz_kb\telapsed\tcommand"); err != nil {
		return err
	}
	for _, instance := range instances {
		if !signalableProcess(instance.PID) {
			if _, err := fmt.Fprintf(file, "%d\t-\texited\t0\t0\t0\t-\t-\n", instance.PID); err != nil {
				return err
			}
			continue
		}
		snapshot := readMetricsSnapshot(instance.PID)
		if _, err := fmt.Fprintf(file, "%d\t%s\t%s\t0\t%s\t%s\t-\t%s\n",
			instance.PID,
			snapshot.PPID,
			snapshot.State,
			snapshot.RSSKB,
			snapshot.VSZKB,
			snapshot.Command,
		); err != nil {
			return err
		}
	}
	return nil
}

func readMetricsSnapshot(pid int) metricsSnapshot {
	snapshot := metricsSnapshot{PPID: "-", State: "-", RSSKB: "0", VSZKB: "0", Command: "-"}
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err == nil {
		closeIndex := bytes.LastIndexByte(content, ')')
		if closeIndex >= 0 && closeIndex+2 < len(content) {
			fields := strings.Fields(string(content[closeIndex+2:]))
			if len(fields) >= 2 {
				snapshot.State = fields[0]
				snapshot.PPID = fields[1]
			}
		}
	}

	statusFile, err := os.Open(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err == nil {
		defer statusFile.Close()
		scanner := bufio.NewScanner(statusFile)
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "VmRSS:"):
				snapshot.RSSKB = firstNumericField(line)
			case strings.HasPrefix(line, "VmSize:"):
				snapshot.VSZKB = firstNumericField(line)
			}
		}
	}

	argv := processArgv(pid)
	if len(argv) > 0 {
		snapshot.Command = strings.Join(argv, " ")
	}
	return snapshot
}

func firstNumericField(line string) string {
	for _, field := range strings.Fields(line) {
		if _, err := strconv.Atoi(field); err == nil {
			return field
		}
	}
	return "0"
}

func (a *harnessApp) log(format string, args ...any) {
	fmt.Fprintf(a.stdout, "[harness] %s\n", fmt.Sprintf(format, args...))
}

func (a *harnessApp) err(format string, args ...any) {
	fmt.Fprintf(a.stderr, "[harness] ERROR: %s\n", fmt.Sprintf(format, args...))
}

func (a *harnessApp) downloadDisabled() {
	a.err("automatic download is disabled until executable authenticity verification exists")
}
