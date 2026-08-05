package cog

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/schahriar/captn/pkg/lsp"
)

const locateServerTimeout = 30 * time.Second

var (
	locatedServersMu sync.Mutex
	locatedServers   = map[string]bool{}
)

// RequireLSPServer reports a missing language server as an error naming the
// command that installs it, leaving the install to the agent captn runs under
// rather than prompting over that agent's own screen.
func RequireLSPServer(ctx context.Context, req lsp.ServerRequirement) error {
	if req.Locate == nil {
		return nil
	}

	locatedServersMu.Lock()
	defer locatedServersMu.Unlock()

	// Only a located server is remembered, so a server installed mid-session puts
	// the next query back to work without restarting captn.
	if locatedServers[req.Name] {
		return nil
	}

	// Locating the binary is a local check, so it does not inherit a caller that
	// may already have gone away mid-query and would make an installed server
	// look missing.
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), locateServerTimeout)
	defer cancel()

	if _, err := req.Locate(lctx); err == nil {
		locatedServers[req.Name] = true
		return nil
	}

	command := strings.TrimSpace(req.InstallCommand)

	if command == "" {
		return fmt.Errorf("%w: %v is not installed and offers no install command", lsp.ErrServerMissing, req.Name)
	}

	return fmt.Errorf("%w: captn needs %v to read this code, run `%v` and try this command again", lsp.ErrServerMissing, req.Name, command)
}
