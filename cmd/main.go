package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/trace"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-yaml"
	"github.com/schahriar/captn/pkg/cog"
)

var CLI struct {
	Run struct {
		Main string `arg:"" name:"path" help:"Path to main file" type:"path"`
	} `cmd:"" help:"Evaluate from main file"`
}

func main() {
	cli := kong.Parse(&CLI)
	switch cli.Command() {
	case "run <path>":
		f, err := os.Create("trace.out")
		if err != nil {
			log.Fatalf("failed to create trace output file: %v", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Fatalf("failed to close trace file: %v", err)
			}
		}()

		if err := trace.Start(f); err != nil {
			log.Fatalf("failed to start trace: %v", err)
		}

		defer trace.Stop()

		ctx := context.Background()

		ctx, task := trace.NewTask(ctx, "loadFile")
		cwd, err := os.Getwd()

		if err != nil {
			panic(fmt.Errorf("invalid CWD (current working directory) %w", err))
		}

		g := cog.NewCOG(cwd)

		file, err := g.LoadFile(ctx, cli.Args[1])
		if err != nil {
			panic(err)
		}

		task.End()

		dbg := file.Module.Debug()
		ser, _ := yaml.Marshal(dbg)

		os.WriteFile("./out.yaml", ser, 0655)
	default:
		panic(cli.Command())
	}
}
