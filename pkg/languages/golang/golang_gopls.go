package languages_golang

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/schahriar/captn/pkg/common"
)

const (
	goplsName           = "gopls"
	goplsInstallCommand = "go install golang.org/x/tools/gopls@latest"
)

func goplsPath(ctx context.Context) (string, error) {
	binary := goplsName

	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}

	// `go install` drops the binary somewhere that is often not on PATH, so the
	// install directory is checked too rather than asking for an install that
	// already happened.
	if dir := goInstallDir(ctx); dir != "" {
		candidate := filepath.Join(dir, binary)

		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("%v was not found in PATH, GOBIN or GOPATH/bin", goplsName)
}

// goInstallDir asks the toolchain where `go install` puts binaries: GOBIN when
// it is set, otherwise bin/ under GOPATH. `go env` is asked rather than the
// process environment because it answers with the defaults too, which is where
// both usually come from.
func goInstallDir(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "go", "env", "GOBIN", "GOPATH").Output()

	if err != nil {
		return ""
	}

	// One line per name, in order, and the GOBIN line is empty when it is unset.
	lines := strings.Split(string(out), "\n")

	if len(lines) < 2 {
		return ""
	}

	if gobin := strings.TrimSpace(lines[0]); gobin != "" {
		return gobin
	}

	for _, path := range filepath.SplitList(strings.TrimSpace(lines[1])) {
		if path != "" {
			return filepath.Join(path, "bin")
		}
	}

	return ""
}

// NormalizeDefinitionRange answers unchanged: gopls already spans the
// identifier in a textDocument/definition reply.
func (glsd *GolangLanguageSupportDefinition) NormalizeDefinitionRange(_ *common.Source, r *common.FileRange) *common.FileRange {
	return r
}
