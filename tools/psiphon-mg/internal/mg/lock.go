package mg

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *app) acquireLock(runtimeRoot string) (func(), int) {
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		a.err("failed to prepare runtime root %s: %v", runtimeRoot, err)
		return func() {}, ExitUsage
	}

	lockDir := filepath.Join(runtimeRoot, "lock")
	if err := os.Mkdir(lockDir, 0o755); err != nil {
		if !os.IsExist(err) {
			a.err("failed to create manager lock directory %s: %v", lockDir, err)
			return func() {}, ExitUsage
		}

		pidPath := filepath.Join(lockDir, "pid")
		ownerPath := filepath.Join(lockDir, "owner")
		for attempts := 0; attempts < 5 && !fileExists(pidPath); attempts++ {
			time.Sleep(100 * time.Millisecond)
		}

		ownerPID, _ := readTrimmedInt(pidPath)
		ownerHint, _ := readTrimmedString(ownerPath)
		alive := ownerPID > 0 && processAlive(ownerPID)
		if alive && ownerHint != "" {
			ownerArgs := processArgs(ownerPID)
			if ownerArgs != "" && !strings.Contains(ownerArgs, ownerHint) {
				alive = false
			}
		}

		if !alive {
			a.log("clearing stale manager lock: %s", lockDir)
			if err := os.RemoveAll(lockDir); err != nil {
				a.err("failed to clear stale manager lock %s: %v", lockDir, err)
				return func() {}, ExitUsage
			}
			if err := os.Mkdir(lockDir, 0o755); err != nil {
				if os.IsExist(err) {
					a.err("manager is locked by another command: %s", lockDir)
					return func() {}, ExitLocked
				}
				a.err("failed to recreate manager lock directory %s: %v", lockDir, err)
				return func() {}, ExitUsage
			}
		} else {
			a.err("manager is locked by another command: %s", lockDir)
			return func() {}, ExitLocked
		}
	}

	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		_ = os.RemoveAll(lockDir)
		a.err("failed to write manager lock metadata %s: %v", filepath.Join(lockDir, "pid"), err)
		return func() {}, ExitUsage
	}
	if err := os.WriteFile(filepath.Join(lockDir, "owner"), []byte(a.owner+"\n"), 0o644); err != nil {
		_ = os.RemoveAll(lockDir)
		a.err("failed to write manager lock metadata %s: %v", filepath.Join(lockDir, "owner"), err)
		return func() {}, ExitUsage
	}

	return func() {
		_ = os.RemoveAll(lockDir)
	}, 0
}

func processArgs(pid int) string {
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(string(cmdline), "\x00", " ")
}
