package mg

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

var (
	installedPsiphonConfigDir    = "/etc/psiphon"
	installedPsiphonBinaryPath   = filepath.Join(installedPsiphonConfigDir, "psiphon-tunnel-core-x86_64")
	installedPsiphonConfigPath   = filepath.Join(installedPsiphonConfigDir, "psiphon.config")
	installedLinphLauncher       = "/usr/local/bin/linph"
	installedPsiphonLauncher     = "/usr/local/bin/psiphon"
	installedPlinstallerLauncher = "/usr/local/bin/plinstaller2"
	installedPluninstallerPath   = "/usr/local/bin/pluninstaller"
	legacyInstalledPsiphonPath   = "/usr/bin/psiphon"
	currentExecutablePath        = os.Executable
)

func RunPsiphon(stdout, stderr io.Writer) int {
	layout := activeInstallLayout()
	cmd := exec.Command(layout.PsiphonBinaryPath, "-config", layout.PsiphonConfigPath)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	return commandExitCode(cmd.Run())
}

func RunPluninstaller(stdout, stderr io.Writer) int {
	return runUninstall("pluninstaller", nil, stdout, stderr)
}

func RunPlinstaller2(stdout, stderr io.Writer) int {
	return runInstall(resolveRepoRoot(), "plinstaller2", nil, stdout, stderr)
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
