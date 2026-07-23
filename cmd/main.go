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
		runAll     bool
		dryRun     bool
		batchSize  int
		logLevel   string
	)

	flag.StringVar(&appCfgPath, "config", "", "Path to app config file (optional)")
	flag.StringVar(&jobName, "job", "", "Job name to execute")
	flag.BoolVar(&listJobs, "list", false, "List available jobs")
	flag.BoolVar(&runAll, "all", false, "Run all available jobs")
	flag.BoolVar(&dryRun, "dry-run", false, "Dry run mode (no DB changes)")
	flag.IntVar(&batchSize, "batch", 500, "Batch insert size")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	if jobName == "" && flag.NArg() > 0 {
		jobName = flag.Arg(0)
	}

	level := parseLogLevel(logLevel)
	log := logger.New(level)

	appCfg, err := config.LoadApp(appCfgPath)
	if err != nil {
		log.Error("Failed to load config: %v", err)
		os.Exit(1)
	}

	var jobsDir, tablesDir string
	if appCfgPath != "" {
		base := filepath.Dir(appCfgPath)
		jobsDir = envOrDefault("JOBS_DIR", filepath.Join(base, "jobs"))
		tablesDir = envOrDefault("TABLES_DIR", filepath.Join(base, "tables"))
	} else {
		jobsDir = envOrDefault("JOBS_DIR", "configs/jobs")
		tablesDir = envOrDefault("TABLES_DIR", "configs/tables")
	}

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

	if runAll {
		if err := runner.RunJob(ctx, "_all"); err != nil {
			log.Error("Failed: %v", err)
			os.Exit(1)
		}
		return
	}

	if jobName == "" {
		log.Error("Job name required. Use: go run ./cmd/ <job>, -job <job>, -all, or -list")
		flag.Usage()
		os.Exit(1)
	}

	if err := runner.RunJob(ctx, jobName); err != nil {
		log.Error("Job failed: %v", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
