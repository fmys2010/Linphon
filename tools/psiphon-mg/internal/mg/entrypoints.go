package mg

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"syscall"
)

var (
	installedPsiphonBinaryPath = "/etc/psiphon/psiphon-tunnel-core-x86_64"
	installedPsiphonConfigPath = "/etc/psiphon/psiphon.config"
	installedPsiphonConfigDir  = "/etc/psiphon"
	installedPsiphonLauncher   = "/usr/bin/psiphon"
)

func RunPsiphon(stdout, stderr io.Writer) int {
	cmd := exec.Command(installedPsiphonBinaryPath, "-config", installedPsiphonConfigPath)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	return commandExitCode(cmd.Run())
}

func RunPluninstaller(stdout, stderr io.Writer) int {
	if err := os.RemoveAll(installedPsiphonConfigDir); err != nil {
		fmt.Fprintf(stderr, "failed to remove %s: %v\n", installedPsiphonConfigDir, err)
		return 1
	}
	if err := os.Remove(installedPsiphonLauncher); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "failed to remove %s: %v\n", installedPsiphonLauncher, err)
		return 1
	}
	return 0
}

func RunPlinstaller2(stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "Automatic remote download/install is disabled until executable authenticity verification exists.")
	fmt.Fprintln(stderr, "Use a reviewed local artifact flow instead: place the binary/config locally and install them manually, or use psiphon-mg with an explicit --binary path.")
	return ExitDownloadFailed
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 128 + int(status.Signal())
			}
			return status.ExitStatus()
		}
		return 1
	}

	if errors.Is(err, exec.ErrNotFound) {
		return 127
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
		return 127
	}

	return 1
}
