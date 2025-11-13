package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/confluence"
)

var confluenceCmd = &cobra.Command{
	Use:     "con",
	Aliases: []string{"confluence"},
	Short:   "Interact with Confluence",
	Long: `Access Confluence pages and perform searches.

Subcommands:
  view   - View a Confluence page
  search - Search for Confluence pages
  create - Create a new Confluence page`,
}

var conViewFormat string
var conViewOutput string

var conViewCmd = &cobra.Command{
	Use:   "view PAGE-ID",
	Short: "View a Confluence page",
	Long: `Fetch and display a Confluence page by its ID.

You can find the page ID in the URL when viewing a page in Confluence:
  https://yourcompany.atlassian.net/wiki/spaces/SPACE/pages/123456789/Page+Title
  The page ID is 123456789`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pageID := args[0]

		// Extract page ID from URL if a URL was provided
		if strings.Contains(pageID, "://") {
			// Extract page ID from URL (e.g., .../pages/123456789/...)
			re := regexp.MustCompile(`/pages/(\d+)`)
			matches := re.FindStringSubmatch(pageID)
			if len(matches) > 1 {
				pageID = matches[1]
			} else {
				return fmt.Errorf("could not extract page ID from URL. Expected format: .../pages/123456789/...")
			}
		}

		// Load configuration
		cfg, err := config.LoadConfluence()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create Confluence client
		client := confluence.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Fetch the page
		page, err := client.GetPage(pageID)
		if err != nil {
			return err
		}

		// Format output
		var output string
		switch conViewFormat {
		case "json":
			jsonData, err := json.MarshalIndent(page, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			output = string(jsonData)
		default:
			output = formatPageReadable(page)
		}

		// Write output
		if conViewOutput != "" {
			file, err := os.Create(conViewOutput)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer file.Close()

			if _, err := file.WriteString(output); err != nil {
				return fmt.Errorf("failed to write to output file: %w", err)
			}
			fmt.Printf("Page information saved to %s\n", conViewOutput)
		} else {
			fmt.Print(output)
		}

		return nil
	},
}

var conSearchLimit int
var conSearchSpace string

