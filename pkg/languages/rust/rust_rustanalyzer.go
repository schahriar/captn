package languages_rust

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
)

const (
	rustAnalyzerName           = "rust-analyzer"
	rustAnalyzerInstallCommand = "rustup component add rust-analyzer rust-src"
)

func rustAnalyzerPath(ctx context.Context) (string, error) {
	binary := rustAnalyzerName

	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}

	// rustup installs the component into the active toolchain rather than onto
	// PATH, so rustup is asked where it landed instead of asking for an
	// install that already happened.
	out, err := exec.CommandContext(ctx, "rustup", "which", rustAnalyzerName).Output()

	if err == nil {
		if path := strings.TrimSpace(string(out)); path != "" {
			return path, nil
		}
	}

	return "", fmt.Errorf("%v was not found in PATH or the active rustup toolchain", rustAnalyzerName)
}

func (rlsd *RustLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           rustAnalyzerName,
		InstallCommand: rustAnalyzerInstallCommand,
		Locate:         rustAnalyzerPath,
	}
}

func (rlsd *RustLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := rustAnalyzerPath(ctx)

	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, execPath)
	cmd.Stderr = os.Stderr

	// rust-analyzer shells out to cargo for workspace metadata; a component
	// resolved through rustup sits next to its toolchain's cargo, which is not
	// necessarily on PATH
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(execPath)+string(os.PathListSeparator)+os.Getenv("PATH"))

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

// NormalizeDefinitionRange answers unchanged: rust-analyzer already spans the
// identifier in a textDocument/definition reply.
func (rlsd *RustLanguageSupportDefinition) NormalizeDefinitionRange(_ *common.Source, r *common.FileRange) *common.FileRange {
	return r
}

// rust-analyzer loads the cargo project at the workspace root and never
// searches below it, so a crate nested deeper in the workspace goes
// unresolved; linkedProjects hands it every manifest under the workspace
func (rlsd *RustLanguageSupportDefinition) GetLSPInitializationOptions(_ context.Context, workspace string) any {
	var manifests []string

	filepath.WalkDir(workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			name := d.Name()

			if path != workspace && (strings.HasPrefix(name, ".") || name == "target" || name == "node_modules" || name == "vendor") {
				return filepath.SkipDir
			}

			return nil
		}

		if d.Name() == "Cargo.toml" {
			manifests = append(manifests, path)
		}

		return nil
	})

	if len(manifests) == 0 {
		return nil
	}

	sort.Strings(manifests)

	return map[string]any{
		"linkedProjects": manifests,
	}
}
