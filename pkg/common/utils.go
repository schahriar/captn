package common

func ClampWithinSlice[V any](n int, slice []V) int {
	return max(0, min(n, len(slice)-1))
}

func Map[T any, R any](slice []T, fn func(T) R) []R {
	result := make([]R, len(slice))
	for i, v := range slice {
		result[i] = fn(v)
	}
	return result
}
