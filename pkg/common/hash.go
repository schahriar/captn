package common

import "hash/crc32"

type StringConvertible interface {
	~string |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~[]byte | ~[]rune
}

func PrimaryHash[T StringConvertible](v T) uint32 {
	return crc32.ChecksumIEEE([]byte(string(v)))
}
