package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var (
	searchFields     string
	searchExpand     string
	searchProperties string
	searchMax        int
	searchAll        bool
	searchFormat     string
	searchParams     []string
	searchListFields bool
	searchBoard      string
	searchQuick      []string
	searchShowJQL    bool
)

var searchCmd = &cobra.Command{
	Use:   "search [JQL]",
	Short: "Search Jira with any JQL",
	Long: `Search Jira with any JQL and any parameter the search API supports.

Examples:
  jet search 'project = PROJ AND type = Bug AND statusCategory != Done'
  jet search 'assignee = currentUser()' --fields summary,status,customfield_10020 --format json
  jet search 'project = PROJ' --fields '*all' --expand names --max 5 --format json
  jet search 'project = PROJ' --all --format json
  jet search 'labels = a11y' --param fieldsByKeys=true
  jet search --list-fields
  jet search --board 123 --quick-filter 456,789 'duedate <= 3d'`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)

		if searchListFields {
			body, err := client.ListFieldsRaw()
			if err != nil {
				return err
			}
			return printFields(body)
		}
		jql := ""
		if len(args) == 1 {
			jql = args[0]
		}
		if searchBoard != "" {
			jql, err = boardJQL(client, searchBoard, searchQuick, jql)
			if err != nil {
				return err
			}
		} else if len(searchQuick) > 0 {
			return fmt.Errorf("--quick-filter needs --board")
		}
		if jql == "" {
			return fmt.Errorf("give a JQL query, --board, or --list-fields")
		}
		if searchShowJQL {
			fmt.Fprintln(os.Stderr, "JQL:", jql)
		}

		params := url.Values{}
		params.Set("jql", jql)
		params.Set("fields", searchFields)
		if searchExpand != "" {
			params.Set("expand", searchExpand)
		}
		if searchProperties != "" {
			params.Set("properties", searchProperties)
		}
		for _, p := range searchParams {
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				return fmt.Errorf("--param needs key=value, got %q", p)
			}
			params.Add(k, v)
		}

		var issues []json.RawMessage
		for {
			pageSize := searchMax - len(issues)
			if searchAll || pageSize > 100 {
				pageSize = 100
			}
			params.Set("maxResults", fmt.Sprint(pageSize))
			page, err := client.SearchJQLRaw(params)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			issues = append(issues, page.Issues...)
			done := page.IsLast || page.NextPageToken == "" || len(page.Issues) == 0
			if !searchAll && len(issues) >= searchMax {
				done = true
			}
			if done {
				break
			}
			params.Set("nextPageToken", page.NextPageToken)
		}

		if searchFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(issues)
		}

		parsed := make([]jira.Issue, 0, len(issues))
		for _, raw := range issues {
			var is jira.Issue
			if err := json.Unmarshal(raw, &is); err != nil {
				return fmt.Errorf("failed to decode issue: %w", err)
			}
			parsed = append(parsed, is)
		}
		if len(parsed) == 0 {
			fmt.Println("No tickets found matching the query.")
			return nil
		}
		displayIssueList(parsed, len(parsed))
		return nil
	},
}

func printFields(body []byte) error {
	var fields []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Schema struct {
			Type string `json:"type"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(body, &fields); err != nil {
		return fmt.Errorf("failed to decode fields: %w", err)
	}
	if searchFormat == "json" {
		_, err := os.Stdout.Write(body)
		return err
	}
	for _, f := range fields {
		fmt.Printf("%s\t%s\t%s\n", f.ID, f.Name, f.Schema.Type)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().StringVar(&searchFields, "fields", "summary,status,assignee,priority,issuetype,project,created,updated", "Fields to return (comma-separated, '*all', '*navigable', or '-field' to exclude)")
	searchCmd.Flags().StringVar(&searchExpand, "expand", "", "Expand options (for example names,schema,changelog,renderedFields)")
	searchCmd.Flags().StringVar(&searchProperties, "properties", "", "Issue properties to return (comma-separated)")
	searchCmd.Flags().IntVar(&searchMax, "max", 50, "Maximum number of results")
	searchCmd.Flags().BoolVar(&searchAll, "all", false, "Fetch every page of results")
	searchCmd.Flags().StringVar(&searchFormat, "format", "readable", "Output format (readable or json)")
	searchCmd.Flags().StringArrayVar(&searchParams, "param", nil, "Extra search API parameter as key=value (repeatable)")
	searchCmd.Flags().StringVar(&searchBoard, "board", "", "Start from this board's filter")
	searchCmd.Flags().StringSliceVar(&searchQuick, "quick-filter", nil, "Board quick filter ids to apply (needs --board)")
	searchCmd.Flags().BoolVar(&searchShowJQL, "show-jql", false, "Print the final JQL to stderr")
	searchCmd.Flags().BoolVar(&searchListFields, "list-fields", false, "List every Jira field id, name, and type")
}
