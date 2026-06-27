package common

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/cespare/xxhash"
)

type HashType [4]uint64

func (ht HashType) Sum() uint64 {
	return ht[0] + ht[1] + ht[2] + ht[3]
}

func (ht HashType) Marshal() ([]byte, error) {
	var raw [32]byte
	binary.BigEndian.PutUint64(raw[0:8], ht[0])
	binary.BigEndian.PutUint64(raw[8:16], ht[1])
	binary.BigEndian.PutUint64(raw[16:24], ht[2])
	binary.BigEndian.PutUint64(raw[24:32], ht[3])

	var out [43]byte
	base64.RawURLEncoding.Encode(out[:], raw[:])
	return out[:], nil
}

func UnmarshalHashType(data []byte) (*HashType, error) {
	var raw [32]byte
	if n, err := base64.RawURLEncoding.Decode(raw[:], data); err != nil || n != len(raw) {
		return nil, fmt.Errorf("invalid hash %q", data)
	}

	ht := &HashType{
		binary.BigEndian.Uint64(raw[0:8]),
		binary.BigEndian.Uint64(raw[8:16]),
		binary.BigEndian.Uint64(raw[16:24]),
		binary.BigEndian.Uint64(raw[24:32]),
	}
	return ht, nil
}

func (ht HashType) String() string {
	b, _ := ht.Marshal()
	return string(b)
}

func (ht HashType) Debug() string {
	return fmt.Sprintf("%016x:%016x:%016x:%016x", ht[0], ht[1], ht[2], ht[3])
}

func (ht HashType) Equals(other HashType) bool {
	return ht[0] == other[0] && ht[1] == other[1] && ht[2] == other[2] && ht[3] == other[3]
}

func (ht HashType) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", ht.String())), nil
}

func (ht HashType) MarshalText() ([]byte, error) {
	return []byte(ht.String()), nil
}

func (ht *HashType) UnmarshalText(text []byte) error {
	var raw [32]byte
	if n, err := base64.RawURLEncoding.Decode(raw[:], text); err == nil && n == len(raw) {
		ht[0] = binary.BigEndian.Uint64(raw[0:8])
		ht[1] = binary.BigEndian.Uint64(raw[8:16])
		ht[2] = binary.BigEndian.Uint64(raw[16:24])
		ht[3] = binary.BigEndian.Uint64(raw[24:32])
		return nil
	}

	parts := strings.Split(string(text), ":")
	if len(parts) != 4 {
		return fmt.Errorf("invalid hash %q", text)
	}

	for i, part := range parts {
		val, err := strconv.ParseUint(part, 16, 64)
		if err != nil {
			return fmt.Errorf("invalid hash component %q: %w", part, err)
		}
		ht[i] = val
	}

	return nil
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
