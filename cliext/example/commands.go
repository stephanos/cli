package cliextexample

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// CommandOptions contains the options for running the CLI.
type CommandOptions struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Fail   func(error)
}

// CommandContext contains the context for command execution.
type CommandContext struct {
	context.Context
	Options CommandOptions
}

// Execute runs the CLI.
func Execute(ctx context.Context, options CommandOptions) {
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	if options.Fail == nil {
		options.Fail = func(err error) {
			fmt.Fprintln(options.Stderr, err)
			os.Exit(1)
		}
	}

	cctx := &CommandContext{
		Context: ctx,
		Options: options,
	}

	cmd := NewRootCommand(cctx)
	cmd.Command.SetArgs(options.Args)
	cmd.Command.SetOut(options.Stdout)
	cmd.Command.SetErr(options.Stderr)
	cmd.Command.SetIn(options.Stdin)

	if err := cmd.Command.ExecuteContext(ctx); err != nil {
		options.Fail(err)
	}
}

type RootCommand struct {
	Command cobra.Command
}

func NewRootCommand(cctx *CommandContext) *RootCommand {
	var s RootCommand
	s.Command.Use = "example"
	s.Command.Short = "Example CLI extension"
	s.Command.Args = cobra.NoArgs
	s.Command.AddCommand(&NewHelloCommand(cctx).Command)
	return &s
}

type HelloCommand struct {
	Command cobra.Command
}

func NewHelloCommand(cctx *CommandContext) *HelloCommand {
	var s HelloCommand
	s.Command.Use = "hello"
	s.Command.Short = "Print hello world message"
	s.Command.Args = cobra.NoArgs
	s.Command.Run = func(c *cobra.Command, args []string) {
		fmt.Fprintln(cctx.Options.Stdout, "Hello, World!")
	}
	return &s
}
