package main

import (
	"embed"

	"github.com/jeeftor/camspeak/cmd"
	"github.com/jeeftor/camspeak/internal/api"
)

//go:embed frontend/dist
var staticFiles embed.FS

// Version is set via -ldflags "-X main.Version=..." at build time.
var Version = "dev"

func main() {
	api.SetStaticFiles(staticFiles)
	api.SetVersion(Version)
	cmd.Execute()
}
