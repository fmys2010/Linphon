package mg

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunLinphUsageWithoutArgs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinph(nil, &stdout, &stderr)
	if exitCode != ExitUsage {
		t.Fatalf("RunLinph() exit = %d, want %d", exitCode, ExitUsage)
	}
	if !strings.Contains(stderr.String(), "linph install") {
		t.Fatalf("RunLinph() stderr = %q, want top-level usage", stderr.String())
	}
}

func TestRunLinphRoutesHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinph([]string{"help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("RunLinph(help) exit = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "Compatibility aliases") {
		t.Fatalf("RunLinph(help) stdout = %q, want compatibility section", stdout.String())
	}
	if !strings.Contains(stdout.String(), "linph start") {
		t.Fatalf("RunLinph(help) stdout = %q, want installed control commands", stdout.String())
	}
}

func TestRunLinphRoutesMgHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinph([]string{"mg", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("RunLinph(mg --help) exit = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "linph mg start REGION") {
		t.Fatalf("RunLinph(mg --help) stdout = %q, want linph mg usage", stdout.String())
	}
}

func TestRunLinphRoutesMultiHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinph([]string{"multi", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("RunLinph(multi --help) exit = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "linph multi locate-binary") {
		t.Fatalf("RunLinph(multi --help) stdout = %q, want linph multi usage", stdout.String())
	}
}

func TestRunLinphRoutesStagedHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinph([]string{"staged", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("RunLinph(staged --help) exit = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "linph staged") {
		t.Fatalf("RunLinph(staged --help) stdout = %q, want linph staged usage", stdout.String())
	}
}

func TestRunLinphRoutesInstalledControlHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinph([]string{"switch-port", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("RunLinph(switch-port --help) exit = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "linph switch-ctry REGION1,REGION2,...") {
		t.Fatalf("RunLinph(switch-port --help) stdout = %q, want installed control usage", stdout.String())
	}
}

func TestRunLinphAliasHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinphAlias("psiphon", []string{"--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("RunLinphAlias(psiphon --help) exit = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "Usage:\n  psiphon") {
		t.Fatalf("RunLinphAlias(psiphon --help) stdout = %q, want psiphon usage", stdout.String())
	}
}

func TestRunLinphRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunLinph([]string{"wat"}, &stdout, &stderr)
	if exitCode != ExitUsage {
		t.Fatalf("RunLinph(wat) exit = %d, want %d", exitCode, ExitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown linph command") {
		t.Fatalf("RunLinph(wat) stderr = %q, want unknown command error", stderr.String())
	}
}
