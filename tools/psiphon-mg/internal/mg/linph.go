package mg

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type linphApp struct {
	stdout   io.Writer
	stderr   io.Writer
	repoRoot string
}

func RunLinph(args []string, stdout, stderr io.Writer) int {
	return RunLinphAlias("linph", args, stdout, stderr)
}

func RunLinphAlias(invokedAs string, args []string, stdout, stderr io.Writer) int {
	app := &linphApp{
		stdout:   stdout,
		stderr:   stderr,
		repoRoot: resolveRepoRoot(),
	}
	return app.run(filepath.Base(invokedAs), args)
}

func (a *linphApp) run(invokedAs string, args []string) int {
	switch invokedAs {
	case "psiphon":
		return a.runInstalledCommand(invokedAs, args)
	case "plinstaller2":
		return runInstall(a.repoRoot, invokedAs, args, a.stdout, a.stderr)
	case "pluninstaller":
		return runUninstall(invokedAs, args, a.stdout, a.stderr)
	}

	if len(args) == 0 {
		a.usage(a.stderr)
		return ExitUsage
	}

	switch args[0] {
	case "run":
		return a.runInstalledCommand("linph run", args[1:])
	case "start", "restart", "stop", "port", "ctry", "log", "switch-port", "switch-ctry":
		return a.runInstalledControlCommand(args[0], args[1:])
	case "install":
		return runInstall(a.repoRoot, "linph install", args[1:], a.stdout, a.stderr)
	case "uninstall":
		return runUninstall("linph uninstall", args[1:], a.stdout, a.stderr)
	case "mg":
		return RunNamed(args[1:], os.Args[0], "linph mg", a.stdout, a.stderr)
	case "multi", "multi-instance":
		return RunMultiInstanceNamed(args[1:], "linph multi", a.stdout, a.stderr)
	case "staged":
		return RunStagedNamed(args[1:], "linph staged", a.stdout, a.stderr)
	case "--help", "-h", "help":
		a.usage(a.stdout)
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown linph command: %s\n", args[0])
		a.usage(a.stderr)
		return ExitUsage
	}
}

func (a *linphApp) runInstalledCommand(usageName string, args []string) int {
	if len(args) != 0 {
		switch args[0] {
		case "--help", "-h", "help":
			runUsage(a.stdout, usageName)
			return 0
		default:
			fmt.Fprintf(a.stderr, "%s does not accept additional arguments\n", usageName)
			runUsage(a.stderr, usageName)
			return ExitUsage
		}
	}
	return RunPsiphon(a.stdout, a.stderr)
}

func (a *linphApp) runInstalledControlCommand(command string, args []string) int {
	installed := &app{stdout: a.stdout, stderr: a.stderr, repoRoot: a.repoRoot, owner: "linph", usageName: "linph"}
	return installed.runInstalledControlCommand(command, args)
}

func (a *linphApp) usage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  linph run
  linph start
  linph restart
  linph stop
  linph port
  linph ctry
  linph log
  linph switch-port HTTP_PORT SOCKS_PORT
  linph switch-ctry REGION1,REGION2,...
  linph install [options]
  linph uninstall [options]
  linph mg <command> [options]
  linph multi <command> [options]
  linph staged [options]

Commands:
  run         Run the installed tunnel-core with the installed config.
  start       Start or reconcile all installed slots.
  restart     Restart all installed slots.
  stop        Stop all installed slots.
  port        Print configured slot port pairs.
  ctry        Print configured regions.
  log         Follow installed logs until Ctrl-C.
  switch-port Update starting ports and restart if running.
  switch-ctry Update regions and restart if running.
  install     Install linph, the tunnel-core artifact, and compatibility aliases.
  uninstall   Remove linph and installed artifacts; use --purge to delete config dir.
  mg          Repo-local single-region manager commands.
  multi       Repo-local multi-instance harness commands.
  staged      Repo-local staged harness runner.

Compatibility aliases:
  psiphon         Equivalent to linph run
  plinstaller2    Equivalent to linph install
  pluninstaller   Equivalent to linph uninstall
`)
}

func runUsage(w io.Writer, usageName string) {
	fmt.Fprintf(w, `Usage:
  %s
`, usageName)
}
