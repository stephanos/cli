package temporalcli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper functions

// createExecutable creates an executable file in the given directory.
func createExecutable(t *testing.T, dir, name, content string) string {
	t.Helper()

	var fullPath string
	var scriptContent string

	if runtime.GOOS == "windows" {
		fullPath = filepath.Join(dir, name+".bat")
		scriptContent = "@echo off\r\n" + content
	} else {
		fullPath = filepath.Join(dir, name)
		scriptContent = "#!/bin/bash\n" + content
	}

	err := os.WriteFile(fullPath, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("Failed to create executable %s: %v", fullPath, err)
	}

	return fullPath
}

// createNonExecutable creates a non-executable file (for negative testing).
func createNonExecutable(t *testing.T, dir, name string) string {
	t.Helper()

	fullPath := filepath.Join(dir, name)
	err := os.WriteFile(fullPath, []byte("not executable"), 0644)
	if err != nil {
		t.Fatalf("Failed to create non-executable file %s: %v", fullPath, err)
	}

	return fullPath
}

// setupTestPATH sets the PATH environment variable to the given directories
// and returns a cleanup function that restores the original PATH.
func setupTestPATH(t *testing.T, paths ...string) func() {
	t.Helper()

	oldPath := os.Getenv("PATH")

	separator := ":"
	if runtime.GOOS == "windows" {
		separator = ";"
	}

	newPath := strings.Join(paths, separator)
	err := os.Setenv("PATH", newPath)
	if err != nil {
		t.Fatalf("Failed to set PATH: %v", err)
	}

	return func() {
		os.Setenv("PATH", oldPath)
	}
}

// TestLookupExtension tests the extension lookup functionality.
func TestLookupExtension(t *testing.T) {
	tests := []struct {
		name              string
		setup             func(t *testing.T) func()
		commandTokens     []string
		expectFound       bool
		expectedExtName   string
		expectedRemaining []string
	}{
		{
			name: "most specific match wins",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "echo 'foo'")
				createExecutable(t, tempDir, "temporal-foo-bar", "echo 'foo bar'")
				createExecutable(t, tempDir, "temporal-foo-bar-baz", "echo 'foo bar baz'")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:     []string{"foo", "bar", "baz"},
			expectFound:       true,
			expectedExtName:   "temporal-foo-bar-baz",
			expectedRemaining: []string{},
		},
		{
			name: "fallback to less specific",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "echo 'foo'")
				createExecutable(t, tempDir, "temporal-foo-bar", "echo 'foo bar'")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:     []string{"foo", "bar", "baz", "qux"},
			expectFound:       true,
			expectedExtName:   "temporal-foo-bar",
			expectedRemaining: []string{"baz", "qux"},
		},
		{
			name: "dash in token becomes underscore in executable name",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo-bar_baz", "echo 'test'")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:     []string{"foo", "bar-baz"},
			expectFound:       true,
			expectedExtName:   "temporal-foo-bar_baz",
			expectedRemaining: []string{},
		},
		{
			name: "no match found",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "echo 'foo'")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:     []string{"bar", "baz"},
			expectFound:       false,
			expectedExtName:   "",
			expectedRemaining: nil,
		},
		{
			name: "empty command tokens",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "echo 'foo'")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:     []string{},
			expectFound:       false,
			expectedExtName:   "",
			expectedRemaining: nil,
		},
		{
			name: "real world scenario with arguments",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-cloud-namespace-list", "echo 'list'")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:     []string{"cloud", "namespace", "list", "--profile", "prod"},
			expectFound:       true,
			expectedExtName:   "temporal-cloud-namespace-list",
			expectedRemaining: []string{"--profile", "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup(t)
			defer cleanup()

			ext, remaining := lookupExtension(tt.commandTokens)

			if tt.expectFound {
				require.NotNil(t, ext, "Expected to find extension")
				assert.Equal(t, tt.expectedExtName, ext.Name, "Extension name mismatch")
				assert.Equal(t, tt.expectedRemaining, remaining, "Remaining arguments mismatch")
			} else {
				assert.Nil(t, ext, "Should not find extension")
				assert.Nil(t, remaining, "Remaining should be nil")
			}
		})
	}
}

