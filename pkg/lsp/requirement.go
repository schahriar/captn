package lsp

import (
	"context"
	"errors"
)

// ErrServerMissing marks the one failure that is not specific to the code being
// read: it fails every file of that language identically, so a caller fanning
// out over matches reports it instead of quietly resolving nothing.
var ErrServerMissing = errors.New("missing language server")

// ServerRequirement describes the language server a language needs. captn never
// installs it: it locates the binary and hands the install command back to the
// agent driving it, which already owns the terminal and its own approval flow.
// Locate is the single answer to "which binary", used both to report a missing
// server and to start it, so the two cannot disagree.
type ServerRequirement struct {
	Name           string
	InstallCommand string
	Locate         func(ctx context.Context) (string, error)
}
