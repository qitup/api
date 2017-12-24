package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

var flags = []cli.Flag{
	cli.StringFlag{
		EnvVar: "GIN_MODE",
		Name: "mode",
	},
	cli.StringFlag{
		EnvVar: "HOST",
		Name:   "host",
	},
	cli.BoolFlag{
		EnvVar: "SECURED",
		Name:   "secured",
	},
	cli.BoolFlag{
		EnvVar: "PUBLIC",
		Name:   "public",
	},
	cli.StringFlag{
		EnvVar: "PORT",
		Name:   "port",
	},
	cli.StringFlag{
		EnvVar: "SESSION_SECRET",
		Name:   "session-secret",
		Value:  "secret",
	},
	cli.StringFlag{
		EnvVar: "SIGNING_KEY",
		Name:   "signing-key",
		Usage:  "signing key",
	},
	cli.StringFlag{
		EnvVar: "DATABASE",
		Name:   "database",
		Value:  "dev",
	},
	cli.StringFlag{
		EnvVar: "SPOTIFY_ID",
		Name:   "spotify-id",
	},
	cli.StringFlag{
		EnvVar: "SPOTIFY_SECRET",
		Name:   "spotify-secret",
	},
}

func main() {
	app := cli.NewApp()
	app.Name = "qitup-api"
	app.Action = api
	app.Flags = flags

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
