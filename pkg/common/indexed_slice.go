package common

import (
	"encoding/json"

	"github.com/goccy/go-yaml"
)

type IndexedSlice[ID comparable, T any] struct {
	Slice   []T
	Index   map[ID]int
	indexer func(T) ID
}

func (is *IndexedSlice[ID, T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(is.Slice)
}

func (is *IndexedSlice[ID, T]) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(is.Slice)
}

func (is *IndexedSlice[ID, T]) Append(val T) int {
	is.Slice = append(is.Slice, val)
	idx := len(is.Slice) - 1
	is.Index[is.indexer(val)] = idx

	return idx
}

func (is *IndexedSlice[ID, T]) Keys() []ID {
	list := make([]ID, 0, len(is.Slice))

	for k := range is.Index {
		list = append(list, k)
	}

	return list
}

func (is *IndexedSlice[ID, T]) GetIndex(key ID) (int, bool) {
	idx, ok := is.Index[key]

	return idx, ok
}

func (is *IndexedSlice[ID, T]) Len() int {
	return len(is.Slice)
}

func (is *IndexedSlice[ID, T]) Get(key ID) (T, bool) {
	var zero T
	idx, ok := is.Index[key]

	if !ok {
		return zero, false
	}

	if idx >= len(is.Slice) {
		return zero, false
	}

	return is.Slice[idx], true
}

func NewIndexedSlice[ID comparable, T any](indexer func(T) ID) *IndexedSlice[ID, T] {
	return &IndexedSlice[ID, T]{
		Slice:   []T{},
		Index:   map[ID]int{},
		indexer: indexer,
	}
}
