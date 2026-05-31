package common

import (
	"context"
	"os"
	"runtime/trace"
)

type Source struct {
	Workspace string
	Path      string
	Buffer    []byte `json:"-"`
}

func NewSourceFromFile(ctx context.Context, workspace string, path string) (*Source, error) {
	_, task := trace.NewTask(ctx, "loadFile")

	defer task.End()

	buf, err := os.ReadFile(path)

	if err != nil {
		return nil, err
	}

	return &Source{
		Workspace: workspace,
		Path:      path,
		Buffer:    buf,
	}, nil
}
