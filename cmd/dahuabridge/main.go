package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"RCooLeR/DahuaBridge/internal/app"
	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"github.com/urfave/cli/v2"
)

func main() {
	cliApp := &cli.App{
		Name:  "dahuabridge",
		Usage: "Bridge Dahua NVR/VTO devices into Home Assistant over MQTT",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to the YAML configuration file",
				Value:   defaultConfigPath(),
			},
		},
		Action: func(c *cli.Context) error {
			cfg, err := config.Load(c.String("config"))
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return app.Run(ctx, cfg, buildinfo.Info())
		},
	}

	if err := cliApp.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func defaultConfigPath() string {
	if value := os.Getenv("DAHUABRIDGE_CONFIG"); value != "" {
		return value
	}
	return "config.yaml"
}
