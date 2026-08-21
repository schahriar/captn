package languages_css

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
	cssServerName               = "vscode-css-language-server"
	vscodeServersInstallCommand = "npm install -g vscode-langservers-extracted"
)

func cssServerPath(ctx context.Context) (string, error) {
	// LookPath also resolves npm's .cmd shims on Windows via PATHEXT
	if path, err := exec.LookPath(cssServerName); err == nil {
		return path, nil
	}

	// `npm install -g` drops binaries somewhere that is often not on PATH,
	// so the global install directory is checked too rather than asking for
	// an install that already happened.
	if dir := npmGlobalBinDir(ctx); dir != "" {
		candidates := []string{cssServerName}

		if runtime.GOOS == "windows" {
			candidates = []string{cssServerName + ".cmd", cssServerName + ".exe"}
		}

		for _, name := range candidates {
			candidate := filepath.Join(dir, name)

			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("%v was not found in PATH or the npm global bin directory", cssServerName)
}

// npmGlobalBinDir asks npm where global binaries land: bin/ under the prefix
// on Unix, the prefix itself on Windows. npm is asked rather than guessing
// because the prefix moves with nvm and per-user configuration.
func npmGlobalBinDir(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "npm", "prefix", "-g").Output()

	if err != nil {
		return ""
	}

	prefix := strings.TrimSpace(string(out))

	if prefix == "" {
		return ""
	}

	if runtime.GOOS == "windows" {
		return prefix
	}

	return filepath.Join(prefix, "bin")
}

// NormalizeDefinitionRange answers unchanged: the vscode css server spans the
// declared name in a textDocument/definition reply.
func (clsd *CSSLanguageSupportDefinition) NormalizeDefinitionRange(_ *common.Source, r *common.FileRange) *common.FileRange {
	return r
}