// TestExecuteExtension tests basic extension execution scenarios.
func TestExecuteExtension(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(t *testing.T) (*extension, []string, func())
		expectedExitCode int
		expectError      bool
	}{
		{
			name: "successful execution",
			setup: func(t *testing.T) (*extension, []string, func()) {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-test", "exit 0")
				cleanup := setupTestPATH(t, tempDir)
				extensions := discoverExtensions()
				require.Len(t, extensions, 1)
				return &extensions[0], []string{}, cleanup
			},
			expectedExitCode: 0,
			expectError:      false,
		},
		{
			name: "non-zero exit code",
			setup: func(t *testing.T) (*extension, []string, func()) {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-test", "exit 42")
				cleanup := setupTestPATH(t, tempDir)
				extensions := discoverExtensions()
				require.Len(t, extensions, 1)
				return &extensions[0], []string{}, cleanup
			},
			expectedExitCode: 42,
			expectError:      false,
		},
		{
			name: "extension with arguments",
			setup: func(t *testing.T) (*extension, []string, func()) {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-test", "exit $#")
				cleanup := setupTestPATH(t, tempDir)
				extensions := discoverExtensions()
				require.Len(t, extensions, 1)
				return &extensions[0], []string{"arg1", "arg2", "arg3"}, cleanup
			},
			expectedExitCode: 3,
			expectError:      false,
		},
		{
			name: "nil extension returns error",
			setup: func(t *testing.T) (*extension, []string, func()) {
				return nil, []string{}, func() {}
			},
			expectedExitCode: 1,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, args, cleanup := tt.setup(t)
			defer cleanup()

			ctx := context.Background()
			ioConfig := newStdIOConfig()
			result, err := executeExtension(ctx, ext, args, ioConfig)

			if tt.expectError {
				assert.Error(t, err, "Expected error")
			} else {
				assert.NoError(t, err, "Unexpected error")
				require.NotNil(t, result, "Result should not be nil")
				assert.Equal(t, tt.expectedExitCode, result.ExitCode, "Exit code mismatch")
			}
		})
	}
}

// TestExecuteExtensionWithOutput tests extension execution with output capture.
func TestExecuteExtensionWithOutput(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(t *testing.T) (*extension, []string, func())
		stdin            string
		expectedStdout   string
		expectedExitCode int
		expectError      bool
	}{
		{
			name: "capture stdout",
			setup: func(t *testing.T) (*extension, []string, func()) {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-test", "echo 'Hello from extension'")
				cleanup := setupTestPATH(t, tempDir)
				extensions := discoverExtensions()
				require.Len(t, extensions, 1)
				return &extensions[0], []string{}, cleanup
			},
			expectedStdout:   "Hello from extension",
			expectedExitCode: 0,
			expectError:      false,
		},
		{
			name: "capture arguments in output",
			setup: func(t *testing.T) (*extension, []string, func()) {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-test", "echo \"Args: $@\"")
				cleanup := setupTestPATH(t, tempDir)
				extensions := discoverExtensions()
				require.Len(t, extensions, 1)
				return &extensions[0], []string{"foo", "bar"}, cleanup
			},
			expectedStdout:   "Args: foo bar",
			expectedExitCode: 0,
			expectError:      false,
		},
		{
			name: "capture exit code with output",
			setup: func(t *testing.T) (*extension, []string, func()) {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-test", "echo 'Error occurred'; exit 1")
				cleanup := setupTestPATH(t, tempDir)
				extensions := discoverExtensions()
				require.Len(t, extensions, 1)
				return &extensions[0], []string{}, cleanup
			},
			expectedStdout:   "Error occurred",
			expectedExitCode: 1,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, args, cleanup := tt.setup(t)
			defer cleanup()

			ctx := context.Background()
			var stdin io.Reader
			if tt.stdin != "" {
				stdin = strings.NewReader(tt.stdin)
			}

			ioConfig, stdout, _ := newCaptureIOConfig(stdin)
			result, err := executeExtension(ctx, ext, args, ioConfig)

			if tt.expectError {
				assert.Error(t, err, "Expected error")
			} else {
				assert.NoError(t, err, "Unexpected error")
				require.NotNil(t, result, "Result should not be nil")
				assert.Equal(t, tt.expectedExitCode, result.ExitCode, "Exit code mismatch")
			}
			assert.Contains(t, stdout.String(), tt.expectedStdout, "Stdout mismatch")
		})
	}
}

// TestExecuteExtensionTimeout tests timeout handling via context.
func TestExecuteExtensionTimeout(t *testing.T) {
	tempDir := t.TempDir()
	createExecutable(t, tempDir, "temporal-sleep", "sleep 5")
	cleanup := setupTestPATH(t, tempDir)
	defer cleanup()

	extensions := discoverExtensions()
	require.Len(t, extensions, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	ioConfig := newStdIOConfig()
	result, err := executeExtension(ctx, &extensions[0], []string{}, ioConfig)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 2*time.Second, "Should timeout quickly")
	require.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Timeout, "Should be marked as timeout")
	assert.NotEqual(t, 0, result.ExitCode, "Should have non-zero exit code on timeout")
	assert.NoError(t, err, "Timeout is not an error, just non-zero exit")
}

