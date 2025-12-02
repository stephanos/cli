# CLI Extension Framework Review

## Executive Summary

This document reviews the `internal/cliext` package, analyzes its integration with the `auth-lib` branch, and proposes a holistic design for CLI extensions including OAuth support and a comprehensive testing strategy.

**Overall Assessment: 7.5/10** - Solid foundation with clear architecture, but needs refinement for production readiness.

---

## Part 1: Current State Analysis

### Architecture Overview

The `internal/cliext` package implements a plugin/extension pattern:

```
┌─────────────────┐     ┌──────────────┐     ┌─────────────────┐
│  temporal CLI   │────▶│   cliext     │────▶│ temporal-foo    │
│  (built-in)     │     │  framework   │     │ (extension)     │
└─────────────────┘     └──────────────┘     └─────────────────┘
        │                      │
        │                      ├── discovery.go  (find extensions in PATH)
        │                      ├── executor.go   (run extensions)
        │                      ├── help.go       (route help requests)
        │                      └── io_util.go    (I/O configuration)
        │
        └── Falls back to extension if no built-in command matches
```

### File Analysis

| File | Purpose | Lines | Test Coverage |
|------|---------|-------|---------------|
| `discovery.go` | Scan PATH for `temporal-*` executables | 270 | ✓ Good |
| `executor.go` | Run extensions as subprocesses | 175 | ✓ Good |
| `help.go` | Format and route help requests | 125 | ✓ Good |
| `io_util.go` | I/O configuration helpers | 53 | ✓ Good |
| `env.go` | Environment variable abstraction | 40 | ✓ **New** |
| `config.go` | Config file loading/writing | 90 | ✓ **New** |
| `profile.go` | Profile management | 85 | ✓ **New** |
| `context.go` | ExtensionContext for profile/config | 90 | ✓ **New** |
| `oauth.go` | OAuth token management | 175 | ✓ **New** |
| `integration_test.go` | Integration tests with example binary | 235 | ✓ Good |

### Critical Issues

#### Issue #1: Windows Executable Detection (HIGH) ✅ FIXED

**Location:** `discovery.go:121-124`

**Status:** Fixed. Now checks for Windows executable extensions (`.exe`, `.bat`, `.cmd`, `.com`).

```go
if runtime.GOOS == "windows" {
    ext := strings.ToLower(filepath.Ext(path))
    return ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".com" || ext == ""
}
```

#### Issue #2: Timeout vs Exit Code Ambiguity (HIGH) ✅ FIXED

**Location:** `executor.go:26-32, 54-78`

**Status:** Fixed. Added `ExecuteResult` struct with `Timeout` and `Duration` fields.

```go
type ExecuteResult struct {
    ExitCode int
    Timeout  bool
    Duration time.Duration
}
```

`ExecuteExtension()` now returns `(*ExecuteResult, error)` and properly tracks timeout vs normal exit.

#### Issue #3: Help Function Ignores IOConfig (MEDIUM) ✅ FIXED

**Location:** `help.go:84`

**Status:** Fixed. Added `ioConfig *IOConfig` parameter to `TryShowExtensionHelp()`.

```go
func TryShowExtensionHelp(ctx context.Context, commandTokens []string,
    timeout time.Duration, ioConfig *IOConfig) *ExecuteExtensionResult
```

If `ioConfig` is nil, falls back to standard I/O.

#### Issue #4: No Discovery Caching (PERFORMANCE) ✅ FIXED

**Location:** `discovery.go:133-168`

**Status:** Fixed. Added `DiscoverExtensionsCached()` and `ClearExtensionCache()`.

```go
// Returns cached extensions, auto-invalidates on PATH change
func DiscoverExtensionsCached() []Extension

// Manually clear cache when needed
func ClearExtensionCache()
```

Cache is thread-safe with RWMutex and automatically invalidates when PATH changes.

#### Issue #5: Silent Permission Errors (DEBUG)

**Location:** `discovery.go:67-71`

```go
if err != nil {
    continue  // Silent failure - no logging
}
```

**Problem:** Inaccessible PATH directories are silently skipped.

**Fix:** Add optional debug logging hook.

### Missing Functionality

| Feature | Impact | Priority |
|---------|--------|----------|
| Extension metadata (version, description) | Can't browse extensions | Medium |
| Environment variable control | Can't isolate extensions | Medium |
| Resource limits (memory, CPU) | Runaway extensions | Low |
| Signal forwarding | No graceful shutdown | Medium |
| Extension lifecycle hooks | No monitoring | Low |

