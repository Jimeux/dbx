package dbx

import (
	"context"
	"database/sql"
	"runtime"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const getPersonQuery = "SELECT * FROM person LIMIT 1"
const getStringQuery = "SELECT first_name FROM person LIMIT 1"
const getAddedAtQuery = "SELECT added_at FROM person LIMIT 1"

func TestGet(t *testing.T) {
	RunWithSchemaContext(context.Background(), defaultSchema, t, func(ctx context.Context, db *sql.DB, tb testing.TB) {
		loadDefaultFixtureContext(ctx, db, tb)

		t.Run("Person value", func(t *testing.T) {
			want := person(1)
			testGet(t, ctx, getPersonQuery, db, want)
		})
		t.Run("Person pointer", func(t *testing.T) {
			want := person(1)
			testGet(t, ctx, getPersonQuery, db, &want)
		})
		t.Run("string value", func(t *testing.T) {
			want := "FirstName1"
			testGet(t, ctx, getStringQuery, db, want)
		})
		t.Run("string pointer", func(t *testing.T) {
			want := "FirstName1"
			testGet(t, ctx, getStringQuery, db, &want)
		})
		t.Run("sql.Null[string] value", func(t *testing.T) {
			want := sql.Null[string]{V: "FirstName1", Valid: true}
			testGet(t, ctx, getStringQuery, db, &want)
		})
		t.Run("time.Time value", func(t *testing.T) {
			want := time.Now()
			testGet(t, ctx, getAddedAtQuery, db, want)
		})
		t.Run("time.Time pointer", func(t *testing.T) {
			want := time.Now()
			testGet(t, ctx, getAddedAtQuery, db, &want)
		})
	})
}

func testGet[T any](t *testing.T, ctx context.Context, query string, db *sql.DB, want T) {
	t.Helper()
	got, err := Get[T](ctx, db, query)
	if err != nil {
		t.Fatalf("got %+v want nil", err)
	}
	if !cmp.Equal(got, want, cmpopts.EquateApproxTime(time.Second*10)) {
		t.Fatalf("(-got +want) %s", cmp.Diff(got, want))
	}
}

func BenchmarkGet(b *testing.B) {
	RunWithSchemaContext(context.Background(), defaultSchema, b, func(ctx context.Context, db *sql.DB, tb testing.TB) {
		loadDefaultFixtureContext(ctx, db, tb)

		b.Run("Person value", func(b *testing.B) {
			benchmarkGet[Person](b, ctx, getStringQuery, db)
		})
		b.Run("Person pointer", func(b *testing.B) {
			benchmarkGet[*Person](b, ctx, getStringQuery, db)
		})
		b.Run("string value", func(b *testing.B) {
			benchmarkGet[string](b, ctx, getStringQuery, db)
		})
		b.Run("string pointer", func(b *testing.B) {
			benchmarkGet[*string](b, ctx, getStringQuery, db)
		})
	})
}

func benchmarkGet[T any](b *testing.B, ctx context.Context, query string, db *sql.DB) {
	b.Helper()
	runtime.GC()
	b.ResetTimer()
	for range b.N {
		if _, err := Get[T](ctx, db, query); err != nil {
			b.Fatalf("got %+v want nil", err)
		}
	}
}
