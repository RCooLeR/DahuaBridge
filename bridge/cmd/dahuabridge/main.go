package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"RCooLeR/DahuaBridge/internal/app"
	"RCooLeR/DahuaBridge/internal/archive"
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
		Commands: []*cli.Command{
			{
				Name:        "migrate-archive",
				Usage:       "Migrate the archive index from SQLite to the configured MySQL database",
				Description: "Stop every bridge instance before running this command. The SQLite source is opened read-only; the MySQL import is transactional, receipt-protected, and verifies row counts and canonical full-row digests.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "source-sqlite",
						Usage:    "Path to the existing SQLite archive database",
						Required: true,
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					if cfg.Archive.DBDriver != "mysql" {
						return fmt.Errorf("archive.db_driver must be mysql for migrate-archive")
					}
					stats, err := archive.MigrateSQLiteToMySQL(ctx, c.String("source-sqlite"), cfg.Archive)
					if err != nil {
						return err
					}
					if stats.AlreadyApplied {
						fmt.Fprintln(c.Root().Writer, "matching SQLite migration receipt already applied; no rows written")
					}
					for _, table := range []string{"archive_event_scan_progress", "smd_ivs_events", "nvr_recording_chunks", "bridge_mp4_clips", "archive_clip_retention"} {
						fmt.Fprintf(c.Root().Writer, "%s: %d rows verified\n", table, stats.Rows[table])
					}
					return nil
				},
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
