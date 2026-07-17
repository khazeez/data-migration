package importer

import (
	"context"
	"fmt"
	"time"

	"data-migration/internal/config"
	"data-migration/internal/db"
	"data-migration/internal/logger"
	"data-migration/internal/mapper"
	"data-migration/internal/sheets"
	"data-migration/internal/transform"
	"data-migration/internal/validator"
)

type Result struct {
	Table    string
	Inserted int
	Errors   int
	Duration time.Duration
}

type Importer struct {
	sheetsCli *sheets.Client
	database  *db.DB
	tableCfg  *config.TableConfig
	log       *logger.Logger
	dryRun    bool
	batchSize int
}

func New(sc *sheets.Client, database *db.DB, tableCfg *config.TableConfig, log *logger.Logger) *Importer {
	return &Importer{
		sheetsCli: sc,
		database:  database,
		tableCfg:  tableCfg,
		log:       log,
		batchSize: 500,
	}
}

func (imp *Importer) SetDryRun(v bool) {
	imp.dryRun = v
}

func (imp *Importer) SetBatchSize(n int) {
	if n > 0 {
		imp.batchSize = n
	}
}

func (imp *Importer) Run(ctx context.Context) (*Result, error) {
	start := time.Now()
	result := &Result{Table: imp.tableCfg.Table}

	imp.log.Info("Reading sheet %s...", imp.tableCfg.Sheet.Worksheet)
	sheetData, err := imp.sheetsCli.ReadSheet(ctx, imp.tableCfg.Sheet.SpreadsheetID, imp.tableCfg.Sheet.Worksheet)
	if err != nil {
		return nil, fmt.Errorf("read sheet: %w", err)
	}

	if len(sheetData.Headers) == 0 {
		imp.log.Warn("Sheet %s is empty", imp.tableCfg.Sheet.Worksheet)
		result.Duration = time.Since(start)
		return result, nil
	}

	imp.log.Info("Validating headers...")
	valReport := validator.ValidateHeaders(sheetData.Headers, imp.tableCfg)
	if !valReport.Valid {
		imp.log.Error("Header validation failed:\n%s", valReport.String())
		return nil, fmt.Errorf("header validation failed for %s", imp.tableCfg.Table)
	}
	if len(valReport.Unmapped) > 0 {
		imp.log.Warn("Unmapped sheet columns: %v", valReport.Unmapped)
	}

	headerIdx := mapper.BuildHeaderIndex(sheetData.Headers)
	t := transform.New(imp.tableCfg)

	imp.log.Info("Transforming %d rows...", len(sheetData.Rows))
	columns, rows, err := t.BuildColumnsAndRows(headerIdx, sheetData.Rows)
	if err != nil {
		return nil, fmt.Errorf("transform data: %w", err)
	}

	if imp.dryRun {
		imp.log.Info("DRY RUN - would insert %d rows into %s", len(rows), imp.tableCfg.Table)
		imp.log.Info("Columns: %v", columns)
		if len(rows) > 0 {
			imp.log.Info("First row sample: %v", rows[0])
		}
		result.Duration = time.Since(start)
		return result, nil
	}

	imp.log.Info("Inserting %d rows into %s (batch size: %d)...", len(rows), imp.tableCfg.Table, imp.batchSize)
	tx, err := imp.database.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	rollback := true
	defer func() {
		if rollback {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				imp.log.Error("Rollback failed: %v", rbErr)
			}
		}
	}()

	if err := imp.database.Truncate(ctx, tx, imp.tableCfg.Table); err != nil {
		return nil, fmt.Errorf("truncate table %s: %w", imp.tableCfg.Table, err)
	}

	if err := imp.database.BatchInsert(ctx, tx, imp.tableCfg.Table, columns, rows, imp.batchSize); err != nil {
		return nil, fmt.Errorf("batch insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	rollback = false

	result.Inserted = len(rows)
	result.Duration = time.Since(start)

	imp.log.Info("Successfully inserted %d rows into %s (%v)", result.Inserted, imp.tableCfg.Table, result.Duration)

	return result, nil
}