// TestTryExecuteExtension tests the integrated discover-match-execute flow.
func TestTryExecuteExtension(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(t *testing.T) func()
		commandTokens    []string
		expectedExitCode int
		expectFound      bool
	}{
		{
			name: "successful execution of matched extension",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "exit 0")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:    []string{"foo"},
			expectedExitCode: 0,
			expectFound:      true,
		},
		{
			name: "execute with fallback matching",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "exit 0")
				createExecutable(t, tempDir, "temporal-foo-bar", "exit 10")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:    []string{"foo", "bar", "baz"},
			expectedExitCode: 10,
			expectFound:      true,
		},
		{
			name: "no extensions found",
			setup: func(t *testing.T) func() {
				return setupTestPATH(t, "")
			},
			commandTokens: []string{"foo"},
			expectFound:   false,
		},
		{
			name: "no matching extension",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "exit 0")
				return setupTestPATH(t, tempDir)
			},
			commandTokens: []string{"bar"},
			expectFound:   false,
		},
		{
			name: "multi-level extension with args",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-cloud-namespace-list", "exit $#")
				return setupTestPATH(t, tempDir)
			},
			commandTokens:    []string{"cloud", "namespace", "list", "--profile", "prod"},
			expectedExitCode: 2,
			expectFound:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup(t)
			defer cleanup()

			ctx := context.Background()
			result, found := tryExecuteExtension(ctx, tt.commandTokens, nil)

			assert.Equal(t, tt.expectFound, found, "Found mismatch")

			if tt.expectFound {
				require.NotNil(t, result, "Result should not be nil when found")
				assert.Equal(t, tt.expectedExitCode, result.ExitCode, "Exit code mismatch")
				assert.NoError(t, result.Error, "Unexpected error")
				require.NotNil(t, result.Extension, "Extension should be set")
			} else {
				assert.Nil(t, result, "Result should be nil when not found")
			}
		})
	}
}

// TestDiscoverExtensions tests the extension discovery functionality.
func TestDiscoverExtensions(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) func()
		expectedCount int
		expectedNames []string
	}{
		{
			name: "single extension",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "exit 0")
				return setupTestPATH(t, tempDir)
			},
			expectedCount: 1,
			expectedNames: []string{"temporal-foo"},
		},
		{
			name: "multiple extensions",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "exit 0")
				createExecutable(t, tempDir, "temporal-bar", "exit 0")
				return setupTestPATH(t, tempDir)
			},
			expectedCount: 2,
			expectedNames: []string{"temporal-foo", "temporal-bar"},
		},
		{
			name: "non-executable ignored",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-good", "exit 0")
				createNonExecutable(t, tempDir, "temporal-bad")
				return setupTestPATH(t, tempDir)
			},
			expectedCount: 1,
			expectedNames: []string{"temporal-good"},
		},
		{
			name: "empty PATH",
			setup: func(t *testing.T) func() {
				return setupTestPATH(t, "")
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup(t)
			defer cleanup()

			extensions := discoverExtensions()
			assert.Len(t, extensions, tt.expectedCount)

			if tt.expectedCount > 0 {
				names := make(map[string]bool)
				for _, ext := range extensions {
					names[ext.Name] = true
				}
				for _, expected := range tt.expectedNames {
					assert.True(t, names[expected], "expected extension %s", expected)
				}
			}
		})
	}
}

// TestFormatExtensionsForHelp tests the extension formatting functionality.
func TestFormatExtensionsForHelp(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) func()
		expected []string
	}{
		{
			name: "formats and sorts",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-foo", "exit 0")
				createExecutable(t, tempDir, "temporal-bar", "exit 0")
				createExecutable(t, tempDir, "temporal-cloud-namespace-list", "exit 0")
				return setupTestPATH(t, tempDir)
			},
			expected: []string{"bar", "cloud namespace list", "foo"},
		},
		{
			name: "underscore to dash conversion",
			setup: func(t *testing.T) func() {
				tempDir := t.TempDir()
				createExecutable(t, tempDir, "temporal-workflow-show_diagram", "exit 0")
				return setupTestPATH(t, tempDir)
			},
			expected: []string{"workflow show-diagram"},
		},
		{
			name: "empty list",
			setup: func(t *testing.T) func() {
				return setupTestPATH(t, "")
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup(t)
			defer cleanup()

			extensions := discoverExtensions()
			formatted := formatExtensionsForHelp(extensions)
			assert.Equal(t, tt.expected, formatted)
		})
	}
}

