package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool *pgxpool.Pool
}

type ConnConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

func Connect(cfg ConnConfig) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("parse default config: %w", err)
	}
	poolCfg.ConnConfig.Host = cfg.Host
	poolCfg.ConnConfig.Port = uint16(cfg.Port)
	poolCfg.ConnConfig.User = cfg.User
	poolCfg.ConnConfig.Password = cfg.Password
	poolCfg.ConnConfig.Database = cfg.DBName
	poolCfg.ConnConfig.TLSConfig = nil

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Close() {
	d.pool.Close()
}

type ConflictClause struct {
	Action string
	Keys   []string
}

func (d *DB) BatchInsert(ctx context.Context, tx Transaction, table string, columns []string, rows [][]interface{}, batchSize int, conflict *ConflictClause) error {
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		if err := d.insertBatch(ctx, tx, table, columns, batch, conflict); err != nil {
			return fmt.Errorf("batch %d-%d: %w", i+1, end, err)
		}
	}
	return nil
}

func (d *DB) insertBatch(ctx context.Context, tx Transaction, table string, columns []string, rows [][]interface{}, conflict *ConflictClause) error {
	if len(rows) == 0 {
		return nil
	}

	placeholders := make([]string, len(rows))
	args := make([]interface{}, 0, len(rows)*len(columns))

	for i, row := range rows {
		colPlaceholders := make([]string, len(columns))
		for j, val := range row {
			idx := i*len(columns) + j + 1
			colPlaceholders[j] = fmt.Sprintf("$%d", idx)
			args = append(args, val)
		}
		placeholders[i] = fmt.Sprintf("(%s)", strings.Join(colPlaceholders, ", "))
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	if conflict != nil && len(conflict.Keys) > 0 {
		switch conflict.Action {
		case "ignore":
			query += fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", strings.Join(conflict.Keys, ", "))
		case "update":
			updates := make([]string, len(columns))
			for i, col := range columns {
				updates[i] = fmt.Sprintf("%s = EXCLUDED.%s", col, col)
			}
			query += fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s",
				strings.Join(conflict.Keys, ", "),
				strings.Join(updates, ", "),
			)
		case "identify":
			keySet := make(map[string]bool, len(conflict.Keys))
			for _, k := range conflict.Keys {
				keySet[k] = true
			}
			updates := make([]string, 0, len(columns))
			where := make([]string, 0, len(columns))
			for _, col := range columns {
				if keySet[col] {
					continue
				}
				updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
				where = append(where, fmt.Sprintf("%s.%s IS DISTINCT FROM EXCLUDED.%s", table, col, col))
			}
			if len(where) > 0 {
				query += fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s WHERE %s",
					strings.Join(conflict.Keys, ", "),
					strings.Join(updates, ", "),
					strings.Join(where, " OR "),
				)
			} else {
				query += fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", strings.Join(conflict.Keys, ", "))
			}
		}
	}

	_, err := tx.Exec(ctx, query, args...)
	return err
}

func (d *DB) Begin(ctx context.Context) (Transaction, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &pgxTx{tx: tx}, nil
}

func (d *DB) FilterExisting(ctx context.Context, tx Transaction, table string, identifyCol string, values []interface{}) (map[string]bool, error) {
	if len(values) == 0 {
		return map[string]bool{}, nil
	}

	placeholders := make([]string, len(values))
	args := make([]interface{}, len(values))
	for i, v := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = v
	}

	query := fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IN (%s)",
		identifyCol, table, identifyCol, strings.Join(placeholders, ", "))

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query existing: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var val interface{}
		if err := rows.Scan(&val); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		existing[fmt.Sprintf("%v", val)] = true
	}
	return existing, nil
}

func (d *DB) LookupValues(ctx context.Context, table, fromCol, toCol string, values []interface{}) (map[string]interface{}, error) {
	if len(values) == 0 {
		return map[string]interface{}{}, nil
	}

	placeholders := make([]string, len(values))
	args := make([]interface{}, len(values))
	for i, v := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = v
	}

	query := fmt.Sprintf("SELECT DISTINCT %s, %s FROM %s WHERE %s IN (%s)",
		fromCol, toCol, table, fromCol, strings.Join(placeholders, ", "))

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("lookup query: %w", err)
	}
	defer rows.Close()

	result := make(map[string]interface{})
	for rows.Next() {
		var from string
		var to interface{}
		if err := rows.Scan(&from, &to); err != nil {
			return nil, fmt.Errorf("lookup scan: %w", err)
		}
		switch v := to.(type) {
		case [16]uint8:
			to = uuid.UUID(v).String()
		case pgtype.UUID:
			to = uuid.UUID(v.Bytes).String()
		}
		result[from] = to
	}

	return result, nil
}

func (d *DB) Truncate(ctx context.Context, tx Transaction, table string) error {
	_, err := tx.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table))
	return err
}

type Rows interface {
	Close()
	Next() bool
	Scan(dest ...interface{}) error
}

type Transaction interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (interface{}, error)
	Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type pgxTx struct {
	tx pgx.Tx
}

func (t *pgxTx) Exec(ctx context.Context, sql string, args ...interface{}) (interface{}, error) {
	return t.tx.Exec(ctx, sql, args...)
}

func (t *pgxTx) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	rows, err := t.tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (t *pgxTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t *pgxTx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}