var conSearchCmd = &cobra.Command{
	Use:   "search QUERY",
	Short: "Search Confluence pages",
	Long: `Search for Confluence pages using text search or CQL.

Examples:
  jet con search "project documentation"
  jet con search "API guide" --space DEV

Use --space to limit search to a specific space.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]

		// Load configuration
		cfg, err := config.LoadConfluence()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Create Confluence client
		client := confluence.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// Perform search
		var results *confluence.SearchResponse
		if conSearchSpace != "" {
			// Build CQL with space filter and text search
			cql := fmt.Sprintf("type=page AND space=\"%s\" AND text~\"%s\"", conSearchSpace, query)
			results, err = client.SearchPages(cql, conSearchLimit)
		} else {
			results, err = client.SearchByText(query, conSearchLimit)
		}

		if err != nil {
			return err
		}

		// Display results
		fmt.Print(formatSearchResults(results))
		return nil
	},
}

func formatPageReadable(page *confluence.Page) string {
	var sb strings.Builder

	// Header
	boldBlue := color.New(color.FgBlue, color.Bold)
	boldGreen := color.New(color.FgGreen, color.Bold)
	cyan := color.New(color.FgCyan)

	sb.WriteString(boldBlue.Sprint("═══════════════════════════════════════════════════════════════\n"))
	sb.WriteString(boldBlue.Sprintf("  %s\n", page.Title))
	sb.WriteString(boldBlue.Sprint("═══════════════════════════════════════════════════════════════\n\n"))

	// Metadata
	sb.WriteString(boldGreen.Sprint("Page ID: "))
	sb.WriteString(fmt.Sprintf("%s\n", page.ID))

	sb.WriteString(boldGreen.Sprint("Status:  "))
	sb.WriteString(fmt.Sprintf("%s\n", page.Status))

	if page.Version != nil {
		sb.WriteString(boldGreen.Sprint("Version: "))
		sb.WriteString(fmt.Sprintf("%d\n", page.Version.Number))
	}

	if page.Links != nil && page.Links.Base != "" && page.Links.WebUI != "" {
		sb.WriteString(boldGreen.Sprint("URL:     "))
		sb.WriteString(cyan.Sprintf("%s%s\n", page.Links.Base, page.Links.WebUI))
	}

	sb.WriteString("\n")

	// Content
	if page.Body != nil && page.Body.Storage != nil {
		sb.WriteString(boldBlue.Sprint("─────────────────────────────────────────────────────────────\n"))
		sb.WriteString(boldBlue.Sprint("Content\n"))
		sb.WriteString(boldBlue.Sprint("─────────────────────────────────────────────────────────────\n\n"))

		// Convert Confluence storage format to readable text
		content := convertStorageFormatToText(page.Body.Storage.Value)
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	return sb.String()
}

func convertStorageFormatToText(storageHTML string) string {
	// Basic HTML to text conversion
	content := storageHTML

	// Remove CDATA sections
	content = regexp.MustCompile(`<!\[CDATA\[(.*?)\]\]>`).ReplaceAllString(content, "$1")

	// Convert headings
	content = regexp.MustCompile(`<h1>(.*?)</h1>`).ReplaceAllString(content, "\n# $1\n")
	content = regexp.MustCompile(`<h2>(.*?)</h2>`).ReplaceAllString(content, "\n## $1\n")
	content = regexp.MustCompile(`<h3>(.*?)</h3>`).ReplaceAllString(content, "\n### $1\n")
	content = regexp.MustCompile(`<h4>(.*?)</h4>`).ReplaceAllString(content, "\n#### $1\n")

	// Convert paragraphs
	content = regexp.MustCompile(`<p>(.*?)</p>`).ReplaceAllString(content, "$1\n\n")

	// Convert line breaks
	content = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(content, "\n")

	// Convert links
	content = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(content, "$2 ($1)")

	// Convert bold
	content = regexp.MustCompile(`<strong>(.*?)</strong>`).ReplaceAllString(content, "**$1**")
	content = regexp.MustCompile(`<b>(.*?)</b>`).ReplaceAllString(content, "**$1**")

	// Convert italic
	content = regexp.MustCompile(`<em>(.*?)</em>`).ReplaceAllString(content, "*$1*")
	content = regexp.MustCompile(`<i>(.*?)</i>`).ReplaceAllString(content, "*$1*")

	// Convert code
	content = regexp.MustCompile(`<code>(.*?)</code>`).ReplaceAllString(content, "`$1`")

	// Convert lists
	content = regexp.MustCompile(`<li>(.*?)</li>`).ReplaceAllString(content, "  • $1\n")
	content = regexp.MustCompile(`<ul[^>]*>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`</ul>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`<ol[^>]*>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`</ol>`).ReplaceAllString(content, "\n")

	// Remove remaining HTML tags
	content = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(content, "")

	// Decode HTML entities
	content = strings.ReplaceAll(content, "&nbsp;", " ")
	content = strings.ReplaceAll(content, "&amp;", "&")
	content = strings.ReplaceAll(content, "&lt;", "<")
	content = strings.ReplaceAll(content, "&gt;", ">")
	content = strings.ReplaceAll(content, "&quot;", "\"")
	content = strings.ReplaceAll(content, "&#39;", "'")

	// Clean up excessive whitespace
	content = regexp.MustCompile(`\n{3,}`).ReplaceAllString(content, "\n\n")
	content = strings.TrimSpace(content)

	return content
}

