package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the jet binary to the latest version",
	Long: `Update the jet binary by rebuilding from the current source code.
	
This command will:
1. Build the latest version of jet from the current directory
2. Replace the existing jet binary with the new version
3. Verify the installation`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🚀 Updating jet binary...")

		// Get the path of the current jet binary
		jetPath, err := exec.LookPath("jet")
		if err != nil {
			return fmt.Errorf("could not find jet binary in PATH: %w", err)
		}

		fmt.Printf("📍 Found jet at: %s\n", jetPath)

		// Find the project root (where go.mod is located)
		projectRoot, err := findProjectRoot()
		if err != nil {
			return fmt.Errorf("could not find project root: %w", err)
		}

		fmt.Printf("📂 Building from: %s\n", projectRoot)

		// Change to project directory
		oldDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get current directory: %w", err)
		}
		defer os.Chdir(oldDir)

		if err := os.Chdir(projectRoot); err != nil {
			return fmt.Errorf("could not change to project directory: %w", err)
		}

		// Build the new binary
		fmt.Println("🔨 Building new binary...")
		buildCmd := exec.Command("go", "build", "-o", "jet", ".")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build jet: %w", err)
		}

		// Replace the existing binary
		fmt.Println("📦 Installing new binary...")
		if err := os.Rename("jet", jetPath); err != nil {
			return fmt.Errorf("failed to replace jet binary: %w", err)
		}

		// Make sure it's executable
		if err := os.Chmod(jetPath, 0755); err != nil {
			return fmt.Errorf("failed to make jet executable: %w", err)
		}

		// Test the new installation
		fmt.Println("🧪 Testing new installation...")
		testCmd := exec.Command(jetPath, "--version")
		if err := testCmd.Run(); err != nil {
			// If --version doesn't work, try --help
			testCmd = exec.Command(jetPath, "--help")
			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("new jet binary failed to run: %w", err)
			}
		}

		fmt.Println("✅ jet has been successfully updated!")
		return nil
	},
}

// findProjectRoot looks for go.mod file to determine project root
func findProjectRoot() (string, error) {
	// Start from current directory and work up
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Check if go.mod exists in current directory
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find go.mod file in current directory or any parent directory")
}

func init() {
	rootCmd.AddCommand(updateCmd)
}