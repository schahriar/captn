package initialize

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/schahriar/captn/pkg/providers"
	"github.com/schahriar/captn/pkg/queries"
	"github.com/schahriar/captn/pkg/server"
)

// TODO: Make provider agnostic

func CreateClaudeCommand(claudeArgs []string) *exec.Cmd {
	args := []string{"--append-system-prompt", fmt.Sprintf(`
Instead of calling Read tool, use the "captn" binary.
Replace all calls to grep and reads with "captn search <path> <snippet> <queryId>"
Where path can be a glob pattern like *.go or a specific file path, and snippet is the code snippet of interest.
%s
captn responses include fileRanges for the code it observed, serialized as "<filePath>:<startLine>:<startColumn>-<endLine>:<endColumn>"
where filePath is relative to the working directory and lines and columns are 1-based, e.g. "pkg/mcp/tool.go:16:1-21:2"
means lines 16 through 21 of pkg/mcp/tool.go. filePath may itself contain ':' or '-', so parse positions from the right.
You may use a fileRange to Read an exact region or to scope further commands when you need the raw source (e.g. to make an edit),
but prefer running captn again with another queryId over reading the files yourself.
If captn reports that a language server is missing, run the install command it names with Bash and then re-run the captn command.
YOU SHOULD NEVER USE grep directly anymore, just use captn
		`, providers.QueryRoutingSystemPrompt(queries.Supported()))}
	args = append(args, claudeArgs...)

	cmd := exec.Command("claude", args...)
	cmd.Env = providers.ClaudeEnv()

	return cmd
}

func Init(claudeArgs []string) error {
	ctx := context.Background()
	srv := server.NewServer()
	if err := srv.Listen(); err != nil {
		return fmt.Errorf("failed to start captn server: %v", err)
	}
	defer srv.Close()

	srv.Serve(ctx)

	cmd := CreateClaudeCommand(claudeArgs)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run claude command: %v", err)
	}

	return nil
}
