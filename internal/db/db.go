package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool *pgxpool.Pool
}

func Connect(dsn string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
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

func (d *DB) BatchInsert(ctx context.Context, tx Transaction, table string, columns []string, rows [][]interface{}, batchSize int) error {
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		if err := d.insertBatch(ctx, tx, table, columns, batch); err != nil {
			return fmt.Errorf("batch %d-%d: %w", i+1, end, err)
		}
	}
	return nil
}

func (d *DB) insertBatch(ctx context.Context, tx Transaction, table string, columns []string, rows [][]interface{}) error {
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

func (d *DB) Truncate(ctx context.Context, tx Transaction, table string) error {
	_, err := tx.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table))
	return err
}

type Transaction interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (interface{}, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type pgxTx struct {
	tx pgx.Tx
}

func (t *pgxTx) Exec(ctx context.Context, sql string, args ...interface{}) (interface{}, error) {
	return t.tx.Exec(ctx, sql, args...)
}

func (t *pgxTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t *pgxTx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}
