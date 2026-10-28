package main

import (
	"os"

	"github.com/jared-christensen/trees/internal/cli"
)

// version is set at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}
