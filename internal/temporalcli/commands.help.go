package temporalcli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// discoverExtensions scans the PATH environment variable and returns all
// discovered Temporal CLI extensions. This is used for "help --all" to
// list all available extensions.
//
// Extensions are executables with the "temporal-" prefix. The function:
//   - Scans all directories in PATH
//   - Filters for files starting with "temporal-"
//   - Validates each file is executable using exec.LookPath
//   - Returns the first occurrence of each extension name (PATH order matters)
func discoverExtensions() []extension {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}

	pathSeparator := ":"
	if runtime.GOOS == "windows" {
		pathSeparator = ";"
	}

	directories := strings.Split(pathEnv, pathSeparator)
	extMap := make(map[string]extension)

	for _, dir := range directories {
		if dir == "" {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()

			if !strings.HasPrefix(name, extensionPrefix) {
				continue
			}

			if _, exists := extMap[name]; exists {
				continue
			}

			execPath, err := exec.LookPath(filepath.Join(dir, name))
			if err != nil {
				continue
			}

			extMap[name] = extension{
				Name:          name,
				Path:          execPath,
				CommandTokens: parseCommandTokens(name),
			}
		}
	}

	extensions := make([]extension, 0, len(extMap))
	for _, ext := range extMap {
		extensions = append(extensions, ext)
	}

	return extensions
}

// parseCommandTokens extracts command tokens from an extension name.
// Example: "temporal-foo-bar_baz" → ["foo", "bar-baz"]
func parseCommandTokens(name string) []string {
	withoutPrefix := strings.TrimPrefix(name, extensionPrefix)
	if withoutPrefix == "" {
		return nil
	}

	tokens := strings.Split(withoutPrefix, "-")
	for i, token := range tokens {
		tokens[i] = strings.ReplaceAll(token, "_", "-")
	}

	return tokens
}

// formatExtensionsForHelp formats discovered extensions for display in help output.
// Returns sorted list of space-separated command strings (e.g., "foo bar", "cloud namespace list").
func formatExtensionsForHelp(extensions []extension) []string {
	if len(extensions) == 0 {
		return nil
	}

	formatted := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		formatted = append(formatted, strings.Join(ext.CommandTokens, " "))
	}

	sort.Strings(formatted)
	return formatted
}

// configureHelpCommand replaces Cobra's default help command with a custom one
// that supports --all to show extensions.
func (c *TemporalCommand) configureHelpCommand(cctx *CommandContext) {
	// Remove the default help command
	c.Command.SetHelpCommand(&cobra.Command{Hidden: true})

	// Add our custom help command
	helpCmd := &cobra.Command{
		Use:   "help [command]",
		Short: "Help for any command",
		Long: `Help provides information for any command in the application.
Use --all to also show available CLI extensions.`,
		Run: func(cmd *cobra.Command, args []string) {
			showAll, _ := cmd.Flags().GetBool("all")

			// Set output writers for help
			c.Command.SetOut(cctx.Options.Stdout)
			c.Command.SetErr(cctx.Options.Stderr)

			if len(args) == 0 {
				// Show root help
				_ = c.Command.Help()
				if showAll {
					printExtensions(cctx)
				}
				return
			}

			// Find the target command
			targetCmd, remainingArgs, err := c.Command.Find(args)
			if err != nil || targetCmd == nil || len(remainingArgs) > 0 {
				// Not a built-in command, try extension with --help
				// Per proposal: "temporal help <cmd>" becomes "temporal <cmd> --help"
				extArgs := append(args, "--help")
				ioConfig := &extensionIOConfig{
					Stdin:  cctx.Options.Stdin,
					Stdout: cctx.Options.Stdout,
					Stderr: cctx.Options.Stderr,
				}
				_, found := tryExecuteExtension(cctx, extArgs, ioConfig)
				if found {
					return
				}
				// No extension either
				fmt.Fprintf(cctx.Options.Stderr, "Unknown command: %s\n", strings.Join(args, " "))
				return
			}

			targetCmd.SetOut(cctx.Options.Stdout)
			targetCmd.SetErr(cctx.Options.Stderr)
			_ = targetCmd.Help()
			if showAll {
				printExtensions(cctx)
			}
		},
	}
	helpCmd.Flags().Bool("all", false, "Show all commands including CLI extensions")
	// Mark help command so it doesn't require env config
	helpCmd.Annotations = map[string]string{"ignoresMissingEnv": "true"}

	c.Command.AddCommand(helpCmd)
}

// printExtensions discovers and prints CLI extensions.
func printExtensions(cctx *CommandContext) {
	extensions := discoverExtensions()
	if len(extensions) == 0 {
		return
	}

	formatted := formatExtensionsForHelp(extensions)
	if len(formatted) == 0 {
		return
	}

	fmt.Fprintln(cctx.Options.Stdout)
	fmt.Fprintln(cctx.Options.Stdout, "CLI Extensions:")
	for _, cmd := range formatted {
		fmt.Fprintf(cctx.Options.Stdout, "  %s\n", cmd)
	}
}
