package main

import (
	"io"
	"os"

	"psiphon-mg/internal/mg"
)

func run(argv []string, stdout, stderr io.Writer) int {
	invokedAs := "linph"
	args := []string{}
	if len(argv) > 0 {
		invokedAs = argv[0]
		args = argv[1:]
	}
	return mg.RunLinphAlias(invokedAs, args, stdout, stderr)
}

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}
