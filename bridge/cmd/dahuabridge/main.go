package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"RCooLeR/DahuaBridge/internal/app"
	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"github.com/urfave/cli/v3"
)

func main() {
	info := buildinfo.Info()
	cliApp := &cli.Command{
		Name:    "dahuabridge",
		Usage:   "Bridge Dahua NVR/VTO/IPC devices into Home Assistant and local HTTP APIs",
		Version: info.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to the YAML configuration file",
				Value:   defaultConfigPath(),
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			cfg, err := config.Load(c.String("config"))
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			return app.Run(ctx, cfg, info)
		},
	}

	if err := cliApp.Run(context.Background(), os.Args); err != nil {
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
