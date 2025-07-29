package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	attachmentDownload bool
	attachmentOutput   string
	attachmentIndex    string
)

var attachmentsCmd = &cobra.Command{
	Use:   "attachments TICKET-KEY",
	Short: "List or download JIRA ticket attachments",
	Long: `List attachments for a JIRA ticket or download specific attachments.

Examples:
  jet attachments LX-2956                    # List all attachments
  jet attachments LX-2956 --download         # Download all attachments
  jet attachments LX-2956 --download --index 1,3  # Download attachments 1 and 3
  jet attachments LX-2956 --download --output /path/to/folder  # Download to specific folder`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketKey := args[0]

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create JIRA client
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Fetch the ticket
		issue, err := client.GetIssue(ticketKey)
		if err != nil {
			return err
		}

		// Get attachments
		attachments := issue.Fields.Attachment
		if len(attachments) == 0 {
			fmt.Printf("No attachments found for ticket %s\n", ticketKey)
			return nil
		}

		// If not downloading, just list attachments
		if !attachmentDownload {
			return listAttachments(ticketKey, attachments)
		}

		// Download attachments
		return downloadAttachments(ticketKey, attachments, cfg)
	},
}

func listAttachments(ticketKey string, attachments []jira.Attachment) error {
	// Define colors
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	blue := color.New(color.FgBlue)
	gray := color.New(color.FgHiBlack)
	green := color.New(color.FgGreen)

	fmt.Printf("%s %s\n", cyan.Sprint("📎 Attachments for"), cyan.Sprint(ticketKey))
	fmt.Println(gray.Sprint("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

	for i, attachment := range attachments {
		authorName := attachment.Author.DisplayName
		if authorName == "" {
			authorName = attachment.Author.Name
		}
		created := attachment.Created
		if len(created) >= 10 {
			created = created[:10]
		}

		// Format file size
		size := formatFileSize(attachment.Size)

		fmt.Printf("%s %s (%s)\n", yellow.Sprintf("%d.", i+1), attachment.Filename, blue.Sprint(size))
		fmt.Printf("   %s %s\n", gray.Sprint("Type:"), attachment.MimeType)
		fmt.Printf("   %s %s (%s)\n", gray.Sprint("Uploaded by:"), authorName, created)
		if attachment.Content != "" {
			fmt.Printf("   %s %s\n", gray.Sprint("URL:"), green.Sprint(attachment.Content))
		}
		fmt.Println()
	}

	fmt.Printf("💡 %s\n", gray.Sprint("Use --download to download attachments"))
	fmt.Printf("💡 %s\n", gray.Sprint("Use --index 1,3 to download specific attachments"))

	return nil
}

func downloadAttachments(ticketKey string, attachments []jira.Attachment, cfg *config.Config) error {
	// Determine which attachments to download
	var indicesToDownload []int
	if attachmentIndex != "" {
		indices := strings.Split(attachmentIndex, ",")
		for _, indexStr := range indices {
			index, err := strconv.Atoi(strings.TrimSpace(indexStr))
			if err != nil {
				return fmt.Errorf("invalid attachment index: %s", indexStr)
			}
			if index < 1 || index > len(attachments) {
				return fmt.Errorf("attachment index %d out of range (1-%d)", index, len(attachments))
			}
			indicesToDownload = append(indicesToDownload, index-1) // Convert to 0-based
		}
	} else {
		// Download all attachments
		for i := range attachments {
			indicesToDownload = append(indicesToDownload, i)
		}
	}

	// Determine output directory
	outputDir := attachmentOutput
	if outputDir == "" {
		outputDir = fmt.Sprintf("%s_attachments", ticketKey)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Define colors
	cyan := color.New(color.FgCyan, color.Bold)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	fmt.Printf("%s %d attachments to %s\n", cyan.Sprint("📁 Downloading"), len(indicesToDownload), outputDir)
	fmt.Println()

	// Download each attachment
	for _, index := range indicesToDownload {
		attachment := attachments[index]
		
		fmt.Printf("%s %s...", yellow.Sprintf("⬇️  Downloading"), attachment.Filename)
		
		if err := downloadSingleAttachment(attachment, outputDir, cfg); err != nil {
			fmt.Printf(" ❌\n")
			fmt.Printf("   Error: %v\n", err)
			continue
		}
		
		fmt.Printf(" ✅\n")
	}

	fmt.Printf("\n%s Downloaded %d attachments to %s\n", green.Sprint("🎉 Success!"), len(indicesToDownload), outputDir)
	return nil
}

func downloadSingleAttachment(attachment jira.Attachment, outputDir string, cfg *config.Config) error {
	if attachment.Content == "" {
		return fmt.Errorf("no download URL available")
	}

	// Create HTTP client with authentication
	client := &http.Client{}
	req, err := http.NewRequest("GET", attachment.Content, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication headers
	if cfg.Email != "" && cfg.Token != "" {
		// Atlassian Cloud authentication
		req.SetBasicAuth(cfg.Email, cfg.Token)
	} else if cfg.Username != "" && cfg.Token != "" {
		// Atlassian Server authentication
		req.SetBasicAuth(cfg.Username, cfg.Token)
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Create the output file
	outputPath := filepath.Join(outputDir, attachment.Filename)
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy the content
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(attachmentsCmd)
	
	attachmentsCmd.Flags().BoolVar(&attachmentDownload, "download", false, "Download attachments instead of just listing them")
	attachmentsCmd.Flags().StringVar(&attachmentOutput, "output", "", "Output directory for downloads (default: TICKET-KEY_attachments)")
	attachmentsCmd.Flags().StringVar(&attachmentIndex, "index", "", "Comma-separated list of attachment indices to download (e.g., 1,3,5)")
}