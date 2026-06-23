package common

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func AbsolutePathFromURI(uri string) (string, error) {
	if uri == "" {
		return "", fmt.Errorf("missing URI")
	}

	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	if u.Scheme != "file" {
		return "", fmt.Errorf("expected file URI, got %q", u.Scheme)
	}

	if u.Host != "" && u.Host != "localhost" {
		if runtime.GOOS == "windows" {
			return filepath.Clean(`\\` + u.Host + filepath.FromSlash(u.Path)), nil
		}

		return "", fmt.Errorf("file URI host must be empty or localhost")
	}

	path := u.Path
	if path == "" {
		return "", fmt.Errorf("missing file URI path")
	}

	if runtime.GOOS == "windows" {
		path = strings.TrimPrefix(path, "/")
	}

	path = filepath.FromSlash(path)

	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("file URI path is not absolute")
	}

	return filepath.Clean(path), nil
}
