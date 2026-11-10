package main

import (
	"errors"
	"fmt"
	"os"

	"gigjar/internal/config"
	"gigjar/internal/database"
	"gigjar/internal/logging"
)

const usage = "usage: migrate [up|down-one|version]"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}

	cfg, err := config.LoadMigrate(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat).With("service", "migrate")

	migrator, err := database.NewMigrator(cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}

	switch command {
	case "up":
		err = migrator.Up()
	case "down-one":
		if cfg.Environment.IsProduction() {
			err = errors.New("down-one is disabled in production")
			break
		}
		err = migrator.DownOne()
	case "version":
	default:
		err = errors.New(usage)
	}
	if err != nil {
		return errors.Join(err, migrator.Close())
	}

	status, versionErr := migrator.Version()
	if versionErr == nil {
		logger.Info("schema status", "command", command, "version", status.Version, "dirty", status.Dirty)
	}
	return errors.Join(versionErr, migrator.Close())
}
