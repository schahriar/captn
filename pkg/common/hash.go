package common

import (
	"fmt"

	"github.com/cespare/xxhash"
)

type HashType [4]uint64

func (ht HashType) Sum() uint64 {
	return ht[0] + ht[1] + ht[2] + ht[3]
}

func (ht HashType) String() string {
	return fmt.Sprintf("%08x%08x%08x%08x", ht[0], ht[1], ht[2], ht[3])
}

func (ht HashType) Add(other HashType) HashType {
	return HashType{
		ht[0] + other[0],
		ht[1] + other[1],
		ht[2] + other[2],
		ht[3] + other[3],
	}
}

type StringConvertible interface {
	~string |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~[]byte | ~[]rune
}

func PrimaryHash[T StringConvertible](v T) HashType {
	return [4]uint64{xxhash.Sum64([]byte(string(v))), 0, 0, 0}
}

func HashMany[T StringConvertible](v ...T) HashType {
	hashes := [4]uint64{0, 0, 0, 0}
	for i, item := range v {
		hashes[i%4] += xxhash.Sum64([]byte(string(item)))
	}
	return hashes
}
