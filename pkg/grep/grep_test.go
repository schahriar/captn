package grep

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func requireGrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("grep"); err != nil {
		t.Fatal("grep not available on PATH")
	}
}

// writeTree lays out a small known source tree in a temp dir and returns its root.
func writeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"alpha.go":         "package alpha\n\nfunc Hello() string { return \"needle\" }\n",
		"sub/beta.go":      "package beta\n\n// needle lives here too\nvar X = 1\n",
		"sub/gamma.txt":    "needle in a text file should be excluded by --include\n",
		"sub/colon.go":     "x := map[string]int{\"a:b\": 1} // needle with colons: here\n",
		"unrelated/zed.go": "package zed\n\nvar Y = 2\n",
	}

	for rel, content := range files {
		path := filepath.Join(root, rel)
		assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		assert.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	return root
}

func TestSearchSource_RealGrep(t *testing.T) {
	requireGrep(t)
	root := writeTree(t)

	matches, err := SearchSource(context.Background(), root, "*.go", "needle")
	assert.NoError(t, err)

	// Only .go files: alpha.go, sub/beta.go, sub/colon.go — gamma.txt excluded.
	paths := map[string]Match{}
	for _, m := range matches {
		rel, err := filepath.Rel(root, m.Path)
		assert.NoError(t, err)
		paths[rel] = m
	}

	assert.Len(t, matches, 3)
	assert.Contains(t, paths, "alpha.go")
	assert.Contains(t, paths, filepath.Join("sub", "beta.go"))
	assert.Contains(t, paths, filepath.Join("sub", "colon.go"))
	assert.NotContains(t, paths, filepath.Join("sub", "gamma.txt"))

	// Line numbers and text survive the round trip.
	assert.Equal(t, 3, paths["alpha.go"].Line)
	assert.Contains(t, paths["alpha.go"].Text, "Hello")

	// A matched line that itself contains colons keeps its full text intact,
	// regardless of which output dialect the local grep used.
	colon := paths[filepath.Join("sub", "colon.go")]
	assert.Contains(t, colon.Text, "a:b")
	assert.Contains(t, colon.Text, "colons: here")
}

func TestSearch_RealGrep_NoIncludeFilter(t *testing.T) {
	requireGrep(t)
	root := writeTree(t)

	matches, err := Search(context.Background(), root, "needle")
	assert.NoError(t, err)

	// No include filter, so the .txt file is matched as well: 3 .go + 1 .txt.
	assert.Len(t, matches, 4)
}

func TestSearchSource_RealGrep_NoMatch(t *testing.T) {
	requireGrep(t)
	root := writeTree(t)

	matches, err := Search(context.Background(), root, "this-string-appears-nowhere")
	assert.NoError(t, err)
	assert.Empty(t, matches)
}

// TestParseGrepOutput_Dialects checks both wire formats against the same logical
// result. The NUL cases mirror what GNU grep (Linux / Git-for-Windows) emits
// with -Z, including Windows drive-letter paths; the colon cases mirror BSD grep.
func TestParseGrepOutput_Dialects(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Match
	}{
		{
			name: "gnu nul delimited",
			in:   "src/main.go\x0012:func main() {\n/abs/util.go\x007:return 1\n",
			want: []Match{
				NewMatch("src/main.go", 12, "func main() {"),
				NewMatch("/abs/util.go", 7, "return 1"),
			},
		},
		{
			name: "gnu nul windows drive path",
			in:   "C:\\src\\main.go\x0042:fmt.Println(\"hi\")\n",
			want: []Match{
				NewMatch("C:\\src\\main.go", 42, "fmt.Println(\"hi\")"),
			},
		},
		{
			name: "gnu nul text contains colons",
			in:   "a/b.go\x009:m := map[string]int{\"k:v\": 1}\n",
			want: []Match{
				NewMatch("a/b.go", 9, "m := map[string]int{\"k:v\": 1}"),
			},
		},
		{
			name: "bsd colon delimited",
			in:   "src/main.go:12:func main() {\nutil.go:7:return 1\n",
			want: []Match{
				NewMatch("src/main.go", 12, "func main() {"),
				NewMatch("util.go", 7, "return 1"),
			},
		},
		{
			name: "bsd colon text contains colons",
			in:   "a/b.go:9:m := map[string]int{\"k:v\": 1}\n",
			want: []Match{
				NewMatch("a/b.go", 9, "m := map[string]int{\"k:v\": 1}"),
			},
		},
		{
			name: "bsd colon windows drive path",
			in:   "C:\\src\\main.go:42:x:y\n",
			want: []Match{
				NewMatch("C:\\src\\main.go", 42, "x:y"),
			},
		},
		{
			name: "empty input",
			in:   "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGrepOutput(strings.NewReader(tt.in))
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