### Test Gaps

**Missing test file:** `io_util_test.go` ✅ **Added**

**Missing test cases:**
- ~~Context cancellation vs timeout~~ ✅ Fixed with `ExecuteResult.Timeout`
- ~~Concurrent discovery calls~~ ✅ Fixed with thread-safe cache
- Extension with spaces in path
- Circular symlinks in PATH
- Very large stdout/stderr output
- Non-ASCII filenames

---

## Part 2: Auth-lib Integration ✅ COMPLETE

The `auth-lib` branch code has been integrated into `internal/cliext/`:

- `env.go` - EnvLookup interface + MapEnvLookup for testing
- `config.go` - LoadConfig/WriteConfig functions
- `profile.go` - LoadProfile with options pattern
- `context.go` - ExtensionContext for passing profile/config to extensions

The `auth-lib` branch adds a config/profile abstraction in a separate `cliext/` package (at repo root):

### New Components

```
cliext/
├── config.go   # LoadConfig(), WriteConfig()
├── profile.go  # LoadProfile()
├── env.go      # EnvLookup interface
├── go.mod      # Separate module
└── go.sum
```

### Design Pattern

```go
// Options/Result pattern for testability
type LoadConfigOptions struct {
    ConfigFilePath string
    EnvLookup      EnvLookup  // Dependency injection for testing
}

type LoadConfigResult struct {
    Config         *envconfig.ClientConfig
    ConfigFilePath string
}
```

### Integration Concern: Two `cliext` Packages

**Current state:**
- `internal/cliext/` - Extension discovery/execution (this branch)
- `cliext/` - Config/profile management (auth-lib branch)

**Recommendation:** Merge into single package structure:
```
internal/cliext/
├── discovery.go      # Extension discovery
├── executor.go       # Extension execution
├── help.go           # Help routing
├── config.go         # Config loading (from auth-lib)
├── profile.go        # Profile management (from auth-lib)
└── example/          # Example extension
```

---

## Part 3: OAuth Design Integration ✅ COMPLETE

Added `oauth.go` with:
- `OAuthConfig` struct for token storage
- `LoadOAuthConfig` / `SaveOAuthConfig` / `DeleteOAuthConfig` functions
- `OAuthLoginOptions` and `OAuthLoginResult` types for login flow
- Token expiration and refresh logic helpers

The OAuth flow requires extensions to:

1. Handle authentication (`temporal <extension> login`)
2. Store tokens in profile config
3. CLI uses stored tokens for subsequent commands
4. CLI handles token refresh automatically

### Required Extension Capabilities

| Capability | Current Support | Needed |
|------------|-----------------|--------|
| Execute extension commands | ✓ Yes | - |
| Pass profile name to extension | ✗ No | Add `--profile` forwarding |
| Extension writes to config | ✗ No | Expose config write API |
| Read OAuth tokens from config | ✗ No | Integrate with auth-lib |
| Token refresh on 401 | ✗ No | Add retry with refresh logic |

### Proposed OAuth Integration

```go
// Extension can request config access
type ExtensionContext struct {
    Profile     string
    ConfigPath  string
    // Extension can read/write profile config
}

// Pass context to extension via environment
func ExecuteExtension(ctx context.Context, ext *Extension, args []string,
    ioConfig *IOConfig, extCtx *ExtensionContext) (*ExecutionResult, error) {

    cmd := exec.CommandContext(ctx, ext.Path, args...)
    cmd.Env = append(os.Environ(),
        "TEMPORAL_PROFILE="+extCtx.Profile,
        "TEMPORAL_CONFIG_FILE="+extCtx.ConfigPath,
    )
    // ...
}
```

---

## Part 4: Testing Strategy ✅ IMPLEMENTED

### Goals

1. **Unit tests** for all functions with mocked dependencies
2. **Integration tests** with real extension binaries
3. **Example extension** that can be built and tested against
4. **Cross-platform** testing (Unix + Windows)

### Test Architecture

