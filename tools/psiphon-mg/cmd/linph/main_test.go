package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHonorsExecutableAlias(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := run([]string{"pluninstaller", "--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run(pluninstaller --help) exit = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:\n  pluninstaller") {
		t.Fatalf("run(pluninstaller --help) stdout = %q", stdout.String())
	}
}
