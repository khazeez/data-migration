package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"data-migration/internal/config"
	"data-migration/internal/db"
	"data-migration/internal/importer"
	"data-migration/internal/logger"
	"data-migration/internal/sheets"
)

type Runner struct {
	appCfg    *config.AppConfig
	jobsDir   string
	tablesDir string
	sheetsCli *sheets.Client
	database  *db.DB
	log       *logger.Logger
	dryRun    bool
	batchSize int
}

func NewRunner(appCfg *config.AppConfig, jobsDir, tablesDir string, log *logger.Logger) *Runner {
	return &Runner{
		appCfg:    appCfg,
		jobsDir:   jobsDir,
		tablesDir: tablesDir,
		log:       log,
		batchSize: 500,
	}
}

func (r *Runner) SetDryRun(v bool) {
	r.dryRun = v
}

func (r *Runner) SetBatchSize(n int) {
	if n > 0 {
		r.batchSize = n
	}
}

func (r *Runner) InitSheets(ctx context.Context) error {
	cli, err := sheets.NewClient(ctx, r.appCfg.Google.Credential)
	if err != nil {
		return fmt.Errorf("init sheets client: %w", err)
	}
	r.sheetsCli = cli
	return nil
}

func (r *Runner) InitDB(ctx context.Context) error {
	d, err := db.Connect(db.ConnConfig{
		Host:     r.appCfg.Database.Host,
		Port:     r.appCfg.Database.Port,
		User:     r.appCfg.Database.User,
		Password: r.appCfg.Database.Password,
		DBName:   r.appCfg.Database.DBName,
	})
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	r.database = d
	return nil
}

func (r *Runner) Close() {
	if r.database != nil {
		r.database.Close()
	}
}

func (r *Runner) RunJob(ctx context.Context, jobName string) error {
	jobPath := filepath.Join(r.jobsDir, jobName+".yml")
	jobCfg, err := config.LoadJob(jobPath)
	if err != nil {
		return fmt.Errorf("load job %s: %w", jobName, err)
	}

	if len(jobCfg.Jobs) > 0 {
		r.log.Info("Starting meta-job: %s (%d sub-jobs)", jobName, len(jobCfg.Jobs))
		totalStart := time.Now()
		for _, sub := range jobCfg.Jobs {
			r.log.Info("--- Running sub-job: %s ---", sub)
			if err := r.RunJob(ctx, sub); err != nil {
				return err
			}
		}
		totalDur := time.Since(totalStart)
		jobsNote := ""
		if r.dryRun {
			jobsNote = " (dry-run)"
		}
		r.log.Info("========================================")
		r.log.Info("All jobs%s complete: %s | %d sub-jobs | %v elapsed",
			jobsNote, jobName, len(jobCfg.Jobs), totalDur)
		r.log.Info("========================================")
		return nil
	}

	r.log.Info("Starting job: %s (tables: %d)", jobName, len(jobCfg.Tables))

	totalStart := time.Now()
	var totalRows int

	tablePaths := config.ResolveTablePaths(r.tablesDir, jobCfg.Tables)

	for _, tp := range tablePaths {
		tableCfg, err := config.LoadTable(tp)
		if err != nil {
			return fmt.Errorf("load table config %s: %w", tp, err)
		}

		r.log.Info("Processing table: %s (sheet: %s)", tableCfg.Table, tableCfg.Sheet.Worksheet)

		imp := importer.New(r.sheetsCli, r.database, tableCfg, r.log)
		imp.SetDryRun(r.dryRun)
		imp.SetBatchSize(r.batchSize)

		result, err := imp.Run(ctx)
		if err != nil {
			r.log.Error("Table %s failed: %v", tableCfg.Table, err)
			return fmt.Errorf("import %s: %w", tableCfg.Table, err)
		}

		totalRows += result.Inserted

		r.log.Info("  ↳ %s: %d rows inserted | %v", tableCfg.Table, result.Inserted, result.Duration)
	}

	totalDur := time.Since(totalStart)
	var jobsNote string
	if r.dryRun {
		jobsNote = " (dry-run)"
	}
	r.log.Info("========================================")
	r.log.Info("Job%s complete: %s | %d tables | %d rows total | %v elapsed",
		jobsNote, jobName, len(jobCfg.Tables), totalRows, totalDur)
	r.log.Info("========================================")

	return nil
}

func (r *Runner) ListJobs() ([]string, error) {
	entries, err := os.ReadDir(r.jobsDir)
	if err != nil {
		return nil, fmt.Errorf("read jobs dir: %w", err)
	}
	var jobs []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yml" {
			jobs = append(jobs, e.Name()[:len(e.Name())-4])
		}
	}
	return jobs, nil
}
