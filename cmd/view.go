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

// Pre-compiled regexes for formatHTMLContent — avoids recompiling on every call.
var (
	reWikiH1     = regexp.MustCompile(`(?m)^h1\.\s*(.+)$`)
	reWikiH2     = regexp.MustCompile(`(?m)^h2\.\s*(.+)$`)
	reWikiH3     = regexp.MustCompile(`(?m)^h3\.\s*(.+)$`)
	reWikiH4     = regexp.MustCompile(`(?m)^h4\.\s*(.+)$`)
	reWikiH5     = regexp.MustCompile(`(?m)^h5\.\s*(.+)$`)
	reWikiH6     = regexp.MustCompile(`(?m)^h6\.\s*(.+)$`)
	reWikiBold   = regexp.MustCompile(`\*([^*]+)\*`)
	reWikiCode   = regexp.MustCompile(`\{\{([^}]+)\}\}`)
	reHTMLH1     = regexp.MustCompile(`<h1[^>]*>(.*?)</h1>`)
	reHTMLH2     = regexp.MustCompile(`<h2[^>]*>(.*?)</h2>`)
	reHTMLH3     = regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`)
	reHTMLH4     = regexp.MustCompile(`<h4[^>]*>(.*?)</h4>`)
	reHTMLStrong = regexp.MustCompile(`<strong[^>]*>(.*?)</strong>`)
	reHTMLB      = regexp.MustCompile(`<b[^>]*>(.*?)</b>`)
	reHTMLEm     = regexp.MustCompile(`<em[^>]*>(.*?)</em>`)
	reHTMLI      = regexp.MustCompile(`<i[^>]*>(.*?)</i>`)
	reHTMLLink   = regexp.MustCompile(`<a[^>]*href=["']([^"']*)["'][^>]*>(.*?)</a>`)
	reHTMLBr     = regexp.MustCompile(`<br\s*/?>`)
	reHTMLBr2    = regexp.MustCompile(`<br[^>]*>`)
	reHTMLP      = regexp.MustCompile(`<p[^>]*>(.*?)</p>`)
	reHTMLUlOpen = regexp.MustCompile(`<ul[^>]*>`)
	reHTMLOlOpen = regexp.MustCompile(`<ol[^>]*>`)
	reHTMLLi     = regexp.MustCompile(`<li[^>]*>(.*?)</li>`)
	reHTMLCode   = regexp.MustCompile(`<code[^>]*>(.*?)</code>`)
	reHTMLDiv    = regexp.MustCompile(`<div[^>]*>`)
	reHTMLSpan   = regexp.MustCompile(`<span[^>]*>(.*?)</span>`)
	reHTMLTag    = regexp.MustCompile(`<[^>]+>`)
	reMultiNL    = regexp.MustCompile(`\n{3,}`)
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

// Shared color palette for issue formatting.
var (
	colCyan    = color.New(color.FgCyan, color.Bold)
	colYellow  = color.New(color.FgYellow, color.Bold)
	colGreen   = color.New(color.FgGreen)
	colBlue    = color.New(color.FgBlue)
	colMagenta = color.New(color.FgMagenta)
	colRed     = color.New(color.FgRed)
	colGray    = color.New(color.FgHiBlack)
)

func formatIssueReadable(issue *jira.Issue) string {
	var output strings.Builder
	formatIssueHeader(&output, issue)
	formatIssueFields(&output, issue)
	formatIssueLinks(&output, issue)
	formatIssuePeople(&output, issue)
	formatIssueMetadata(&output, issue)
	formatIssueDescription(&output, issue)
	formatIssueAttachments(&output, issue)
	formatIssueComments(&output, issue)
	return output.String()
}

func formatIssueHeader(w *strings.Builder, issue *jira.Issue) {
	w.WriteString(colCyan.Sprintf("🎫 TICKET: %s\n", issue.Key))
	w.WriteString(colGray.Sprint("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
	w.WriteString(fmt.Sprintf("%s %s\n", colYellow.Sprint("📝 Summary:"), issue.Fields.Summary))

	statusColor := getStatusColor(issue.Fields.Status.Name)
	w.WriteString(fmt.Sprintf("%s %s\n", colBlue.Sprint("📊 Status:"), statusColor.Sprint(issue.Fields.Status.Name)))
	w.WriteString(fmt.Sprintf("%s %s\n", colMagenta.Sprint("🏷️  Type:"), issue.Fields.IssueType.Name))

	if issue.Fields.Priority.Name != "" {
		priorityColor := getPriorityColor(issue.Fields.Priority.Name)
		w.WriteString(fmt.Sprintf("%s %s\n", colRed.Sprint("⚡ Priority:"), priorityColor.Sprint(issue.Fields.Priority.Name)))
	} else {
		w.WriteString(fmt.Sprintf("%s %s\n", colRed.Sprint("⚡ Priority:"), colGray.Sprint("None")))
	}

	w.WriteString(fmt.Sprintf("%s %s (%s)\n", colGreen.Sprint("📁 Project:"), issue.Fields.Project.Name, colGreen.Sprint(issue.Fields.Project.Key)))
}

func formatIssueFields(w *strings.Builder, issue *jira.Issue) {
	if issue.Fields.EpicLink != nil && issue.Fields.EpicLink.Key != "" {
		if issue.Fields.EpicLink.Summary != "" {
			w.WriteString(fmt.Sprintf("%s %s (%s)\n", colCyan.Sprint("🎯 Epic:"), issue.Fields.EpicLink.Summary, colCyan.Sprint(issue.Fields.EpicLink.Key)))
		} else {
			w.WriteString(fmt.Sprintf("%s %s\n", colCyan.Sprint("🎯 Epic:"), colCyan.Sprint(issue.Fields.EpicLink.Key)))
		}
	} else if issue.Fields.Parent != nil && issue.Fields.Parent.Key != "" {
		if issue.Fields.Parent.Summary != "" {
			w.WriteString(fmt.Sprintf("%s %s (%s)\n", colCyan.Sprint("🔗 Parent:"), issue.Fields.Parent.Summary, colCyan.Sprint(issue.Fields.Parent.Key)))
		} else {
			w.WriteString(fmt.Sprintf("%s %s\n", colCyan.Sprint("🔗 Parent:"), colCyan.Sprint(issue.Fields.Parent.Key)))
		}
	}
}

func formatIssueLinks(w *strings.Builder, issue *jira.Issue) {
	if len(issue.Fields.IssueLinks) == 0 {
		return
	}
	w.WriteString("\n")
	w.WriteString(colCyan.Sprintf("🔗 Linked Issues (%d):\n", len(issue.Fields.IssueLinks)))
	w.WriteString(colGray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))

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
			sc := getStatusColor(linkedIssue.Fields.Status.Name)
			w.WriteString(fmt.Sprintf("  %s %s\n", colMagenta.Sprint(relationshipText+":"), colCyan.Sprint(linkedIssue.Key)))
			w.WriteString(fmt.Sprintf("    %s %s\n", colGray.Sprint("Summary:"), linkedIssue.Fields.Summary))
			w.WriteString(fmt.Sprintf("    %s %s | %s %s\n",
				colGray.Sprint("Status:"), sc.Sprint(linkedIssue.Fields.Status.Name),
				colGray.Sprint("Type:"), linkedIssue.Fields.IssueType.Name))
		}
	}
	w.WriteString("\n")
}

func formatIssuePeople(w *strings.Builder, issue *jira.Issue) {
	if issue.Fields.Assignee != nil {
		name := issue.Fields.Assignee.DisplayName
		if name == "" {
			name = issue.Fields.Assignee.Name
		}
		w.WriteString(fmt.Sprintf("%s %s\n", colBlue.Sprint("👤 Assignee:"), name))
	} else {
		w.WriteString(fmt.Sprintf("%s %s\n", colBlue.Sprint("👤 Assignee:"), colGray.Sprint("Unassigned")))
	}

	if issue.Fields.Reporter != nil {
		name := issue.Fields.Reporter.DisplayName
		if name == "" {
			name = issue.Fields.Reporter.Name
		}
		w.WriteString(fmt.Sprintf("%s %s\n", colBlue.Sprint("📝 Reporter:"), name))
	}
}

func formatIssueMetadata(w *strings.Builder, issue *jira.Issue) {
	if len(issue.Fields.Labels) > 0 {
		w.WriteString(fmt.Sprintf("%s %s\n", colMagenta.Sprint("🏷️  Labels:"), strings.Join(issue.Fields.Labels, ", ")))
	}
	if len(issue.Fields.Components) > 0 {
		var names []string
		for _, comp := range issue.Fields.Components {
			names = append(names, comp.Name)
		}
		w.WriteString(fmt.Sprintf("%s %s\n", colGreen.Sprint("🔧 Components:"), strings.Join(names, ", ")))
	}
	if len(issue.Fields.FixVersions) > 0 {
		var names []string
		for _, v := range issue.Fields.FixVersions {
			names = append(names, v.Name)
		}
		w.WriteString(fmt.Sprintf("%s %s\n", colYellow.Sprint("🎯 Fix Versions:"), strings.Join(names, ", ")))
	}
	if len(issue.Fields.Created) >= 10 {
		w.WriteString(fmt.Sprintf("%s %s\n", colGray.Sprint("📅 Created:"), issue.Fields.Created[:10]))
	}
	if len(issue.Fields.Updated) >= 10 {
		w.WriteString(fmt.Sprintf("%s %s\n", colGray.Sprint("🔄 Updated:"), issue.Fields.Updated[:10]))
	}
}

func formatIssueDescription(w *strings.Builder, issue *jira.Issue) {
	if issue.Fields.DescriptionText == "" {
		return
	}
	w.WriteString("\n")
	w.WriteString(colYellow.Sprint("📖 Description:\n"))
	w.WriteString(colGray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))
	w.WriteString(formatHTMLContent(issue.Fields.DescriptionText) + "\n")
}

func formatIssueAttachments(w *strings.Builder, issue *jira.Issue) {
	attachments := issue.Fields.Attachment
	if len(attachments) == 0 {
		return
	}
	w.WriteString("\n")
	w.WriteString(colGreen.Sprintf("📎 Attachments (%d):\n", len(attachments)))
	w.WriteString(colGray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))

	for i, a := range attachments {
		authorName := a.Author.DisplayName
		if authorName == "" {
			authorName = a.Author.Name
		}
		created := a.Created
		if len(created) >= 10 {
			created = created[:10]
		}
		w.WriteString(fmt.Sprintf("%s %s (%s)\n", colYellow.Sprintf("%d.", i+1), a.Filename, colBlue.Sprint(formatFileSize(a.Size))))
		w.WriteString(fmt.Sprintf("   %s %s\n", colGray.Sprint("Type:"), a.MimeType))
		w.WriteString(fmt.Sprintf("   %s %s (%s)\n", colGray.Sprint("Uploaded by:"), authorName, created))
		if a.Content != "" {
			w.WriteString(fmt.Sprintf("   %s %s\n", colGray.Sprint("URL:"), a.Content))
		}
		w.WriteString("\n")
	}
}

func formatIssueComments(w *strings.Builder, issue *jira.Issue) {
	comments := issue.Fields.Comment.Comments
	if len(comments) == 0 {
		return
	}
	w.WriteString("\n")
	w.WriteString(colCyan.Sprintf("💬 Comments (%d):\n", len(comments)))
	w.WriteString(colGray.Sprint("━━━━━━━━━━━━━━━━━━━━\n"))

	start := 0
	if len(comments) > 5 {
		start = len(comments) - 5
	}
	for i, c := range comments[start:] {
		authorName := c.Author.DisplayName
		if authorName == "" {
			authorName = c.Author.Name
		}
		created := c.Created
		if len(created) >= 10 {
			created = created[:10]
		}
		w.WriteString(fmt.Sprintf("%s %s (%s):\n", colYellow.Sprintf("%d.", i+1), authorName, colGray.Sprint(created)))
		w.WriteString(fmt.Sprintf("   %s\n\n", formatHTMLContent(c.Body)))
	}
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
	content = reWikiH1.ReplaceAllStringFunc(content, func(match string) string {
		return headerColor.Sprintf("# %s", reWikiH1.ReplaceAllString(match, "$1"))
	})
	content = reWikiH2.ReplaceAllStringFunc(content, func(match string) string {
		return headerColor.Sprintf("## %s", reWikiH2.ReplaceAllString(match, "$1"))
	})
	content = reWikiH3.ReplaceAllStringFunc(content, func(match string) string {
		return subHeaderColor.Sprintf("### %s", reWikiH3.ReplaceAllString(match, "$1"))
	})
	content = reWikiH4.ReplaceAllStringFunc(content, func(match string) string {
		return subHeaderColor.Sprintf("#### %s", reWikiH4.ReplaceAllString(match, "$1"))
	})
	content = reWikiH5.ReplaceAllStringFunc(content, func(match string) string {
		return subHeaderColor.Sprintf("##### %s", reWikiH5.ReplaceAllString(match, "$1"))
	})
	content = reWikiH6.ReplaceAllStringFunc(content, func(match string) string {
		return subHeaderColor.Sprintf("###### %s", reWikiH6.ReplaceAllString(match, "$1"))
	})

	// Handle wiki markup formatting
	content = reWikiBold.ReplaceAllStringFunc(content, func(match string) string {
		return emphasisColor.Sprintf("• %s", reWikiBold.ReplaceAllString(match, "$1"))
	})

	// Handle wiki markup code blocks
	codeStyle := color.New(color.FgHiBlack, color.BgWhite)
	content = reWikiCode.ReplaceAllStringFunc(content, func(match string) string {
		return codeStyle.Sprintf(" %s ", reWikiCode.ReplaceAllString(match, "$1"))
	})

	// Replace HTML headers with colored versions
	content = reHTMLH1.ReplaceAllStringFunc(content, func(match string) string {
		return "\n" + headerColor.Sprintf("# %s", reHTMLH1.ReplaceAllString(match, "$1")) + "\n"
	})
	content = reHTMLH2.ReplaceAllStringFunc(content, func(match string) string {
		return "\n" + headerColor.Sprintf("## %s", reHTMLH2.ReplaceAllString(match, "$1")) + "\n"
	})
	content = reHTMLH3.ReplaceAllStringFunc(content, func(match string) string {
		return "\n" + subHeaderColor.Sprintf("### %s", reHTMLH3.ReplaceAllString(match, "$1")) + "\n"
	})
	content = reHTMLH4.ReplaceAllStringFunc(content, func(match string) string {
		return "\n" + subHeaderColor.Sprintf("#### %s", reHTMLH4.ReplaceAllString(match, "$1")) + "\n"
	})

	// Replace other HTML elements
	content = reHTMLStrong.ReplaceAllStringFunc(content, func(match string) string {
		return emphasisColor.Sprintf("**%s**", reHTMLStrong.ReplaceAllString(match, "$1"))
	})
	content = reHTMLB.ReplaceAllStringFunc(content, func(match string) string {
		return emphasisColor.Sprintf("**%s**", reHTMLB.ReplaceAllString(match, "$1"))
	})
	content = reHTMLEm.ReplaceAllStringFunc(content, func(match string) string {
		return emphasisColor.Sprintf("*%s*", reHTMLEm.ReplaceAllString(match, "$1"))
	})
	content = reHTMLI.ReplaceAllStringFunc(content, func(match string) string {
		return emphasisColor.Sprintf("*%s*", reHTMLI.ReplaceAllString(match, "$1"))
	})

	// Handle links
	content = reHTMLLink.ReplaceAllStringFunc(content, func(match string) string {
		matches := reHTMLLink.FindStringSubmatch(match)
		if len(matches) >= 3 {
			return linkColor.Sprintf("[%s](%s)", matches[2], matches[1])
		}
		return match
	})

	// Handle line breaks
	content = reHTMLBr.ReplaceAllString(content, "\n")
	content = reHTMLBr2.ReplaceAllString(content, "\n")

	// Handle paragraphs
	content = reHTMLP.ReplaceAllStringFunc(content, func(match string) string {
		return "\n" + reHTMLP.ReplaceAllString(match, "$1") + "\n"
	})

	// Handle lists
	content = reHTMLUlOpen.ReplaceAllString(content, "\n")
	content = strings.ReplaceAll(content, "</ul>", "\n")
	content = reHTMLOlOpen.ReplaceAllString(content, "\n")
	content = strings.ReplaceAll(content, "</ol>", "\n")
	content = reHTMLLi.ReplaceAllStringFunc(content, func(match string) string {
		return "• " + reHTMLLi.ReplaceAllString(match, "$1") + "\n"
	})

	// Handle code blocks and inline code
	content = reHTMLCode.ReplaceAllStringFunc(content, func(match string) string {
		return codeStyle.Sprintf(" %s ", reHTMLCode.ReplaceAllString(match, "$1"))
	})

	// Handle div and span by just removing tags but keeping content
	content = reHTMLDiv.ReplaceAllString(content, "")
	content = strings.ReplaceAll(content, "</div>", "\n")
	content = reHTMLSpan.ReplaceAllStringFunc(content, func(match string) string {
		return reHTMLSpan.ReplaceAllString(match, "$1")
	})

	// Clean up any remaining simple tags
	content = reHTMLTag.ReplaceAllString(content, "")

	// Clean up excessive newlines
	content = reMultiNL.ReplaceAllString(content, "\n\n")
	content = strings.TrimSpace(content)

	return content
}

func init() {
	rootCmd.AddCommand(viewCmd)
	
	viewCmd.Flags().StringVar(&viewFormat, "format", "readable", "Output format (readable or json)")
	viewCmd.Flags().StringVarP(&viewOutput, "output", "o", "", "Output file (default: stdout)")
}