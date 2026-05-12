package main

import (
	"os"

	"psiphon-mg/internal/mg"
)

func main() {
	os.Exit(mg.RunPsiphon(os.Stdout, os.Stderr))
}
