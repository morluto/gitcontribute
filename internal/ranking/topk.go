// Package ranking owns exact bounded selection utilities.
package ranking

import (
	"container/heap"
	"sort"
)

// TopK returns the first k values under better without mutating the input.
// Better must define a strict total order and return true when a ranks before b.
func TopK[T any](values []T, k int, better func(a, b T) bool) []T {
	if k <= 0 || len(values) == 0 {
		return []T{}
	}
	if k >= len(values) {
		out := append([]T(nil), values...)
		sort.Slice(out, func(i, j int) bool { return better(out[i], out[j]) })
		return out
	}

	selected := &worstFirstHeap[T]{better: better, values: make([]T, 0, k)}
	heap.Init(selected)
	for _, value := range values {
		if selected.Len() < k {
			heap.Push(selected, value)
			continue
		}
		if better(value, selected.values[0]) {
			selected.values[0] = value
			heap.Fix(selected, 0)
		}
	}

	out := append([]T(nil), selected.values...)
	sort.Slice(out, func(i, j int) bool { return better(out[i], out[j]) })
	return out
}

type worstFirstHeap[T any] struct {
	values []T
	better func(a, b T) bool
}

func (h worstFirstHeap[T]) Len() int { return len(h.values) }

func (h worstFirstHeap[T]) Less(i, j int) bool {
	return h.better(h.values[j], h.values[i])
}

func (h worstFirstHeap[T]) Swap(i, j int) {
	h.values[i], h.values[j] = h.values[j], h.values[i]
}

func (h *worstFirstHeap[T]) Push(value any) {
	typed, ok := value.(T)
	if !ok {
		panic("ranking heap received a value of the wrong type")
	}
	h.values = append(h.values, typed)
}

func (h *worstFirstHeap[T]) Pop() any {
	last := len(h.values) - 1
	value := h.values[last]
	var zero T
	h.values[last] = zero
	h.values = h.values[:last]
	return value
}
