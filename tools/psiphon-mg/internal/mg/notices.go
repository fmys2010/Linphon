package mg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

func noticeFlag(noticesPath, noticeType string) string {
	if noticesPath == "" {
		return "no"
	}
	file, err := os.Open(noticesPath)
	if err != nil {
		return "no"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var notice tunnelNotice
		if err := json.Unmarshal(scanner.Bytes(), &notice); err != nil {
			continue
		}
		if notice.NoticeType == noticeType {
			return "yes"
		}
	}
	return "no"
}

func tunnelsReady(noticesPath string) bool {
	if noticesPath == "" {
		return false
	}
	file, err := os.Open(noticesPath)
	if err != nil {
		return false
	}
	defer file.Close()

	latest := -1
	seen := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var notice tunnelNotice
		if err := json.Unmarshal(scanner.Bytes(), &notice); err != nil {
			continue
		}
		if notice.NoticeType == "Tunnels" {
			seen = true
			latest = notice.Data.Count
		}
	}
	return seen && latest > 0
}

func tunnelsReadyFlag(noticesPath string) string {
	if tunnelsReady(noticesPath) {
		return "yes"
	}
	return "no"
}

func formatOptionalInt(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func (a *app) printStatus(runtimeRoot string, stateKind managerState, state activeState) {
	displayState := string(stateKind)
	if stateKind == stateNone {
		displayState = "stopped"
	}

	httpNotice := "no"
	socksNotice := "no"
	tunnelsNotice := "no"
	if state.NoticesPath != "" {
		httpNotice = noticeFlag(state.NoticesPath, "ListeningHttpProxyPort")
		socksNotice = noticeFlag(state.NoticesPath, "ListeningSocksProxyPort")
		tunnelsNotice = tunnelsReadyFlag(state.NoticesPath)
	}

	fmt.Fprintf(a.stdout, "runtime_root=%s\n", runtimeRoot)
	fmt.Fprintf(a.stdout, "state=%s\n", displayState)
	if displayState == "running" {
		fmt.Fprintln(a.stdout, "running=yes")
	} else {
		fmt.Fprintln(a.stdout, "running=no")
	}
	fmt.Fprintf(a.stdout, "region=%s\n", state.Region)
	fmt.Fprintf(a.stdout, "pid=%s\n", formatOptionalInt(state.PID))
	fmt.Fprintf(a.stdout, "http_port=%s\n", formatOptionalInt(state.HTTPPort))
	fmt.Fprintf(a.stdout, "socks_port=%s\n", formatOptionalInt(state.SocksPort))
	fmt.Fprintf(a.stdout, "http_notice=%s\n", httpNotice)
	fmt.Fprintf(a.stdout, "socks_notice=%s\n", socksNotice)
	fmt.Fprintf(a.stdout, "tunnels_notice=%s\n", tunnelsNotice)
	fmt.Fprintf(a.stdout, "notices_path=%s\n", state.NoticesPath)
	fmt.Fprintf(a.stdout, "run_dir=%s\n", state.RunDir)
	fmt.Fprintf(a.stdout, "stdout_path=%s\n", state.StdoutPath)
	fmt.Fprintf(a.stdout, "stderr_path=%s\n", state.StderrPath)
	fmt.Fprintln(a.stdout, "connectivity=unknown")
}
