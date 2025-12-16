package dbx

import (
	"context"
	"database/sql"
	"errors"
	"iter"
	"strings"
)

var DefaultMapper = NewMapperFunc("db", strings.ToLower)

type DB struct {
	*sql.DB
	mapper *Mapper
	// TODO @Jimeux want to focus on type-safety, so no need to support this?
	isUnsafe    bool // true allows silently ignoring SQL columns that are missing in struct fields
	noRowsErr   bool
	errHandlers []func(error) error
}

type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// TODO 2024/02/24 @Jimeux want to prevent *sql.RawBytes in a constraint

func Get[T any](ctx context.Context, q Queryer, query string, args ...any) (T, error) {
	// TODO 2024/02/25 @Jimeux decide on nil vs. error handling
	for row, err := range scan[T](ctx, q, query, args...) {
		if err != nil {
			switch q := q.(type) {
			case *DB:
				if !q.noRowsErr && errors.Is(err, sql.ErrNoRows) {
					return row, nil
				}
			}
		}
		return row, err
	}
	var t T
	return t, nil
}

func Select[T any](ctx context.Context, q Queryer, query string, args ...any) Scanner[T] {
	return Scanner[T](scan[T](ctx, q, query, args...))
}

// Scanner returns the row(s) of a query as type T.
type Scanner[T any] iter.Seq2[T, error]

// Collect collects the result of the query in a slice of type []T.
func (s Scanner[T]) Collect() ([]T, error) {
	return s.slice(0)
}

// CollectCap collects the result as the query in a slice of type []T.
// cap sets the initial capacity of the slice.
// Use this method when you know the expected size of the result to avoid re-allocations.
// Note: it does NOT limit the actual number of results.
func (s Scanner[T]) CollectCap(cap int) ([]T, error) {
	return s.slice(cap)
}

func (s Scanner[T]) slice(cap int) ([]T, error) {
	res := make([]T, 0, cap)
	for ent, err := range s {
		if err != nil {
			return nil, err
		}
		res = append(res, ent)
	}
	return res, nil
}

func (s Scanner[T]) Filter(fn func(T) bool) Scanner[T] {
	return func(yield func(T, error) bool) {
		for row, err := range s {
			if err != nil {
				yield(row, err)
				return
			}
			if !fn(row) {
				continue
			}
			if !yield(row, nil) {
				return
			}
		}
	}
}

func Map[T, U any](seq Scanner[T], fn func(T) U) Scanner[U] {
	return func(yield func(U, error) bool) {
		for row, err := range seq {
			if err != nil {
				var u U
				yield(u, err)
				return
			}
			if !yield(fn(row), nil) {
				return
			}
		}
	}
}
