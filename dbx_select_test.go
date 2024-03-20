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

const selectPersonQuery = "SELECT * FROM person"
const selectStringQuery = "SELECT first_name FROM person"

func ExampleSelect() {
	ctx := context.Background()
	var db *sql.DB

	// loop through iterator directly to populate a map
	byName := make(map[string]Person)
	for p, err := range Select[Person](ctx, db, selectPersonQuery) {
		if err != nil {
			return
		}
		byName[p.FirstName] = p
	}
	_ = byName

	// collect results into a slice
	people, err := Select[Person](ctx, db, selectPersonQuery).Slice()
	if err != nil {
		return
	}
	_ = people

	// filter results in memory and collect into a slice
	people, err = Select[Person](ctx, db, selectPersonQuery).
		Filter(func(p Person) bool { return p.Email != "" }).
		SliceCap(10)
	if err != nil {
		return
	}
	_ = people
}

func TestSelect(t *testing.T) {
	RunWithSchemaContext(context.Background(), defaultSchema, t, func(ctx context.Context, db *sql.DB, tb testing.TB) {
		loadDefaultFixtureContext(ctx, db, tb)

		t.Run("Person values", func(t *testing.T) {
			want := person(1)
			testSelect[Person](t, ctx, selectPersonQuery, db, want, personRecordNum)
		})
		t.Run("Person pointers", func(t *testing.T) {
			want := person(1)
			testSelect[*Person](t, ctx, selectPersonQuery, db, &want, personRecordNum)
		})
		t.Run("string values", func(t *testing.T) {
			want := "FirstName1"
			testSelect[string](t, ctx, selectStringQuery, db, want, personRecordNum)
		})
		t.Run("string pointers", func(t *testing.T) {
			want := "FirstName1"
			testSelect[*string](t, ctx, selectStringQuery, db, &want, personRecordNum)
		})
	})
}

func testSelect[T any](t *testing.T, ctx context.Context, query string, db *sql.DB, want T, wantLen int) {
	t.Helper()
	got, err := Select[T](ctx, db, query).SliceCap(20)
	if err != nil {
		t.Fatalf("got %+v want nil", err)
	}
	if len(got) != wantLen {
		t.Fatalf("got %+v len want len %+v", len(got), wantLen)
	}
	if !cmp.Equal(got[0], want, cmpopts.EquateApproxTime(time.Second*10)) {
		t.Fatalf("(-got +want) %s", cmp.Diff(got[0], want))
	}
}

func BenchmarkSelectRows(b *testing.B) {
	RunWithSchemaContext(context.Background(), defaultSchema, b, func(ctx context.Context, db *sql.DB, t testing.TB) {
		loadDefaultFixtureContext(ctx, db, t)

		b.Run("Person value", func(b *testing.B) {
			benchmarkSelect[Person](b, ctx, selectPersonQuery, db, personRecordNum)
		})
		b.Run("Person pointers", func(b *testing.B) {
			benchmarkSelect[*Person](b, ctx, selectPersonQuery, db, personRecordNum)
		})
		b.Run("string values", func(b *testing.B) {
			benchmarkSelect[string](b, ctx, selectStringQuery, db, personRecordNum)
		})
		b.Run("string pointers", func(b *testing.B) {
			benchmarkSelect[*string](b, ctx, selectStringQuery, db, personRecordNum)
		})
	})
}

func benchmarkSelect[T any](b *testing.B, ctx context.Context, query string, db *sql.DB, want int) {
	b.Helper()
	runtime.GC()
	b.ResetTimer()
	for range b.N {
		got, err := Select[T](ctx, db, query).SliceCap(want)
		if err != nil {
			b.Fatalf("got %+v want nil", err)
		}
		if len(got) != want {
			b.Fatalf("got %+v len want len %+v", len(got), want)
		}
	}
}
