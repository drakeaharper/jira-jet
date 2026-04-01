package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jet/internal/jira"
)

type DetailModel struct {
	viewport    viewport.Model
	issue       *jira.Issue
	loading     bool
	spinner     spinner.Model
	ready       bool
	width       int
	height      int
	commenting  bool
	commentArea textarea.Model
}

func NewDetailModel() DetailModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)

	ta := textarea.New()
	ta.Placeholder = "Write a comment..."
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	return DetailModel{
		loading:     true,
		spinner:     s,
		commentArea: ta,
	}
}

func (d DetailModel) SetSize(width, height int) DetailModel {
	d.width = width
	d.height = height
	if d.commenting {
		d.viewport = viewport.New(width, height-5)
	} else {
		d.viewport = viewport.New(width, height)
	}
	d.viewport.HighPerformanceRendering = false
	d.ready = true
	if d.issue != nil {
		d.viewport.SetContent(d.renderContent())
	}
	return d
}

func (d DetailModel) SetIssue(issue *jira.Issue) DetailModel {
	d.issue = issue
	d.loading = false
	if d.ready {
		d.viewport.SetContent(d.renderContent())
		d.viewport.GotoTop()
	}
	return d
}

func (d DetailModel) Update(msg tea.Msg, client *jira.Client) (DetailModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if d.commenting {
			switch msg.String() {
			case "esc":
				d.commenting = false
				d.commentArea.Blur()
				d.viewport = viewport.New(d.width, d.height)
				if d.issue != nil {
					d.viewport.SetContent(d.renderContent())
				}
				return d, nil
			case "ctrl+s":
				comment := strings.TrimSpace(d.commentArea.Value())
				if comment != "" && d.issue != nil {
					d.commenting = false
					d.commentArea.Blur()
					d.commentArea.Reset()
					return d, addCommentCmd(client, d.issue.Key, comment)
				}
				return d, nil
			}
			var cmd tea.Cmd
			d.commentArea, cmd = d.commentArea.Update(msg)
			return d, cmd
		}

		switch {
		case key.Matches(msg, globalKeys.Back):
			return d, func() tea.Msg { return goBackMsg{} }

		case msg.String() == "j":
			d.viewport.LineDown(1)
			return d, nil
		case msg.String() == "k":
			d.viewport.LineUp(1)
			return d, nil

		case key.Matches(msg, detailKeys.Edit):
			if d.issue != nil {
				return d, func() tea.Msg { return navigateToFormMsg{issue: d.issue} }
			}

		case key.Matches(msg, detailKeys.Transition):
			if d.issue != nil {
				return d, func() tea.Msg { return navigateToTransitionMsg{key: d.issue.Key} }
			}

		case key.Matches(msg, detailKeys.Comment):
			d.commenting = true
			d.commentArea.Focus()
			d.viewport = viewport.New(d.width, d.height-5)
			if d.issue != nil {
				d.viewport.SetContent(d.renderContent())
			}
			return d, d.commentArea.Focus()

		case key.Matches(msg, detailKeys.Grab):
			if d.issue != nil {
				return d, grabIssueCmd(client, d.issue.Key)
			}

		case key.Matches(msg, detailKeys.Claude):
			if d.issue != nil {
				issueCopy := *d.issue
				return d, func() tea.Msg {
					return launchClaudeTaskMsg{issue: &issueCopy, instruction: ""}
				}
			}

		case key.Matches(msg, detailKeys.Open):
			// We don't have the base URL here, so skip browser open for now
			return d, nil
		}

	case spinner.TickMsg:
		if d.loading {
			var cmd tea.Cmd
			d.spinner, cmd = d.spinner.Update(msg)
			cmds = append(cmds, cmd)
			return d, tea.Batch(cmds...)
		}
	}

	// Pass through to viewport for scrolling
	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return d, tea.Batch(cmds...)
}

func (d DetailModel) View() string {
	if d.loading {
		return lipgloss.Place(
			d.width, d.height,
			lipgloss.Center, lipgloss.Center,
			d.spinner.View()+" Loading ticket...",
		)
	}

	if d.commenting {
		return lipgloss.JoinVertical(lipgloss.Left,
			d.viewport.View(),
			lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorCyan).Render(d.commentArea.View()),
			dimStyle.Render("  ctrl+s: submit  esc: cancel"),
		)
	}

	return d.viewport.View()
}

