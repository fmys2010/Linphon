package main

import (
	"os"

	"psiphon-mg/internal/mg"
)

func main() {
	os.Exit(mg.RunMultiInstance(os.Args[1:], os.Stdout, os.Stderr))
}
