package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

type indexedSliceSimple struct {
	Key   string
	Value int
}

func TestIndexedSliceSimpleCaseEmptyState(t *testing.T) {
	is := common.NewIndexedSlice(func(isp indexedSliceSimple) string {
		return isp.Key
	})

	assert.Equal(t, is.Len(), 0)

	zero, found := is.Get("test")
	assert.Equal(t, indexedSliceSimple{}, zero)
	assert.Equal(t, false, found)

	assert.Len(t, is.Keys(), 0)

	idx, ok := is.GetIndex("test")

	assert.Equal(t, false, ok)
	assert.Equal(t, 0, idx)

	jsonb, err := is.MarshalJSON()

	assert.NoError(t, err)
	assert.Equal(t, "[]", string(jsonb))

	yamlb, err := is.MarshalYAML()

	assert.NoError(t, err)
	assert.Equal(t, "[]\n", string(yamlb))
}

func TestIndexedSliceSimpleCaseInsertion(t *testing.T) {
	is := common.NewIndexedSlice(func(isp indexedSliceSimple) string {
		return isp.Key
	})

	assert.Equal(t, is.Len(), 0)

	idx := is.Append(indexedSliceSimple{
		Key:   "test",
		Value: 10,
	})

	assert.Equal(t, 0, idx)
	assert.Equal(t, 1, is.Len())

	assert.ElementsMatch(t, is.Keys(), []string{"test"})

	idx, ok := is.GetIndex("test")
	assert.True(t, ok)
	assert.Equal(t, 0, idx)

	_, ok = is.GetIndex("unknown")
	assert.False(t, ok)

	is.Get("test")
}
