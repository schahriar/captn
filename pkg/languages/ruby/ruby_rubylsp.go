package languages_ruby

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
	rubyLSPName           = "ruby-lsp"
	rubyLSPInstallCommand = "gem install ruby-lsp"
)

func rubyLSPPath(ctx context.Context) (string, error) {
	// LookPath also resolves RubyGems' .bat wrappers on Windows via PATHEXT
	if path, err := exec.LookPath(rubyLSPName); err == nil {
		return path, nil
	}

	// `gem install` drops binstubs somewhere that is often not on PATH, so
	// the interpreter is asked where rather than asking for an install that
	// already happened. The binstub pins its own interpreter, so finding it
	// is enough to run it.
	if dir := gemBinDir(ctx); dir != "" {
		candidates := []string{rubyLSPName}

		if runtime.GOOS == "windows" {
			candidates = []string{rubyLSPName + ".bat", rubyLSPName + ".cmd"}
		}

		for _, name := range candidates {
			candidate := filepath.Join(dir, name)

			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("%v was not found in PATH or the gem executable directory", rubyLSPName)
}

func gemBinDir(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "ruby", "-e", "print Gem.bindir").Output()

	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// NormalizeDefinitionRange passes ruby-lsp's name-extent answers through with
// two repairs: a definition into an RBS signature stub (where every core
// method like new or upcase resolves) collapses to zero width since captn
// cannot parse .rbs and zero width is the "nothing to link" shape, and an
// attr_accessor-family answer names `label` while the declared token is
// `:label`, so a range preceded by a single colon widens to contain it.
func (rlsd *RubyLanguageSupportDefinition) NormalizeDefinitionRange(src *common.Source, r *common.FileRange) *common.FileRange {
	if src == nil || r == nil {
		return r
	}

	if filepath.Ext(src.Path) == ".rbs" {
		return common.NewFileRange(r.Source, r.Start, r.Start)
	}

	at := r.Start.BytePosition

	if r.Start.Column > 0 && at > 0 && at <= len(src.Buffer) &&
		src.Buffer[at-1] == ':' && (at < 2 || src.Buffer[at-2] != ':') {
		start := r.Start
		start.Column--
		start.BytePosition--

		return common.NewFileRange(r.Source, start, r.End)
	}

	return r
}

func (rlsd *RubyLanguageSupportDefinition) GetLSPInitializationOptions(_ context.Context, _ string) any {
	return nil
}