func (d DetailModel) renderContent() string {
	if d.issue == nil {
		return ""
	}

	var b strings.Builder
	issue := d.issue
	w := d.width - 2

	// Header
	b.WriteString(titleStyle.Render(fmt.Sprintf("  %s", issue.Key)) + "\n")
	b.WriteString(headerStyle.Render(issue.Fields.Summary) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", min(w, 60))) + "\n\n")

	// Status badge + Type + Priority
	status := StatusStyle(issue.Fields.Status.Name).Render(fmt.Sprintf(" %s ", issue.Fields.Status.Name))
	line := status
	if issue.Fields.IssueType.Name != "" {
		line += "  " + dimStyle.Render("Type:") + " " + valueStyle.Render(issue.Fields.IssueType.Name)
	}
	if issue.Fields.Priority.Name != "" {
		line += "  " + dimStyle.Render("Priority:") + " " + PriorityStyle(issue.Fields.Priority.Name).Render(issue.Fields.Priority.Name)
	}
	b.WriteString(line + "\n\n")

	// Project
	b.WriteString(labelStyle.Render("Project: ") + valueStyle.Render(fmt.Sprintf("%s (%s)", issue.Fields.Project.Name, issue.Fields.Project.Key)) + "\n")

	// Assignee
	assignee := "Unassigned"
	if issue.Fields.Assignee != nil {
		assignee = issue.Fields.Assignee.DisplayName
		if assignee == "" {
			assignee = issue.Fields.Assignee.Name
		}
	}
	b.WriteString(labelStyle.Render("Assignee: ") + valueStyle.Render(assignee) + "\n")

	// Reporter
	if issue.Fields.Reporter != nil {
		reporter := issue.Fields.Reporter.DisplayName
		if reporter == "" {
			reporter = issue.Fields.Reporter.Name
		}
		b.WriteString(labelStyle.Render("Reporter: ") + valueStyle.Render(reporter) + "\n")
	}

	// Epic / Parent
	if issue.Fields.EpicLink != nil && issue.Fields.EpicLink.Key != "" {
		epic := issue.Fields.EpicLink.Key
		if issue.Fields.EpicLink.Summary != "" {
			epic = fmt.Sprintf("%s (%s)", issue.Fields.EpicLink.Summary, issue.Fields.EpicLink.Key)
		}
		b.WriteString(labelStyle.Render("Epic: ") + lipgloss.NewStyle().Foreground(colorCyan).Render(epic) + "\n")
	} else if issue.Fields.Parent != nil && issue.Fields.Parent.Key != "" {
		parent := issue.Fields.Parent.Key
		if issue.Fields.Parent.Summary != "" {
			parent = fmt.Sprintf("%s (%s)", issue.Fields.Parent.Summary, issue.Fields.Parent.Key)
		}
		b.WriteString(labelStyle.Render("Parent: ") + lipgloss.NewStyle().Foreground(colorCyan).Render(parent) + "\n")
	}

	// Labels
	if len(issue.Fields.Labels) > 0 {
		b.WriteString(labelStyle.Render("Labels: ") + lipgloss.NewStyle().Foreground(colorMagenta).Render(strings.Join(issue.Fields.Labels, ", ")) + "\n")
	}

	// Components
	if len(issue.Fields.Components) > 0 {
		var names []string
		for _, c := range issue.Fields.Components {
			names = append(names, c.Name)
		}
		b.WriteString(labelStyle.Render("Components: ") + valueStyle.Render(strings.Join(names, ", ")) + "\n")
	}

	// Fix Versions
	if len(issue.Fields.FixVersions) > 0 {
		var names []string
		for _, v := range issue.Fields.FixVersions {
			names = append(names, v.Name)
		}
		b.WriteString(labelStyle.Render("Fix Versions: ") + valueStyle.Render(strings.Join(names, ", ")) + "\n")
	}

	// Dates
	if len(issue.Fields.Created) >= 10 {
		b.WriteString(dimStyle.Render("Created: "+issue.Fields.Created[:10]) + "  ")
	}
	if len(issue.Fields.Updated) >= 10 {
		b.WriteString(dimStyle.Render("Updated: "+issue.Fields.Updated[:10]))
	}
	b.WriteString("\n")

	// Linked Issues
	if len(issue.Fields.IssueLinks) > 0 {
		b.WriteString("\n" + titleStyle.Render(fmt.Sprintf("Linked Issues (%d)", len(issue.Fields.IssueLinks))) + "\n")
		b.WriteString(dimStyle.Render(strings.Repeat("─", min(w, 40))) + "\n")
		for _, link := range issue.Fields.IssueLinks {
			var linkedIssue *jira.LinkedIssue
			var rel string
			if link.OutwardIssue != nil {
				linkedIssue = link.OutwardIssue
				rel = link.Type.Outward
			} else if link.InwardIssue != nil {
				linkedIssue = link.InwardIssue
				rel = link.Type.Inward
			}
			if linkedIssue != nil {
				status := StatusStyle(linkedIssue.Fields.Status.Name).Render(linkedIssue.Fields.Status.Name)
				b.WriteString(fmt.Sprintf("  %s %s  %s\n", lipgloss.NewStyle().Foreground(colorMagenta).Render(rel+":"),
					lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(linkedIssue.Key),
					status))
				b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(linkedIssue.Fields.Summary)))
			}
		}
	}

	// Description
	if issue.Fields.DescriptionText != "" {
		b.WriteString("\n" + titleStyle.Render("Description") + "\n")
		b.WriteString(dimStyle.Render(strings.Repeat("─", min(w, 40))) + "\n")
		desc := cleanHTMLForTUI(issue.Fields.DescriptionText)
		b.WriteString(desc + "\n")
	}

	// Attachments
	if len(issue.Fields.Attachment) > 0 {
		b.WriteString("\n" + titleStyle.Render(fmt.Sprintf("Attachments (%d)", len(issue.Fields.Attachment))) + "\n")
		b.WriteString(dimStyle.Render(strings.Repeat("─", min(w, 40))) + "\n")
		for i, att := range issue.Fields.Attachment {
			size := formatFileSize(att.Size)
			b.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1,
				valueStyle.Render(att.Filename),
				dimStyle.Render(size)))
		}
	}

	// Comments
	comments := issue.Fields.Comment.Comments
	if len(comments) > 0 {
		b.WriteString("\n" + titleStyle.Render(fmt.Sprintf("Comments (%d)", len(comments))) + "\n")
		b.WriteString(dimStyle.Render(strings.Repeat("─", min(w, 40))) + "\n")
		start := 0
		if len(comments) > 10 {
			start = len(comments) - 10
			b.WriteString(dimStyle.Render(fmt.Sprintf("  ... %d earlier comments hidden ...\n\n", start)))
		}
		for _, comment := range comments[start:] {
			author := comment.Author.DisplayName
			if author == "" {
				author = comment.Author.Name
			}
			created := comment.Created
			if len(created) >= 10 {
				created = created[:10]
			}
			b.WriteString(fmt.Sprintf("  %s  %s\n", lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Render(author), dimStyle.Render(created)))
			body := cleanHTMLForTUI(comment.Body)
			// Indent comment body
			for _, line := range strings.Split(body, "\n") {
				b.WriteString("    " + line + "\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// cleanHTMLForTUI strips HTML tags and wiki markup, returning plain text.
func cleanHTMLForTUI(content string) string {
	if content == "" {
		return content
	}

	// Wiki markup headers
	for i := 6; i >= 1; i-- {
		prefix := fmt.Sprintf("h%d.", i)
		re := regexp.MustCompile(fmt.Sprintf(`(?m)^%s\s*(.+)$`, prefix))
		hdr := strings.Repeat("#", i) + " "
		content = re.ReplaceAllString(content, hdr+"$1")
	}

	// HTML headers
	for i := 1; i <= 6; i++ {
		re := regexp.MustCompile(fmt.Sprintf(`<h%d[^>]*>(.*?)</h%d>`, i, i))
		hdr := strings.Repeat("#", i) + " "
		content = re.ReplaceAllString(content, "\n"+hdr+"$1\n")
	}

	// Bold/italic
	content = regexp.MustCompile(`<strong[^>]*>(.*?)</strong>`).ReplaceAllString(content, "$1")
	content = regexp.MustCompile(`<b[^>]*>(.*?)</b>`).ReplaceAllString(content, "$1")
	content = regexp.MustCompile(`<em[^>]*>(.*?)</em>`).ReplaceAllString(content, "$1")
	content = regexp.MustCompile(`<i[^>]*>(.*?)</i>`).ReplaceAllString(content, "$1")

	// Links
	content = regexp.MustCompile(`<a[^>]*href=["']([^"']*)["'][^>]*>(.*?)</a>`).ReplaceAllString(content, "$2 ($1)")

	// Line breaks and paragraphs
	content = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`<p[^>]*>(.*?)</p>`).ReplaceAllString(content, "\n$1\n")

	// Lists
	content = regexp.MustCompile(`<li[^>]*>(.*?)</li>`).ReplaceAllString(content, "  - $1\n")
	content = regexp.MustCompile(`<ul[^>]*>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`</ul>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`<ol[^>]*>`).ReplaceAllString(content, "\n")
	content = regexp.MustCompile(`</ol>`).ReplaceAllString(content, "")

	// Code
	content = regexp.MustCompile(`<code[^>]*>(.*?)</code>`).ReplaceAllString(content, "`$1`")
	content = regexp.MustCompile(`\{\{([^}]+)\}\}`).ReplaceAllString(content, "`$1`")

	// Strip remaining tags
	content = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(content, "")

	// Clean up whitespace
	content = regexp.MustCompile(`\n{3,}`).ReplaceAllString(content, "\n\n")
	content = strings.TrimSpace(content)

	return content
}

// formatFileSize converts bytes to human-readable size.
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