func formatSearchResults(results *confluence.SearchResponse) string {
	var sb strings.Builder

	boldBlue := color.New(color.FgBlue, color.Bold)
	boldGreen := color.New(color.FgGreen, color.Bold)
	cyan := color.New(color.FgCyan)

	sb.WriteString(boldBlue.Sprintf("Found %d result(s)\n\n", results.Size))

	for i, result := range results.Results {
		sb.WriteString(boldBlue.Sprintf("%d. ", i+1))

		// Title
		title := result.Title
		if result.Content.Title != "" {
			title = result.Content.Title
		}
		sb.WriteString(boldGreen.Sprintf("%s\n", title))

		// Page ID
		if result.Content.ID != "" {
			sb.WriteString(cyan.Sprintf("   ID: %s", result.Content.ID))

			// Space info
			if result.Content.Space.Key != "" {
				sb.WriteString(fmt.Sprintf(" | Space: %s", result.Content.Space.Key))
			}
			sb.WriteString("\n")
		}

		// URL
		if result.URL != "" {
			sb.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
		} else if result.Content.Links.WebUI != "" {
			sb.WriteString(fmt.Sprintf("   Path: %s\n", result.Content.Links.WebUI))
		}

		// Excerpt
		if result.Excerpt != "" {
			excerpt := stripHTMLTags(result.Excerpt)
			// Limit excerpt length
			if len(excerpt) > 200 {
				excerpt = excerpt[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s\n", excerpt))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

func stripHTMLTags(html string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]+>`)
	text := re.ReplaceAllString(html, "")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	// Clean up whitespace
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return text
}

var conCreateSpace string
var conCreateParent string
var conCreateFile string

var conCreateCmd = &cobra.Command{
	Use:   "create TITLE",
	Short: "Create a new Confluence page",
	Long: `Create a new Confluence page with the specified title and content.

You must specify the space ID or key where the page will be created using the --space flag.
Content can be provided via --file flag or will be read from stdin.

Examples:
  jet con create "My New Page" --space ENG --file content.html
  jet con create "My New Page" --space 123456 --file content.html
  echo "<p>Hello world</p>" | jet con create "My Page" --space ENG
  jet con create "Child Page" --space ENG --parent 789012

Content should be in Confluence storage format (HTML-like format).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := args[0]

		if conCreateSpace == "" {
			return fmt.Errorf("space ID or key is required (use --space flag)")
		}

		// Load configuration
		cfg, err := config.LoadConfluence()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		// Read content from file or stdin
		var content string
		if conCreateFile != "" {
			data, err := os.ReadFile(conCreateFile)
			if err != nil {
				return fmt.Errorf("failed to read content file: %w", err)
			}
			content = string(data)
		} else {
			// Read from stdin
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read content from stdin: %w", err)
			}
			content = string(data)
		}

		if strings.TrimSpace(content) == "" {
			return fmt.Errorf("content cannot be empty")
		}

		// Create Confluence client
		client := confluence.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		// If space is a key (not numeric), convert to ID
		spaceID := conCreateSpace
		// Check if it's not all digits - if so, it's a space key
		if !regexp.MustCompile(`^\d+$`).MatchString(conCreateSpace) {
			fmt.Printf("Looking up space ID for key: %s\n", conCreateSpace)
			space, err := client.GetSpace(conCreateSpace)
			if err != nil {
				return fmt.Errorf("failed to get space: %w", err)
			}
			spaceID = space.ID
			fmt.Printf("Found space ID: %s\n", spaceID)
		}

		// Create the page
		page, err := client.CreatePage(spaceID, title, content, conCreateParent)
		if err != nil {
			return err
		}

		// Display success message
		boldGreen := color.New(color.FgGreen, color.Bold)
		cyan := color.New(color.FgCyan)

		fmt.Println(boldGreen.Sprint("✓ Page created successfully!"))
		fmt.Printf("\nPage ID: %s\n", page.ID)
		fmt.Printf("Title:   %s\n", page.Title)
		fmt.Printf("Status:  %s\n", page.Status)

		if page.Links != nil && page.Links.Base != "" && page.Links.WebUI != "" {
			fmt.Printf("URL:     %s\n", cyan.Sprintf("%s%s", page.Links.Base, page.Links.WebUI))
		}

		return nil
	},
}

func init() {
	// Add view subcommand to confluence command
	conViewCmd.Flags().StringVarP(&conViewFormat, "format", "f", "readable", "Output format (readable, json)")
	conViewCmd.Flags().StringVarP(&conViewOutput, "output", "o", "", "Write output to file")
	confluenceCmd.AddCommand(conViewCmd)

	// Add search subcommand to confluence command
	conSearchCmd.Flags().IntVarP(&conSearchLimit, "limit", "l", 10, "Maximum number of results")
	conSearchCmd.Flags().StringVarP(&conSearchSpace, "space", "s", "", "Limit search to specific space")
	confluenceCmd.AddCommand(conSearchCmd)

	// Add create subcommand to confluence command
	conCreateCmd.Flags().StringVarP(&conCreateSpace, "space", "s", "", "Space ID or key where the page will be created (required)")
	conCreateCmd.Flags().StringVarP(&conCreateParent, "parent", "p", "", "Parent page ID (optional)")
	conCreateCmd.Flags().StringVarP(&conCreateFile, "file", "f", "", "Read content from file instead of stdin")
	conCreateCmd.MarkFlagRequired("space")
	confluenceCmd.AddCommand(conCreateCmd)

	// Add confluence command to root
	rootCmd.AddCommand(confluenceCmd)
}
