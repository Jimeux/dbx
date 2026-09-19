package dbx

import (
	"context"
	"database/sql"
	"strings"
)

var DefaultMapper = NewMapperFunc("db", strings.ToLower)

type DB struct {
	*sql.DB
	mapper       *Mapper // required: construct with NewDB, as a bare DB{} literal panics on struct queries
	useErrNoRows bool    // return sql.ErrNoRows for Get calls with no result
}

func NewDB(db *sql.DB, mapper *Mapper, useErrNoRows bool) *DB {
	if mapper == nil {
		mapper = DefaultMapper
	}
	return &DB{
		DB:           db,
		mapper:       mapper,
		useErrNoRows: useErrNoRows,
	}
}

type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// TODO 2024/02/24 @Jimeux want to prevent *sql.RawBytes in a constraint

func Get[T any](ctx context.Context, q Queryer, query string, args ...any) (T, error) {
	for row, err := range scan[T](ctx, q, query, args...) {
		return row, err
	}

	var t T
	switch q := q.(type) { // TODO would prefer a less hacky (and costly?) way
	case *DB:
		if q.useErrNoRows {
			return t, sql.ErrNoRows
		}
	}
	return t, nil
}

func Select[T any](ctx context.Context, q Queryer, query string, args ...any) Scanner[T] {
	return Scanner[T](scan[T](ctx, q, query, args...))
}
