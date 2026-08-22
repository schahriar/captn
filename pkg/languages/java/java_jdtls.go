package languages_java

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
)

const (
	jdtlsName           = "jdtls"
	jdtlsInstallCommand = "brew install jdtls"
)

func jdtlsPath(ctx context.Context) (string, error) {
	// LookPath also resolves the .bat launcher on Windows via PATHEXT
	if path, err := exec.LookPath(jdtlsName); err == nil {
		return path, nil
	}

	// Homebrew links the launcher under its prefix, which is often not on
	// PATH for non-interactive shells, so the prefix is asked rather than
	// asking for an install that already happened.
	if prefix := brewPrefix(ctx); prefix != "" {
		candidates := []string{jdtlsName}

		if runtime.GOOS == "windows" {
			candidates = []string{jdtlsName + ".bat"}
		}

		for _, name := range candidates {
			candidate := filepath.Join(prefix, "bin", name)

			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("%v was not found in PATH or the Homebrew prefix", jdtlsName)
}

func brewPrefix(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "brew", "--prefix").Output()

	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

func (jlsd *JavaLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := jdtlsPath(ctx)

	if err != nil {
		return nil, err
	}

	// jdtls keeps per-workspace index state in -data and refuses a directory
	// another instance holds, so each server gets a fresh one
	dataDir, err := os.MkdirTemp("", "captn-jdtls-")

	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, execPath, "-data", dataDir)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return lsp.NewServerProcess(stdout, stdin, cmd.Wait, func() error {
		return cmd.Process.Kill()
	}), nil
}

func (jlsd *JavaLanguageSupportDefinition) NormalizeDefinitionRange(_ *common.Source, r *common.FileRange) *common.FileRange {
	return r
}

func (jlsd *JavaLanguageSupportDefinition) GetLSPInitializationOptions(_ context.Context, _ string) any {
	return nil
}
