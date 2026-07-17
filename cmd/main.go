package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"data-migration/internal/config"
	"data-migration/internal/jobs"
	"data-migration/internal/logger"
)

func main() {
	var (
		appCfgPath string
		jobName    string
		listJobs   bool
		dryRun     bool
		batchSize  int
		logLevel   string
	)

	flag.StringVar(&appCfgPath, "config", "configs/app.yml", "Path to app config file")
	flag.StringVar(&jobName, "job", "", "Job name to execute")
	flag.BoolVar(&listJobs, "list", false, "List available jobs")
	flag.BoolVar(&dryRun, "dry-run", false, "Dry run mode (no DB changes)")
	flag.IntVar(&batchSize, "batch", 500, "Batch insert size")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	level := parseLogLevel(logLevel)
	log := logger.New(level)

	appCfg, err := config.LoadApp(appCfgPath)
	if err != nil {
		log.Error("Failed to load config: %v", err)
		os.Exit(1)
	}

	baseDir := filepath.Dir(appCfgPath)
	jobsDir := filepath.Join(baseDir, "jobs")
	tablesDir := filepath.Join(baseDir, "tables")

	runner := jobs.NewRunner(appCfg, jobsDir, tablesDir, log)
	runner.SetDryRun(dryRun)
	runner.SetBatchSize(batchSize)

	if listJobs {
		available, err := runner.ListJobs()
		if err != nil {
			log.Error("Failed to list jobs: %v", err)
			os.Exit(1)
		}
		fmt.Println("Available jobs:")
		for _, j := range available {
			fmt.Printf("  - %s\n", j)
		}
		return
	}

	if jobName == "" {
		log.Error("Job name is required. Use --job flag or --list to see available jobs")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	if err := runner.InitSheets(ctx); err != nil {
		log.Error("Failed to init Google Sheets: %v", err)
		os.Exit(1)
	}
	defer runner.Close()

	if !dryRun {
		if err := runner.InitDB(ctx); err != nil {
			log.Error("Failed to init database: %v", err)
			os.Exit(1)
		}
	}

	if err := runner.RunJob(ctx, jobName); err != nil {
		log.Error("Job failed: %v", err)
		os.Exit(1)
	}
}

func parseLogLevel(s string) logger.Level {
	switch s {
	case "debug":
		return logger.LevelDebug
	case "info":
		return logger.LevelInfo
	case "warn":
		return logger.LevelWarn
	case "error":
		return logger.LevelError
	default:
		return logger.LevelInfo
	}
}