// TestExtensionIntegration tests that extensions are only used when commands are not found
func TestExtensionIntegration(t *testing.T) {
	tests := []struct {
		name           string
		setupExtension func(t *testing.T, dir string) string
		args           []string
		expectError    bool
		expectOutput   string
		expectExtRan   bool
	}{
		{
			name: "built-in command runs normally",
			args: []string{"--version"},
			setupExtension: func(t *testing.T, dir string) string {
				return ""
			},
			expectError:  false,
			expectExtRan: false,
		},
		{
			name: "extension runs for unknown command",
			args: []string{"myext", "arg1"},
			setupExtension: func(t *testing.T, dir string) string {
				extPath := filepath.Join(dir, "temporal-myext")
				content := "#!/bin/sh\necho 'Extension ran'\n"
				err := os.WriteFile(extPath, []byte(content), 0755)
				require.NoError(t, err)
				return dir
			},
			expectError:  false,
			expectOutput: "Extension ran",
			expectExtRan: true,
		},
		{
			name: "extension cannot override built-in workflow",
			args: []string{"workflow"},
			setupExtension: func(t *testing.T, dir string) string {
				extPath := filepath.Join(dir, "temporal-workflow")
				content := "#!/bin/sh\necho 'Extension workflow'\n"
				err := os.WriteFile(extPath, []byte(content), 0755)
				require.NoError(t, err)
				return dir
			},
			expectError:  true,
			expectExtRan: false,
		},
		{
			name: "extension can add workflow subcommand",
			args: []string{"workflow", "diagram"},
			setupExtension: func(t *testing.T, dir string) string {
				extPath := filepath.Join(dir, "temporal-workflow-diagram")
				content := "#!/bin/sh\necho 'Workflow diagram'\n"
				err := os.WriteFile(extPath, []byte(content), 0755)
				require.NoError(t, err)
				return dir
			},
			expectError:  false,
			expectOutput: "Workflow diagram",
			expectExtRan: true,
		},
		{
			name: "extension cannot override activity",
			args: []string{"activity"},
			setupExtension: func(t *testing.T, dir string) string {
				extPath := filepath.Join(dir, "temporal-activity")
				content := "#!/bin/sh\necho 'Extension activity'\n"
				err := os.WriteFile(extPath, []byte(content), 0755)
				require.NoError(t, err)
				return dir
			},
			expectError:  true,
			expectExtRan: false,
		},
		{
			name: "no extension found for unknown command",
			args: []string{"nonexistent", "command"},
			setupExtension: func(t *testing.T, dir string) string {
				return dir
			},
			expectError:  true,
			expectExtRan: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			extDir := tt.setupExtension(t, tmpDir)

			oldPath := os.Getenv("PATH")
			if extDir != "" {
				os.Setenv("PATH", extDir+string(os.PathListSeparator)+oldPath)
			}
			defer os.Setenv("PATH", oldPath)

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			var failErr error
			failCalled := false

			ctx := context.Background()
			Execute(ctx, CommandOptions{
				Args:   tt.args,
				Stdin:  os.Stdin,
				Stdout: stdout,
				Stderr: stderr,
				Fail: func(err error) {
					failErr = err
					failCalled = true
				},
			})

			if tt.expectError {
				assert.True(t, failCalled, "Expected Fail() to be called")
			} else {
				assert.False(t, failCalled, "Expected Fail() not to be called, but got error: %v", failErr)
			}

			if tt.expectOutput != "" {
				output := stdout.String() + stderr.String()
				assert.Contains(t, output, tt.expectOutput)
			}
		})
	}
}

// TestExtensionExitCode tests that extension exit codes are properly handled
func TestExtensionExitCode(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		exitCode    int
		expectError bool
	}{
		{
			name:        "extension succeeds with exit 0",
			exitCode:    0,
			expectError: false,
		},
		{
			name:        "extension fails with exit 1",
			exitCode:    1,
			expectError: true,
		},
		{
			name:        "extension fails with exit 42",
			exitCode:    42,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extPath := filepath.Join(tmpDir, "temporal-exitext")
			content := "#!/bin/sh\nexit " + string(rune('0'+tt.exitCode)) + "\n"
			err := os.WriteFile(extPath, []byte(content), 0755)
			require.NoError(t, err)

			oldPath := os.Getenv("PATH")
			os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
			defer os.Setenv("PATH", oldPath)

			failCalled := false

			ctx := context.Background()
			Execute(ctx, CommandOptions{
				Args:   []string{"exitext"},
				Stdin:  os.Stdin,
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
				Fail: func(err error) {
					failCalled = true
				},
			})

			if tt.expectError {
				assert.True(t, failCalled, "Expected Fail() to be called")
			} else {
				assert.False(t, failCalled, "Expected Fail() not to be called")
			}

			os.Remove(extPath)
		})
	}
}
