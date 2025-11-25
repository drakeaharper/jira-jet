package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	viewFormat string
	viewOutput string
)

var viewCmd = &cobra.Command{
	Use:   "view TICKET-KEY|URL",
	Short: "View a JIRA ticket",
	Long:  `Fetch and display information about a JIRA ticket.
You can provide either a ticket key (e.g., LX-2894) or a full JIRA URL (e.g., https://company.atlassian.net/browse/ABC-123).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketKey := args[0]

		// Check if the argument is a URL and extract the ticket key
		if strings.Contains(ticketKey, "://") {
			// Validate URL format
			parsedURL, err := url.Parse(ticketKey)
			if err != nil {
				return fmt.Errorf("invalid URL format: %w", err)
			}
			
			// Ensure it's HTTPS
			if parsedURL.Scheme != "https" {
				return fmt.Errorf("only HTTPS URLs are allowed for security")
			}
			
			// Validate it looks like a JIRA URL
			if !strings.Contains(parsedURL.Host, "atlassian.net") && !strings.HasSuffix(parsedURL.Host, "jira.com") {
				return fmt.Errorf("URL must be from a recognized JIRA domain (atlassian.net or jira.com)")
			}
			
			// Extract ticket key from URL (e.g., https://company.atlassian.net/browse/ABC-123)
			re := regexp.MustCompile(`/browse/([A-Z]+-\d+)`)
			matches := re.FindStringSubmatch(ticketKey)
			if len(matches) > 1 {
				ticketKey = matches[1]
			} else {
				return fmt.Errorf("invalid JIRA URL format. Expected format: https://domain.atlassian.net/browse/TICKET-123")
			}
		}

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

		// Format output
		var output string
		switch viewFormat {
		case "json":
			jsonData, err := json.MarshalIndent(issue, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			output = string(jsonData)
		default:
			output = formatIssueReadable(issue)
		}

		// Write output
		if viewOutput != "" {
			file, err := os.Create(viewOutput)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer file.Close()

			if _, err := file.WriteString(output); err != nil {
				return fmt.Errorf("failed to write to output file: %w", err)
			}
			fmt.Printf("Ticket information saved to %s\n", viewOutput)
		} else {
			fmt.Print(output)
		}

		return nil
	},
}

func formatIssueReadable(issue *jira.Issue) string {
	var output strings.Builder

	// Define colors
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	green := color.New(color.FgGreen)
	blue := color.New(color.FgBlue)
	magenta := color.New(color.FgMagenta)
	red := color.New(color.FgRed)
	gray := color.New(color.FgHiBlack)

	// Ticket header
	output.WriteString(cyan.Sprintf("🎫 TICKET: %s\n", issue.Key))
	output.WriteString(gray.Sprint("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
	
	// Summary
	output.WriteString(fmt.Sprintf("%s %s\n", yellow.Sprint("📝 Summary:"), issue.Fields.Summary))
	
	// Status with color coding
	statusColor := getStatusColor(issue.Fields.Status.Name)
	output.WriteString(fmt.Sprintf("%s %s\n", blue.Sprint("📊 Status:"), statusColor.Sprint(issue.Fields.Status.Name)))
	
	// Type
	output.WriteString(fmt.Sprintf("%s %s\n", magenta.Sprint("🏷️  Type:"), issue.Fields.IssueType.Name))
	
	// Priority
	if issue.Fields.Priority.Name != "" {
		priorityColor := getPriorityColor(issue.Fields.Priority.Name)
		output.WriteString(fmt.Sprintf("%s %s\n", red.Sprint("⚡ Priority:"), priorityColor.Sprint(issue.Fields.Priority.Name)))
	} else {
		output.WriteString(fmt.Sprintf("%s %s\n", red.Sprint("⚡ Priority:"), gray.Sprint("None")))
	}
	
	// Project
	output.WriteString(fmt.Sprintf("%s %s (%s)\n", green.Sprint("📁 Project:"), issue.Fields.Project.Name, green.Sprint(issue.Fields.Project.Key)))

	// Epic Link
	if issue.Fields.EpicLink != nil && issue.Fields.EpicLink.Key != "" {
		if issue.Fields.EpicLink.Summary != "" {
			output.WriteString(fmt.Sprintf("%s %s (%s)\n", cyan.Sprint("🎯 Epic:"), issue.Fields.EpicLink.Summary, cyan.Sprint(issue.Fields.EpicLink.Key)))
		} else {
			output.WriteString(fmt.Sprintf("%s %s\n", cyan.Sprint("🎯 Epic:"), cyan.Sprint(issue.Fields.EpicLink.Key)))
		}
	} else if issue.Fields.Parent != nil && issue.Fields.Parent.Key != "" {
		if issue.Fields.Parent.Summary != "" {
			output.WriteString(fmt.Sprintf("%s %s (%s)\n", cyan.Sprint("🔗 Parent:"), issue.Fields.Parent.Summary, cyan.Sprint(issue.Fields.Parent.Key)))
		} else {
			output.WriteString(fmt.Sprintf("%s %s\n", cyan.Sprint("🔗 Parent:"), cyan.Sprint(issue.Fields.Parent.Key)))
		}
	}

	// Linked Issues
	if len(issue.Fields.IssueLinks) > 0 {
		output.WriteString("\n")
		output.WriteString(cyan.Sprintf("🔗 Linked Issues (%d):\n", len(issue.Fields.IssueLinks)))
		output.WriteString(gray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))

		for _, link := range issue.Fields.IssueLinks {
			var linkedIssue *jira.LinkedIssue
			var relationshipText string

			if link.OutwardIssue != nil {
				linkedIssue = link.OutwardIssue
				relationshipText = link.Type.Outward
			} else if link.InwardIssue != nil {
				linkedIssue = link.InwardIssue
				relationshipText = link.Type.Inward
			}

			if linkedIssue != nil {
				statusColor := getStatusColor(linkedIssue.Fields.Status.Name)
				output.WriteString(fmt.Sprintf("  %s %s\n", magenta.Sprint(relationshipText+":"), cyan.Sprint(linkedIssue.Key)))
				output.WriteString(fmt.Sprintf("    %s %s\n", gray.Sprint("Summary:"), linkedIssue.Fields.Summary))
				output.WriteString(fmt.Sprintf("    %s %s | %s %s\n",
					gray.Sprint("Status:"), statusColor.Sprint(linkedIssue.Fields.Status.Name),
					gray.Sprint("Type:"), linkedIssue.Fields.IssueType.Name))
			}
		}
		output.WriteString("\n")
	}

	// Assignee
	if issue.Fields.Assignee != nil {
		assigneeName := issue.Fields.Assignee.DisplayName
		if assigneeName == "" {
			assigneeName = issue.Fields.Assignee.Name
		}
		output.WriteString(fmt.Sprintf("%s %s\n", blue.Sprint("👤 Assignee:"), assigneeName))
	} else {
		output.WriteString(fmt.Sprintf("%s %s\n", blue.Sprint("👤 Assignee:"), gray.Sprint("Unassigned")))
	}

	// Reporter
	if issue.Fields.Reporter != nil {
		reporterName := issue.Fields.Reporter.DisplayName
		if reporterName == "" {
			reporterName = issue.Fields.Reporter.Name
		}
		output.WriteString(fmt.Sprintf("%s %s\n", blue.Sprint("📝 Reporter:"), reporterName))
	}

	// Labels
	if len(issue.Fields.Labels) > 0 {
		output.WriteString(fmt.Sprintf("%s %s\n", magenta.Sprint("🏷️  Labels:"), strings.Join(issue.Fields.Labels, ", ")))
	}

	// Components
	if len(issue.Fields.Components) > 0 {
		var compNames []string
		for _, comp := range issue.Fields.Components {
			compNames = append(compNames, comp.Name)
		}
		output.WriteString(fmt.Sprintf("%s %s\n", green.Sprint("🔧 Components:"), strings.Join(compNames, ", ")))
	}

	// Fix Versions
	if len(issue.Fields.FixVersions) > 0 {
		var versionNames []string
		for _, version := range issue.Fields.FixVersions {
			versionNames = append(versionNames, version.Name)
		}
		output.WriteString(fmt.Sprintf("%s %s\n", yellow.Sprint("🎯 Fix Versions:"), strings.Join(versionNames, ", ")))
	}

	// Dates
	if len(issue.Fields.Created) >= 10 {
		output.WriteString(fmt.Sprintf("%s %s\n", gray.Sprint("📅 Created:"), issue.Fields.Created[:10]))
	}
	if len(issue.Fields.Updated) >= 10 {
		output.WriteString(fmt.Sprintf("%s %s\n", gray.Sprint("🔄 Updated:"), issue.Fields.Updated[:10]))
	}

	// Description
	if issue.Fields.DescriptionText != "" {
		output.WriteString("\n")
		output.WriteString(yellow.Sprint("📖 Description:\n"))
		output.WriteString(gray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))
		formattedDescription := formatHTMLContent(issue.Fields.DescriptionText)
		output.WriteString(formattedDescription + "\n")
	}

	// Attachments
	attachments := issue.Fields.Attachment
	if len(attachments) > 0 {
		output.WriteString("\n")
		output.WriteString(green.Sprintf("📎 Attachments (%d):\n", len(attachments)))
		output.WriteString(gray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))
		
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
			
			output.WriteString(fmt.Sprintf("%s %s (%s)\n", yellow.Sprintf("%d.", i+1), attachment.Filename, blue.Sprint(size)))
			output.WriteString(fmt.Sprintf("   %s %s\n", gray.Sprint("Type:"), attachment.MimeType))
			output.WriteString(fmt.Sprintf("   %s %s (%s)\n", gray.Sprint("Uploaded by:"), authorName, created))
			if attachment.Content != "" {
				output.WriteString(fmt.Sprintf("   %s %s\n", gray.Sprint("URL:"), attachment.Content))
			}
			output.WriteString("\n")
		}
	}

	// Comments
	comments := issue.Fields.Comment.Comments
	if len(comments) > 0 {
		output.WriteString("\n")
		output.WriteString(cyan.Sprintf("💬 Comments (%d):\n", len(comments)))
		output.WriteString(gray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))
		
		// Show last 5 comments
		start := 0
		if len(comments) > 5 {
			start = len(comments) - 5
		}
		
		for i, comment := range comments[start:] {
			authorName := comment.Author.DisplayName
			if authorName == "" {
				authorName = comment.Author.Name
			}
			created := comment.Created
			if len(created) >= 10 {
				created = created[:10]
			}
			
			output.WriteString(fmt.Sprintf("%s %s (%s):\n", yellow.Sprintf("%d.", i+1), authorName, gray.Sprint(created)))
			formattedComment := formatHTMLContent(comment.Body)
			output.WriteString(fmt.Sprintf("   %s\n\n", formattedComment))
		}
	}

	return output.String()
}

// Helper functions for color coding
func getStatusColor(status string) *color.Color {
	switch strings.ToLower(status) {
	case "open", "to do", "new":
		return color.New(color.FgRed)
	case "in progress", "in development":
		return color.New(color.FgYellow)
	case "done", "closed", "resolved":
		return color.New(color.FgGreen)
	case "review", "code review":
		return color.New(color.FgCyan)
	default:
		return color.New(color.FgWhite)
	}
}

func getPriorityColor(priority string) *color.Color {
	switch strings.ToLower(priority) {
	case "highest", "critical":
		return color.New(color.FgRed, color.Bold)
	case "high":
		return color.New(color.FgRed)
	case "medium":
		return color.New(color.FgYellow)
	case "low":
		return color.New(color.FgGreen)
	case "lowest":
		return color.New(color.FgHiBlack)
	default:
		return color.New(color.FgWhite)
	}
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatHTMLContent formats HTML-like content and wiki markup for better readability
func formatHTMLContent(content string) string {
	if content == "" {
		return content
	}

	// Define colors for formatting
	headerColor := color.New(color.FgCyan, color.Bold)
	subHeaderColor := color.New(color.FgBlue, color.Bold)
	emphasisColor := color.New(color.FgYellow)
	linkColor := color.New(color.FgGreen, color.Underline)

	// Handle Atlassian/Confluence wiki markup headers first
	content = regexp.MustCompile(`(?m)^h1\.\s*(.+)$`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`(?m)^h1\.\s*(.+)$`).ReplaceAllString(match, "$1")
		return headerColor.Sprintf("# %s", text)
	})

	content = regexp.MustCompile(`(?m)^h2\.\s*(.+)$`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`(?m)^h2\.\s*(.+)$`).ReplaceAllString(match, "$1")
		return headerColor.Sprintf("## %s", text)
	})

	content = regexp.MustCompile(`(?m)^h3\.\s*(.+)$`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`(?m)^h3\.\s*(.+)$`).ReplaceAllString(match, "$1")
		return subHeaderColor.Sprintf("### %s", text)
	})

	content = regexp.MustCompile(`(?m)^h4\.\s*(.+)$`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`(?m)^h4\.\s*(.+)$`).ReplaceAllString(match, "$1")
		return subHeaderColor.Sprintf("#### %s", text)
	})

	content = regexp.MustCompile(`(?m)^h5\.\s*(.+)$`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`(?m)^h5\.\s*(.+)$`).ReplaceAllString(match, "$1")
		return subHeaderColor.Sprintf("##### %s", text)
	})

	content = regexp.MustCompile(`(?m)^h6\.\s*(.+)$`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`(?m)^h6\.\s*(.+)$`).ReplaceAllString(match, "$1")
		return subHeaderColor.Sprintf("###### %s", text)
	})

	// Handle wiki markup formatting
	content = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(match, "$1")
		return emphasisColor.Sprintf("• %s", text)
	})

	// Handle wiki markup code blocks
	content = regexp.MustCompile(`\{\{([^}]+)\}\}`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`\{\{([^}]+)\}\}`).ReplaceAllString(match, "$1")
		return color.New(color.FgHiBlack, color.BgWhite).Sprintf(" %s ", text)
	})

	// Replace HTML headers with colored versions
	content = regexp.MustCompile(`<h1[^>]*>(.*?)</h1>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<h1[^>]*>(.*?)</h1>`).ReplaceAllString(match, "$1")
		return "\n" + headerColor.Sprintf("# %s", text) + "\n"
	})

	content = regexp.MustCompile(`<h2[^>]*>(.*?)</h2>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<h2[^>]*>(.*?)</h2>`).ReplaceAllString(match, "$1")
		return "\n" + headerColor.Sprintf("## %s", text) + "\n"
	})

	content = regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`).ReplaceAllString(match, "$1")
		return "\n" + subHeaderColor.Sprintf("### %s", text) + "\n"
	})

	content = regexp.MustCompile(`<h4[^>]*>(.*?)</h4>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<h4[^>]*>(.*?)</h4>`).ReplaceAllString(match, "$1")
		return "\n" + subHeaderColor.Sprintf("#### %s", text) + "\n"
	})

	// Replace other HTML elements
	content = regexp.MustCompile(`<strong[^>]*>(.*?)</strong>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<strong[^>]*>(.*?)</strong>`).ReplaceAllString(match, "$1")
		return emphasisColor.Sprintf("**%s**", text)
	})

	content = regexp.MustCompile(`<b[^>]*>(.*?)</b>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<b[^>]*>(.*?)</b>`).ReplaceAllString(match, "$1")
		return emphasisColor.Sprintf("**%s**", text)
	})

	content = regexp.MustCompile(`<em[^>]*>(.*?)</em>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<em[^>]*>(.*?)</em>`).ReplaceAllString(match, "$1")
		return emphasisColor.Sprintf("*%s*", text)
	})

	content = regexp.MustCompile(`<i[^>]*>(.*?)</i>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<i[^>]*>(.*?)</i>`).ReplaceAllString(match, "$1")
		return emphasisColor.Sprintf("*%s*", text)
	})

	// Handle links
	content = regexp.MustCompile(`<a[^>]*href=["']([^"']*)["'][^>]*>(.*?)</a>`).ReplaceAllStringFunc(content, func(match string) string {
		re := regexp.MustCompile(`<a[^>]*href=["']([^"']*)["'][^>]*>(.*?)</a>`)
		matches := re.FindStringSubmatch(match)
		if len(matches) >= 3 {
			return linkColor.Sprintf("[%s](%s)", matches[2], matches[1])
		}
		return match
	})

	// Handle line breaks
	content = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`<br[^>]*>`).ReplaceAllString(content, "\n")

	// Handle paragraphs
	content = regexp.MustCompile(`<p[^>]*>(.*?)</p>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<p[^>]*>(.*?)</p>`).ReplaceAllString(match, "$1")
		return "\n" + text + "\n"
	})

	// Handle lists
	content = regexp.MustCompile(`<ul[^>]*>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`</ul>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`<ol[^>]*>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`</ol>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`<li[^>]*>(.*?)</li>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<li[^>]*>(.*?)</li>`).ReplaceAllString(match, "$1")
		return "• " + text + "\n"
	})

	// Handle code blocks and inline code
	content = regexp.MustCompile(`<code[^>]*>(.*?)</code>`).ReplaceAllStringFunc(content, func(match string) string {
		text := regexp.MustCompile(`<code[^>]*>(.*?)</code>`).ReplaceAllString(match, "$1")
		return color.New(color.FgHiBlack, color.BgWhite).Sprintf(" %s ", text)
	})

	// Handle div and span by just removing tags but keeping content
	content = regexp.MustCompile(`<div[^>]*>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`</div>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`<span[^>]*>(.*?)</span>`).ReplaceAllStringFunc(content, func(match string) string {
		return regexp.MustCompile(`<span[^>]*>(.*?)</span>`).ReplaceAllString(match, "$1")
	})

	// Clean up any remaining simple tags
	content = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(content, "")

	// Clean up excessive newlines
	content = regexp.MustCompile(`\n{3,}`).ReplaceAllString(content, "\n\n")
	content = strings.TrimSpace(content)

	return content
}

func init() {
	rootCmd.AddCommand(viewCmd)
	
	viewCmd.Flags().StringVar(&viewFormat, "format", "readable", "Output format (readable or json)")
	viewCmd.Flags().StringVarP(&viewOutput, "output", "o", "", "Output file (default: stdout)")
}