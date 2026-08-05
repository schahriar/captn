package initialize

import (
	"context"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/schahriar/captn/pkg/server"
	"github.com/schahriar/captn/pkg/tui"
)

func InitWithTUI(ctx context.Context, claudeArgs []string) error {
	srv := server.NewServer()
	if err := srv.Listen(); err != nil {
		log.Fatalf("failed to start captn server: %v", err)
	}
	defer srv.Close()

	cmd := CreateClaudeCommand(claudeArgs)

	overlay, err := tui.NewOverlay(cmd)
	if err != nil {
		return err
	}

	ctx = tui.WithStatusProvider(ctx, overlay)
	srv.Serve(ctx)

	go func() {
		time.Sleep(1000 * time.Millisecond)
		loader := tui.NewLoader()
		overlay.SetStatus(
			tui.Decorate(
				tui.Group(
					tui.Text(" captn "),
					loader,
				),
				tui.ShimmerColor(tui.NewRGB(70, 130, 220), tui.NewRGB(180, 220, 255)),
			),
		)

		overlay.SetSubStatus(
			tui.Group(
				tui.Text(tui.Dim("Ask anything and claude will coordinate with captn")),
			),
		)

		time.Sleep(5 * time.Second)

		overlay.SetSubStatus(tui.Group()) // Reset substatus after 5 seconds
		overlay.Hide()
	}()

	if err := overlay.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}
