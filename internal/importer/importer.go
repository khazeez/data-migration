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

	if imp.tableCfg.Filter != nil {
		before := len(sheetData.Rows)
		sheetData.Rows = filterRows(sheetData.Headers, sheetData.Rows, imp.tableCfg.Filter)
		if len(sheetData.Rows) < before {
			imp.log.Info("Filtered out %d rows (not matching %s = %v)", before-len(sheetData.Rows), imp.tableCfg.Filter.Column, imp.tableCfg.Filter.Value)
		}
	}

	imp.log.Info("Transforming %d rows...", len(sheetData.Rows))
	columns, rows, err := t.BuildColumnsAndRows(headerIdx, sheetData.Rows)
	if err != nil {
		return nil, fmt.Errorf("transform data: %w", err)
	}

	if hasLookup(imp.tableCfg.Mapping) {
		imp.log.Info("Resolving lookup values...")
		if err := imp.resolveLookups(ctx, columns, rows); err != nil {
			return nil, fmt.Errorf("resolve lookups: %w", err)
		}
	}

	if imp.tableCfg.Filter != nil {
		imp.log.Info("Filter: %s = %v", imp.tableCfg.Filter.Column, imp.tableCfg.Filter.Value)
	}

	if imp.tableCfg.OnConflict != nil {
		imp.log.Info("Conflict action: %s (keys: %v)", imp.tableCfg.OnConflict.Action, imp.tableCfg.OnConflict.Keys)
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

	if imp.tableCfg.OnConflict != nil && imp.tableCfg.OnConflict.Action == "ignore" && len(imp.tableCfg.OnConflict.Keys) > 0 {
		rows, err = imp.filterExisting(ctx, tx, columns, rows)
		if err != nil {
			return nil, fmt.Errorf("filter existing: %w", err)
		}
	}

	if len(rows) == 0 {
		imp.log.Info("No rows to insert into %s", imp.tableCfg.Table)
		result.Duration = time.Since(start)
		rollback = false
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return result, nil
	}

	imp.log.Info("Inserting %d rows into %s (batch size: %d)...", len(rows), imp.tableCfg.Table, imp.batchSize)
	conflict := resolveConflict(imp.tableCfg.OnConflict)
	if err := imp.database.BatchInsert(ctx, tx, imp.tableCfg.Table, columns, rows, imp.batchSize, conflict); err != nil {
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

func filterRows(columns []string, rows [][]interface{}, f *config.FilterConfig) [][]interface{} {
	idx := -1
	for i, col := range columns {
		if col == f.Column {
			idx = i
			break
		}
	}
	if idx < 0 {
		return rows
	}

	var filtered [][]interface{}
	for _, row := range rows {
		if idx < len(row) && fmt.Sprintf("%v", row[idx]) == fmt.Sprintf("%v", f.Value) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func (imp *Importer) filterExisting(ctx context.Context, tx db.Transaction, columns []string, rows [][]interface{}) ([][]interface{}, error) {
	keyCol := imp.tableCfg.OnConflict.Keys[0]
	keyIdx := -1
	for i, col := range columns {
		if col == keyCol {
			keyIdx = i
			break
		}
	}
	if keyIdx < 0 {
		return nil, fmt.Errorf("identify column %q not found in columns", keyCol)
	}

	values := make([]interface{}, len(rows))
	for i, row := range rows {
		values[i] = row[keyIdx]
	}

	existing, err := imp.database.FilterExisting(ctx, tx, imp.tableCfg.Table, keyCol, values)
	if err != nil {
		return nil, err
	}

	var filtered [][]interface{}
	for _, row := range rows {
		key := fmt.Sprintf("%v", row[keyIdx])
		if !existing[key] {
			filtered = append(filtered, row)
		}
	}

	skipped := len(rows) - len(filtered)
	if skipped > 0 {
		imp.log.Info("Skipped %d rows (already exist)", skipped)
	}

	return filtered, nil
}

func hasLookup(mapping map[string]config.ColumnMap) bool {
	for _, cm := range mapping {
		if cm.Lookup != nil {
			return true
		}
	}
	return false
}

func (imp *Importer) resolveLookups(ctx context.Context, columns []string, rows [][]interface{}) error {
	type lookupJob struct {
		idx   int
		col   string
		table string
		from  string
		to    string
	}

	var jobs []lookupJob
	for sheetCol, cm := range imp.tableCfg.Mapping {
		if cm.Lookup == nil {
			continue
		}
		for i, col := range columns {
			if col == cm.Column {
				jobs = append(jobs, lookupJob{
					idx:   i,
					col:   col,
					table: cm.Lookup.Table,
					from:  cm.Lookup.From,
					to:    cm.Lookup.To,
				})
				imp.log.Info("Lookup %s → %s.%s (%s = %s.%s)", sheetCol, cm.Lookup.Table, cm.Lookup.To, cm.Lookup.From, cm.Lookup.Table, cm.Lookup.To)
				break
			}
		}
	}

	if len(jobs) == 0 {
		return nil
	}

	for _, j := range jobs {
		seen := make(map[string]bool)
		var uniqueVals []interface{}
		for _, row := range rows {
			if j.idx < len(row) && row[j.idx] != nil {
				key := fmt.Sprintf("%v", row[j.idx])
				if !seen[key] {
					seen[key] = true
					uniqueVals = append(uniqueVals, row[j.idx])
				}
			}
		}

		if len(uniqueVals) == 0 {
			continue
		}

		if imp.dryRun {
			imp.log.Info("DRY RUN - would resolve %d values: %v", len(uniqueVals), uniqueVals)
			continue
		}

		imp.log.Info("Resolving %d unique values for %s...", len(uniqueVals), j.col)
		resolved, err := imp.database.LookupValues(ctx, j.table, j.from, j.to, uniqueVals)
		if err != nil {
			return fmt.Errorf("lookup %s.%s: %w", j.table, j.from, err)
		}

		resolvedCount := 0
		for _, row := range rows {
			if j.idx < len(row) && row[j.idx] != nil {
				key := fmt.Sprintf("%v", row[j.idx])
				if v, ok := resolved[key]; ok {
					row[j.idx] = v
					resolvedCount++
				} else {
					imp.log.Warn("Lookup value %q not found in %s.%s", row[j.idx], j.table, j.from)
					row[j.idx] = nil
				}
			}
		}
		imp.log.Info("Resolved %d/%d values for %s", resolvedCount, len(rows), j.col)
	}

	return nil
}

func resolveConflict(cfg *config.ConflictConfig) *db.ConflictClause {
	if cfg == nil || cfg.Action == "ignore" {
		return nil
	}
	return &db.ConflictClause{
		Action: cfg.Action,
		Keys:   cfg.Keys,
	}
}
