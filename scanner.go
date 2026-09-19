package dbx

import "iter"

// Scanner yields the row(s) of a query as type T.
type Scanner[T any] iter.Seq2[T, error]

// Collect collects the result of the query in a slice of type []T.
func (s Scanner[T]) Collect() ([]T, error) {
	return s.slice(0)
}

// CollectCap collects the result of the query in a slice of type []T.
// capacity sets the initial capacity of the slice.
// Use this method when you know the expected size of the result to avoid re-allocations.
// Note: it does NOT limit the actual number of results.
func (s Scanner[T]) CollectCap(capacity int) ([]T, error) {
	return s.slice(capacity)
}

func (s Scanner[T]) slice(capacity int) ([]T, error) {
	res := make([]T, 0, capacity)
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

func (s Scanner[T]) Map[U any](fn func(T) U) Scanner[U] {
	return func(yield func(U, error) bool) {
		for row, err := range s {
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
