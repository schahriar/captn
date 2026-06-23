package common

import (
	"context"
)

func Map[T any, R any](slice []T, fn func(T) R) []R {
	result := make([]R, len(slice))
	for i, v := range slice {
		result[i] = fn(v)
	}
	return result
}

func ValuesOf[K comparable, T any](m map[K]T) []T {
	result := make([]T, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

func ParallelCollect[In any, Out any](ctx context.Context, inputs []In, work func(context.Context, In) (Out, error)) ([]Out, error) {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultsChan := make(chan Out, len(inputs))
	errChan := make(chan error, 1)

	for _, val := range inputs {
		go func(v In) {
			value, err := work(workerCtx, v)
			if err != nil {
				select {
				case errChan <- err:
				default:
				}
				cancel()
				return
			}

			resultsChan <- value
		}(val)
	}

	results := make([]Out, 0, len(inputs))
	for completed := 0; completed < len(inputs); {
		select {
		case err := <-errChan:
			return nil, err
		case res := <-resultsChan:
			results = append(results, res)
			completed++
		case <-ctx.Done():
			cancel()
			return nil, ctx.Err()
		}
	}

	return results, nil
}