```
internal/cliext/
├── discovery_test.go       # Unit tests with temp directories
├── executor_test.go        # Unit tests with test binaries
├── help_test.go            # Unit tests
├── io_util_test.go         # NEW: Unit tests for I/O utilities
├── integration_test.go     # NEW: Integration tests
│
└── example/
    ├── cmd/
    │   └── temporal-example/
    │       └── main.go     # NEW: Buildable extension binary
    ├── commands.yml
    ├── commands.gen.go
    ├── commands.go
    ├── commands.hello.go
    └── commands.user.go
```

### Example Extension Binary

Create a standalone binary for integration testing:

```go
// internal/cliext/example/cmd/temporal-example/main.go
package main

import (
    "context"
    "github.com/temporalio/cli/internal/temporalcli"
    "github.com/temporalio/cli/internal/ext/example"
)

func main() {
    ctx := context.Background()
    cliextexample.Execute(ctx, temporalcli.CommandOptions{})
}
```

### Integration Test Design

```go
// internal/cliext/integration_test.go
package ext_test

import (
    "os/exec"
    "path/filepath"
    "testing"
)

var exampleBinaryPath string

func TestMain(m *testing.M) {
    // Build example extension before tests
    tmpDir, _ := os.MkdirTemp("", "cliext-test")
    exampleBinaryPath = filepath.Join(tmpDir, "temporal-example")

    cmd := exec.Command("go", "build", "-o", exampleBinaryPath,
        "./example/cmd/temporal-example")
    if err := cmd.Run(); err != nil {
        log.Fatalf("Failed to build example extension: %v", err)
    }

    // Add to PATH for discovery tests
    os.Setenv("PATH", tmpDir + string(os.PathListSeparator) + os.Getenv("PATH"))

    code := m.Run()
    os.RemoveAll(tmpDir)
    os.Exit(code)
}

func TestDiscoverExampleExtension(t *testing.T) {
    extensions := DiscoverExtensions()

    found := false
    for _, ext := range extensions {
        if ext.Name == "temporal-example" {
            found = true
            break
        }
    }

    if !found {
        t.Error("Example extension not discovered")
    }
}

func TestExecuteExampleHello(t *testing.T) {
    ioConfig, stdout, _ := NewCaptureIOConfig(nil)

    result := TryExecuteExtension(
        context.Background(),
        []string{"example", "hello"},
        0,
        ioConfig,
    )

    if result == nil || result.Extension == nil {
        t.Fatal("Extension not found")
    }

    if result.ExitCode != 0 {
        t.Errorf("Expected exit code 0, got %d", result.ExitCode)
    }

    output := stdout.String()
    if !strings.Contains(output, "Hello") {
        t.Errorf("Expected 'Hello' in output, got: %s", output)
    }
}

func TestExtensionHelp(t *testing.T) {
    ioConfig, stdout, _ := NewCaptureIOConfig(nil)

    result := TryShowExtensionHelp(
        context.Background(),
        []string{"help", "example"},
        0,
        ioConfig,  // After fixing Issue #3
    )

    if result.ExitCode != 0 {
        t.Errorf("Help failed with exit code %d", result.ExitCode)
    }

    output := stdout.String()
    if !strings.Contains(output, "temporal-example") {
        t.Errorf("Help output missing extension name")
    }
}

func TestExtensionWithProfile(t *testing.T) {
    // Test that profile is passed to extension
    // Requires ExtensionContext implementation
}

func TestExtensionTimeout(t *testing.T) {
    // Create extension that sleeps
    // Verify timeout kills it and returns correct result
}
```

### Test Matrix

| Test Type | discovery | executor | help | io_util | integration |
|-----------|-----------|----------|------|---------|-------------|
| Happy path | ✓ | ✓ | ✓ | ✓ | ✓ |
| Error cases | ✓ | ✓ | ✓ | ✓ | ✓ |
| Edge cases | Partial | Partial | ✓ | ✓ | Partial |
| Platform-specific | ✓ | ✓ | N/A | ✓ | N/A |
| Concurrency | ✓ (cache) | N/A | N/A | N/A | N/A |
| Performance | ✓ (cache) | N/A | N/A | N/A | TODO |

---

## Part 5: Holistic Design Recommendations

### Package Structure (Proposed)

