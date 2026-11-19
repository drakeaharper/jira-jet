package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"jet/internal/config"
	"jet/internal/jira"
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Inspect Jira boards",
}

var boardQuickFiltersCmd = &cobra.Command{
	Use:   "quickfilters BOARD-ID",
	Short: "List a board's quick filters and their JQL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}
		client := jira.NewClient(cfg.URL, cfg.Email, cfg.Username, cfg.Token)
		qfs, err := client.BoardQuickFilters(args[0])
		if err != nil {
			return err
		}
		for _, q := range qfs {
			fmt.Printf("%d\t%s\t%s\n", q.ID, q.Name, q.JQL)
		}
		return nil
	},
}

// boardJQL joins a board's filter, the chosen quick filters, and extra JQL into one query.
func boardJQL(client *jira.Client, boardID string, quickFilterIDs []string, extra string) (string, error) {
	base, err := client.BoardFilterJQL(boardID)
	if err != nil {
		return "", err
	}
	parts := []string{}
	order := ""
	if i := strings.Index(strings.ToLower(base), "order by"); i >= 0 {
		base, order = strings.TrimSpace(base[:i]), base[i:]
	}
	if base != "" {
		parts = append(parts, "("+base+")")
	}
	if len(quickFilterIDs) > 0 {
		qfs, err := client.BoardQuickFilters(boardID)
		if err != nil {
			return "", err
		}
		byID := map[string]string{}
		for _, q := range qfs {
			byID[strconv.Itoa(q.ID)] = q.JQL
		}
		for _, id := range quickFilterIDs {
			j, ok := byID[strings.TrimSpace(id)]
			if !ok {
				return "", fmt.Errorf("board %s has no quick filter %s", boardID, id)
			}
			parts = append(parts, "("+j+")")
		}
	}
	if extra != "" {
		parts = append(parts, "("+extra+")")
	}
	return strings.TrimSpace(strings.Join(parts, " AND ") + " " + order), nil
}

func init() {
	rootCmd.AddCommand(boardCmd)
	boardCmd.AddCommand(boardQuickFiltersCmd)
}
