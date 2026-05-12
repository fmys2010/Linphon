package main

import (
	"os"

	"psiphon-mg/internal/mg"
)

func main() {
	os.Exit(mg.RunPluninstaller(os.Stdout, os.Stderr))
}
