package temporalcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const (
	// extensionPrefix is the required prefix for all Temporal CLI extensions.
	extensionPrefix = "temporal-"
)

// extension represents a discovered CLI extension.
type extension struct {
	// Name is the base name of the extension executable (e.g., "temporal-foo").
	Name string

	// Path is the full path to the extension executable.
	Path string

	// CommandTokens are the command parts extracted from the name.
	// For "temporal-foo-bar", this would be ["foo", "bar"].
	CommandTokens []string
}

// extensionIOConfig holds the I/O configuration for extension execution.
type extensionIOConfig struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// newStdIOConfig creates an extensionIOConfig that uses standard I/O streams.
func newStdIOConfig() *extensionIOConfig {
	return &extensionIOConfig{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// newCaptureIOConfig creates an extensionIOConfig that captures stdout and stderr.
func newCaptureIOConfig(stdin io.Reader) (*extensionIOConfig, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if stdin == nil {
		stdin = os.Stdin
	}

	return &extensionIOConfig{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}, stdout, stderr
}

// executeResult holds detailed results from executing an extension.
type executeResult struct {
	ExitCode int           // Process exit code (0 = success)
	Timeout  bool          // True if the process was killed due to context deadline
	Duration time.Duration // How long the process ran
}

// executeExtensionResult holds the result of an extension execution attempt.
type executeExtensionResult struct {
	Extension *extension    // nil if no extension was found
	ExitCode  int
	Timeout   bool          // True if the process was killed due to context deadline
	Duration  time.Duration // How long the process ran
	Error     error
}

// lookupExtension searches for an extension executable by command tokens.
// It performs most-specific to least-specific matching using exec.LookPath.
//
// For example, given command tokens ["foo", "bar-baz", "qux"], it will try:
//  1. temporal-foo-bar_baz-qux
//  2. temporal-foo-bar_baz
//  3. temporal-foo
//
// Returns the matched extension and remaining arguments, or (nil, nil) if no match.
func lookupExtension(commandTokens []string) (*extension, []string) {
	if len(commandTokens) == 0 {
		return nil, nil
	}

	// Try matching from most specific to least specific
	for i := len(commandTokens); i > 0; i-- {
		tokensToMatch := commandTokens[:i]
		remaining := commandTokens[i:]

		// Build executable name: temporal-foo-bar_baz
		// Tokens are joined with "-", dashes within tokens become underscores
		parts := make([]string, len(tokensToMatch))
		for j, token := range tokensToMatch {
			parts[j] = strings.ReplaceAll(token, "-", "_")
		}
		name := extensionPrefix + strings.Join(parts, "-")

		// Use exec.LookPath to find the executable
		path, err := exec.LookPath(name)
		if err == nil {
			return &extension{
				Name:          name,
				Path:          path,
				CommandTokens: tokensToMatch,
			}, remaining
		}
	}

	return nil, nil
}

// executeExtension executes an extension binary with the provided I/O configuration.
func executeExtension(ctx context.Context, ext *extension, args []string, ioConfig *extensionIOConfig) (*executeResult, error) {
	if ext == nil {
		return nil, fmt.Errorf("extension cannot be nil")
	}

	if ioConfig == nil {
		ioConfig = newStdIOConfig()
	}

	cmd := exec.CommandContext(ctx, ext.Path, args...)

	// Wire up I/O
	cmd.Stdin = ioConfig.Stdin
	cmd.Stdout = ioConfig.Stdout
	cmd.Stderr = ioConfig.Stderr

	// Run the command and measure duration
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &executeResult{
		Duration: duration,
	}

	if err != nil {
		// Check if it was a timeout (deadline exceeded)
		if ctx.Err() == context.DeadlineExceeded {
			result.Timeout = true
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = -1
			}
			return result, nil
		}

		// Check if context was canceled
		if errors.Is(err, context.Canceled) {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = -1
			}
			return result, fmt.Errorf("extension %s was canceled: %w", ext.Name, err)
		}

		// Extract exit code from error
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}

		// Other errors (e.g., command not found, permission denied)
		result.ExitCode = 1
		return result, fmt.Errorf("failed to execute extension %s: %w", ext.Name, err)
	}

	result.ExitCode = 0
	return result, nil
}

