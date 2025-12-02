package temporalcli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHelpAll tests the "help --all" flag that shows extensions
func TestHelpAll(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		setupExtensions  func(t *testing.T, dir string)
		expectExtensions []string
		expectNoExt      bool
	}{
		{
			name: "help --all shows extensions",
			args: []string{"help", "--all"},
			setupExtensions: func(t *testing.T, dir string) {
				createExecutable(t, dir, "temporal-myext", "exit 0")
				createExecutable(t, dir, "temporal-another-cmd", "exit 0")
			},
			expectExtensions: []string{"myext", "another cmd"},
		},
		{
			name: "help without --all does not show extensions section",
			args: []string{"help"},
			setupExtensions: func(t *testing.T, dir string) {
				createExecutable(t, dir, "temporal-myext", "exit 0")
			},
			expectNoExt: true,
		},
		{
			name: "help --all with no extensions",
			args: []string{"help", "--all"},
			setupExtensions: func(t *testing.T, dir string) {
				// No extensions
			},
			expectNoExt: true,
		},
		{
			name: "help --all shows multi-level extensions",
			args: []string{"help", "--all"},
			setupExtensions: func(t *testing.T, dir string) {
				createExecutable(t, dir, "temporal-cloud-namespace-list", "exit 0")
			},
			expectExtensions: []string{"cloud namespace list"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setupExtensions(t, tmpDir)

			// Set PATH to include extension directory
			oldPath := os.Getenv("PATH")
			os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
			defer os.Setenv("PATH", oldPath)

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			ctx := context.Background()
			Execute(ctx, CommandOptions{
				Args:   tt.args,
				Stdin:  os.Stdin,
				Stdout: stdout,
				Stderr: stderr,
				Fail:   func(err error) {},
			})

			output := stdout.String()

			if tt.expectNoExt {
				assert.NotContains(t, output, "CLI Extensions:")
			} else {
				assert.Contains(t, output, "CLI Extensions:")
				for _, ext := range tt.expectExtensions {
					assert.Contains(t, output, ext)
				}
			}
		})
	}
}

// TestHelpExtension tests "help <extension>" showing extension help
func TestHelpExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// Create extension that responds to --help
	createExecutable(t, tmpDir, "temporal-myext", `
if [ "$1" = "--help" ]; then
    echo "My extension help text"
    exit 0
fi
exit 1
`)

	// Set PATH
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	ctx := context.Background()
	Execute(ctx, CommandOptions{
		Args:   []string{"help", "myext"},
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
		Fail:   func(err error) {},
	})

	output := stdout.String()
	assert.Contains(t, output, "My extension help text")
}

// TestHelpBuiltInCommand tests "help <builtin>" works normally
func TestHelpBuiltInCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	ctx := context.Background()
	Execute(ctx, CommandOptions{
		Args:   []string{"help", "workflow"},
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
		Fail:   func(err error) {},
	})

	output := stdout.String()
	// Should show workflow command help
	assert.Contains(t, output, "workflow")
}

// TestHelpUnknownCommand tests "help <unknown>" shows error
func TestHelpUnknownCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	ctx := context.Background()
	Execute(ctx, CommandOptions{
		Args:   []string{"help", "nonexistent"},
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
		Fail:   func(err error) {},
	})

	output := stderr.String()
	assert.Contains(t, output, "Unknown command")
}
