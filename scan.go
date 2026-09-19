package dbx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"reflect"
)

func scan[T any](ctx context.Context, q Queryer, query string, args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		rows, err := q.QueryContext(ctx, query, args...)
		if err != nil {
			var t T
			yield(t, err)
			return
		}
		defer func() { _ = rows.Close() }()

		base := reflect.TypeFor[T]()
		scannable := isScannable(derefType(base))
		columns, err := rows.Columns()
		if err != nil {
			var t T
			yield(t, err)
			return
		}

		// if it's a base type make sure it only has 1 column; if not return an error
		if scannable && len(columns) > 1 {
			var t T
			yield(t, fmt.Errorf("dest type %s is scanned as a single value but query returned %d columns", base, len(columns)))
			return
		}

		if scannable { // non-struct or sql.Scanner type
			for rows.Next() {
				var t T
				if err := rows.Scan(&t); err != nil {
					yield(t, fmt.Errorf("rows.Scan failure for type %T: %w", t, err))
					return
				}
				if !yield(t, nil) {
					return
				}
			}
		} else { // struct type
			var m *Mapper
			switch q := q.(type) {
			case *DB:
				m = q.mapper
			default:
				m = DefaultMapper
			}

			fields := m.TraversalsByName(base, columns)
			// if fields are missing, return an error
			if f, ok := missingFields(fields); !ok {
				var t T
				yield(t, fmt.Errorf("missing destination name %s in %s", columns[f], base))
				return
			}
			values := make([]any, len(columns))

			var vp reflect.Value
			var t *T

			for rows.Next() {
				if base.Kind() == reflect.Pointer {
					vp = reflect.New(base.Elem())
				} else {
					t = new(T)
					vp = reflect.ValueOf(t)
				}
				v := reflect.Indirect(vp)

				// fill values slice with default values of the correct types
				if err := fieldsByTraversal(v, fields, values); err != nil {
					var t T
					yield(t, err)
					return
				}

				// scan into the struct field pointers and yield the result
				if err := rows.Scan(values...); err != nil {
					var t T
					yield(t, fmt.Errorf("failed to scan values for type %T: %w", t, err))
					return
				}

				if base.Kind() == reflect.Pointer {
					ptr, ok := reflect.TypeAssert[T](vp)
					if !ok {
						yield(ptr, fmt.Errorf("failed to convert pointer of type %T", ptr))
						return
					}
					if !yield(ptr, nil) {
						return
					}
				} else {
					if !yield(*t, nil) {
						return
					}
				}
			}
		}

		if err := rows.Err(); err != nil {
			var t T
			yield(t, err)
			return
		}
	}
}

func missingFields(traversals [][]int) (field int, ok bool) {
	for i, t := range traversals {
		if len(t) == 0 {
			return i, false
		}
	}
	return 0, true
}

var _scannerInterface = reflect.TypeFor[sql.Scanner]()

// isScannable takes the [reflect.Type] and the actual dest value and returns
// whether it's Scannable. Something is scannable if:
//   - it is not a struct
//   - it implements [sql.Scanner]
//   - it has no exported fields
func isScannable(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(_scannerInterface) {
		return true
	}
	if t.Kind() != reflect.Struct {
		return true
	}
	// it's not important that we use the right mapper for this particular object,
	// we're only concerned on how many exported fields this struct has
	// TODO 2024/02/24 @Jimeux decide how mapper is handled
	return len(DefaultMapper.TypeMap(t).Index) == 0 // len(mapper().TypeMap(t).Traversal) == 0
}

func fieldsByTraversal(v reflect.Value, traversals [][]int, values []any) error {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return errors.New("fieldsByTraversal argument must be a struct")
	}

	for i, traversal := range traversals {
		// missing fields are not allowed, so len(traversal) will never be 0.
		// create a value of the field type and set it as any in values.
		f, err := fieldByIndexes(v, traversal)
		if err != nil {
			return err
		}
		values[i] = f.Addr().Interface()
	}
	return nil
}

// fieldByIndexes returns a value for the field given by the struct traversal
// for the given value.
// If traversal is []int{0, 1}, then the path would go from the first field of v
// to the second field of that field, which must also be a struct.
func fieldByIndexes(v reflect.Value, traversal []int) (reflect.Value, error) {
	for _, i := range traversal {
		// keep overwriting v until we reach the end of the traversal
		v = reflect.Indirect(v).Field(i)
		// a nil pointer or map must be allocated before we can traverse into it.
		// Values reached through an unexported embedded field are read-only, so
		// return an error instead of letting reflect panic.
		if k := v.Kind(); (k == reflect.Pointer || k == reflect.Map) && v.IsNil() {
			if !v.CanSet() {
				return reflect.Value{}, fmt.Errorf("cannot allocate field of type %s reached through an unexported embedded field", v.Type())
			}
			if k == reflect.Pointer {
				v.Set(reflect.New(derefType(v.Type())))
			} else {
				v.Set(reflect.MakeMap(v.Type()))
			}
		}
	}
	return v, nil
}
