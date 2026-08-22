package languages_c

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

const clangdName = "clangd"

// clangd ships with the platform toolchain rather than a language package
// manager, so the install command is the toolchain's
func clangdInstallCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "xcode-select --install"
	case "windows":
		return "winget install -e --id LLVM.LLVM"
	default:
		return "sudo apt-get install -y clangd"
	}
}

func clangdPath(ctx context.Context) (string, error) {
	binary := clangdName

	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}

	// Xcode and the Command Line Tools put clangd on xcrun's search path
	// rather than PATH
	if runtime.GOOS == "darwin" {
		out, err := exec.CommandContext(ctx, "xcrun", "--find", clangdName).Output()

		if err == nil {
			candidate := strings.TrimSpace(string(out))

			if info, serr := os.Stat(candidate); serr == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("%v was not found in PATH or the platform toolchain", clangdName)
}

// NormalizeDefinitionRange passes clangd's name-extent answers through with
// one repair: a definition into a header without a C extension (libc++'s
// <string>, libstdc++'s bits/*.tcc, where every std:: member resolves)
// collapses to zero width since captn cannot parse those files and zero
// width is the "nothing to link" shape. An #include already resolves to a
// zero-width position at the header's start.
func (clsd *CLanguageSupportDefinition) NormalizeDefinitionRange(src *common.Source, r *common.FileRange) *common.FileRange {
	if src == nil || r == nil {
		return r
	}

	switch filepath.Ext(src.Path) {
	case ".c", ".h", ".hpp", ".hh", ".hxx", ".cpp", ".cc", ".cxx":
		return r
	}

	return common.NewFileRange(r.Source, r.Start, r.Start)
}

func (clsd *CLanguageSupportDefinition) GetLSPInitializationOptions(_ context.Context, _ string) any {
	return nil
}
