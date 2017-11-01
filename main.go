package main

import (
	"os"
	"github.com/urfave/cli"
	"fmt"
	"encoding/base64"
)

var flags = []cli.Flag{
	cli.StringFlag{
		EnvVar: "PUBLIC_HTTP_HOST",
		Name:   "public-http-host",
	},
	cli.StringFlag{
		EnvVar: "PUBLIC_WS_HOST",
		Name:   "public-ws-host",
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
}

func before(context *cli.Context) error {
	key_data := context.String("signing-key")

	// Decode the signing key
	if key, err := base64.StdEncoding.DecodeString(key_data); err == nil {
		context.Set("signing-key", string(key))
	} else {
		return err
	}

	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "qitup-api"
	app.Action = api
	app.Flags = flags
	app.Before = before

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