// tryExecuteExtension attempts to find and execute an extension for the given command tokens.
// Returns the result and true if an extension was found and executed, or (nil, false) if no
// extension matched the command tokens.
func tryExecuteExtension(ctx context.Context, commandTokens []string, ioConfig *extensionIOConfig) (*executeExtensionResult, bool) {
	// Lookup extension using exec.LookPath
	ext, remainingArgs := lookupExtension(commandTokens)
	if ext == nil {
		return nil, false
	}

	// Execute the extension with remaining args
	if ioConfig == nil {
		ioConfig = newStdIOConfig()
	}
	result, err := executeExtension(ctx, ext, remainingArgs, ioConfig)

	execResult := &executeExtensionResult{
		Extension: ext,
		Error:     err,
	}

	if result != nil {
		execResult.ExitCode = result.ExitCode
		execResult.Timeout = result.Timeout
		execResult.Duration = result.Duration
	}

	return execResult, true
}

// Execute runs the Temporal CLI with the given context and options. This
// intentionally does not return an error but rather invokes Fail on the
// options.
func Execute(ctx context.Context, options CommandOptions) {
	// Create context and run. We always get a context and cancel func back even
	// if an error was returned. This is so we can use the context to print an
	// error message using the appropriate Fail() method, regardless of why the
	// failure occurred.
	//
	// (In most cases, an error here likely means a problem with the user's env
	// config file, or some other issue in their environment.)
	cctx, cancel, err := NewCommandContext(ctx, options)
	defer cancel()

	if err == nil {
		// We have a context; let's actually run the command.
		cmd := NewTemporalCommand(cctx)
		cmd.Command.SetArgs(cctx.Options.Args)

		// Check if a built-in command exists for these args before executing
		_, foundArgs, cmdErr := cmd.Command.Find(cctx.Options.Args)

		// A built-in command exists if Find succeeds AND there are no remaining args
		// (remaining args would indicate an unknown subcommand)
		builtInCommandExists := cmdErr == nil && len(foundArgs) == 0

		err = cmd.Command.ExecuteContext(cctx)

		// If no built-in command exists for these args, try extensions
		// (Even if a command "ran" to show help, if it's not a real command, we should try extensions)
		if !builtInCommandExists && !cctx.ActuallyRanCommand {
			// Try to execute as extension
			ioConfig := &extensionIOConfig{
				Stdin:  cctx.Options.Stdin,
				Stdout: cctx.Options.Stdout,
				Stderr: cctx.Options.Stderr,
			}
			result, found := tryExecuteExtension(cctx, cctx.Options.Args, ioConfig)
			if found {
				// Extension was found and executed
				cctx.ActuallyRanCommand = true
				if result.ExitCode != 0 {
					// Extension failed, create error with exit code
					err = fmt.Errorf("extension exited with code %d", result.ExitCode)
				} else {
					// Extension succeeded, clear any error
					err = nil
				}
			}
			// If not found, keep the original error
		}
	}

	if err != nil {
		// Either we failed to create the context, OR the command itself failed.
		// Either way, we need to print an error message.
		cctx.Options.Fail(err)
	}

	// If no command ever actually got run, exit nonzero with an error.  This is
	// an ugly hack to make sure that iff the user explicitly asked for help, we
	// exit with a zero error code.  (The other situation in which help is
	// printed is when the user invokes an unknown command--we still want a
	// non-zero exit in that case.)  We should revisit this if/when the
	// following Cobra issues get fixed:
	//
	// - https://github.com/spf13/cobra/issues/1156
	// - https://github.com/spf13/cobra/issues/706
	if !cctx.ActuallyRanCommand {
		zeroExitArgs := []string{"--help", "-h", "--version", "-v", "help"}
		if slices.ContainsFunc(cctx.Options.Args, func(a string) bool {
			return slices.Contains(zeroExitArgs, a)
		}) {
			return
		}
		cctx.Options.Fail(fmt.Errorf("unknown command"))
	}
}