```
internal/
├── cliext/
│   ├── discovery.go        # Extension discovery
│   ├── executor.go         # Extension execution
│   ├── help.go             # Help routing
│   ├── context.go          # NEW: ExtensionContext for profile/config
│   ├── config.go           # From auth-lib: config loading
│   ├── profile.go          # From auth-lib: profile management
│   ├── oauth.go            # NEW: OAuth token management
│   │
│   ├── example/            # Example extension (for testing)
│   │   ├── cmd/temporal-example/main.go
│   │   └── ...
│   │
│   └── testutil/           # NEW: Test utilities
│       ├── mock_extension.go
│       └── test_helpers.go
│
└── temporalcli/
    ├── commands.go
    ├── commands.execute.go  # Uses cliext
    └── ...
```

### API Design (Proposed)

```go
package ext

// Core types
type Extension struct {
    Name          string
    Path          string
    CommandTokens []string
    Metadata      *ExtensionMetadata  // NEW: optional metadata
}

type ExtensionMetadata struct {
    Version     string
    Description string
    Author      string
}

// Execution with context
type ExecuteOptions struct {
    Args       []string
    IO         *IOConfig
    Timeout    time.Duration
    Env        map[string]string  // NEW: custom env vars
    WorkDir    string             // NEW: working directory
    Profile    string             // NEW: profile name
    ConfigPath string             // NEW: config file path
}

type ExecuteResult struct {
    Extension  *Extension
    ExitCode   int
    Duration   time.Duration
    Timeout    bool              // NEW: was killed by timeout
    Error      error
}

func ExecuteExtension(ctx context.Context, ext *Extension,
    opts ExecuteOptions) (*ExecuteResult, error)

// High-level API
func TryExecuteExtension(ctx context.Context, args []string,
    opts ExecuteOptions) *ExecuteResult

// OAuth integration
type OAuthConfig struct {
    ClientID     string
    ClientSecret string
    TokenURL     string
    AccessToken  string
    RefreshToken string
    ExpiresAt    time.Time
    Audience     string
    Scopes       []string
}

func LoadOAuthConfig(profile string, configPath string) (*OAuthConfig, error)
func SaveOAuthConfig(profile string, configPath string, cfg *OAuthConfig) error
func RefreshAccessToken(cfg *OAuthConfig) (*OAuthConfig, error)
```

### Testing Recommendations

1. **Add `io_util_test.go`** - 100% coverage for I/O utilities
2. **Create buildable example extension** - `cmd/temporal-example/main.go`
3. **Add integration test suite** - Tests against real binary
4. **Add benchmark tests** - Discovery performance with large PATH
5. **Add fuzz tests** - Extension name parsing edge cases

### Migration Path

1. **Phase 1: Fix Critical Issues**
   - Windows executable detection
   - Timeout/exit code distinction
   - Help IOConfig parameter

2. **Phase 2: Merge auth-lib**
   - Integrate config/profile into cliext
   - Add ExtensionContext for profile passing

3. **Phase 3: OAuth Support**
   - Add OAuth config types
   - Implement token refresh logic
   - Extension login flow

4. **Phase 4: Testing**
   - Build example extension binary
   - Add integration tests
   - Add missing unit tests

---

## Action Items

### Immediate (This PR) ✅ COMPLETE

- [x] Fix Windows executable detection in `discovery.go`
- [x] Add `io_util_test.go` with full coverage
- [x] Create `cmd/temporal-example/main.go` for integration testing
- [x] Add `integration_test.go` with basic tests
- [x] Add IOConfig parameter to `TryShowExtensionHelp()`
- [x] Improve timeout vs exit code reporting (ExecuteResult struct)
- [x] Add discovery caching

### Short-term (Next PR) ✅ COMPLETE

- [x] Merge auth-lib config/profile into `internal/cliext`
- [x] Add `ExtensionContext` for profile/config passing
- [x] Add environment variable forwarding to extensions (TEMPORAL_PROFILE, TEMPORAL_CONFIG_FILE)

### Medium-term ✅ MOSTLY COMPLETE

- [x] Implement OAuth config loading/saving
- [x] Add token expiration/refresh logic helpers
- [ ] Add debug logging support (optional enhancement)

---

## Appendix: Test Commands

```bash
# Run unit tests
go test ./internal/cliext/...

# Run with verbose output
go test -v ./internal/cliext/...

# Run specific test
go test -v -run TestDiscovery ./internal/cliext/...

# Run with race detection
go test -race ./internal/cliext/...

# Run benchmarks (future)
go test -bench=. ./internal/cliext/...

# Build example extension
go build -o /tmp/temporal-example ./internal/cliext/example/cmd/temporal-example

# Test extension manually
PATH=/tmp:$PATH temporal example hello
```
