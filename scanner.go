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
			yield(t, fmt.Errorf("non-struct dest type %s with >1 columns (%d)", base.Kind(), len(columns)))
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
			isUnsafe := false
			var m *Mapper
			switch q := q.(type) {
			case *DB:
				m = q.mapper
				isUnsafe = q.isUnsafe
			default:
				m = DefaultMapper
			}

			fields := m.TraversalsByName(base, columns)
			// if we are not unsafe and are missing fields, return an error
			if f, err := missingFields(fields); err != nil && !isUnsafe {
				var t T
				yield(t, fmt.Errorf("missing destination name %s in %T", columns[f], base))
				return
			}
			values := make([]any, len(columns))

			var vp reflect.Value
			var t *T

			for rows.Next() {
				if base.Kind() == reflect.Ptr {
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

				if base.Kind() == reflect.Ptr {
					t, ok := vp.Interface().(T)
					if !ok {
						yield(t, fmt.Errorf("failed to convert pointer of type %T", t))
						return
					}
					if !yield(t, nil) {
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

func missingFields(traversals [][]int) (field int, err error) {
	for i, t := range traversals {
		if len(t) == 0 {
			return i, errors.New("missing field")
		}
	}
	return 0, nil
}

var _scannerInterface = reflect.TypeOf((*sql.Scanner)(nil)).Elem()

// isScannable takes the reflect.Type and the actual dest value and returns
// whether or not it's Scannable. Something is scannable if:
//   - it is not a struct
//   - it implements sql.Scanner
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
		if len(traversal) == 0 {
			values[i] = new(any)
			continue
		}
		// create a value of the field type and set it as any in values
		f := fieldByIndexes(v, traversal)
		values[i] = f.Addr().Interface()
	}
	return nil
}

// fieldByIndexes returns a value for the field given by the struct traversal
// for the given value.
// If traversal is []int{0, 1}, then the path would go from the first field of v
// to the second field of that field, which must also be a struct.
func fieldByIndexes(v reflect.Value, traversal []int) reflect.Value {
	for _, i := range traversal {
		// keep overwriting v until we reach the end of the traversal
		v = reflect.Indirect(v).Field(i)
		// if this is a pointer and it's nil, allocate a new value and set it
		if v.Kind() == reflect.Ptr && v.IsNil() {
			alloc := reflect.New(derefType(v.Type()))
			v.Set(alloc)
		} else if v.Kind() == reflect.Map && v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	}
	return v
}
