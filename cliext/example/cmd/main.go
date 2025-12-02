package main

import (
	"context"
	"os"

	cliextexample "github.com/temporalio/cli/cliext/example"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cliextexample.Execute(ctx, cliextexample.CommandOptions{
		Args:   os.Args[1:],
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
}
