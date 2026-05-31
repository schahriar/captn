package common

import (
	"context"
	"os"
	"runtime/trace"
)

type Source struct {
	Path   string
	Buffer []byte `json:"-"`
}

func NewSourceFromFile(ctx context.Context, path string) (*Source, error) {
	_, task := trace.NewTask(ctx, "loadFile")

	defer task.End()

	buf, err := os.ReadFile(path)

	if err != nil {
		return nil, err
	}

	return &Source{
		Path:   path,
		Buffer: buf,
	}, nil
}
