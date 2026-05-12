package main

import (
	"os"

	"psiphon-mg/internal/mg"
)

func main() {
	os.Exit(mg.RunPlinstaller2(os.Stdout, os.Stderr))
}
