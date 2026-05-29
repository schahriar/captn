package common

import "os"

type Source struct {
	Path   string
	Buffer []byte `json:"-"`
}

func NewSourceFromFile(path string) (*Source, error) {
	buf, err := os.ReadFile(path)

	if err != nil {
		return nil, err
	}

	return &Source{
		Path:   path,
		Buffer: buf,
	}, nil
}
