package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func memoryCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Manage company knowledge (Memory)"}
	cmd.AddCommand(memoryAddCmd())
	cmd.AddCommand(memoryListCmd())
	cmd.AddCommand(memorySearchCmd())
	return cmd
}

func memoryAddCmd() *cobra.Command {
	var companyID, mtype, title, content, source, tags string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Record a knowledge entry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || mtype == "" || title == "" {
				return fmt.Errorf("--company, --type and --title are required")
			}
			m, err := svc.CreateMemory(cmd.Context(), companyID, mtype, title, content, source, tags)
			if err != nil {
				return err
			}
			fmt.Printf("memory recorded (%s)\nid: %s\n", m.Type, m.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&mtype, "type", "", "memory type (lesson/knowledge/project_context/decision_ref/architecture/convention/task_history)")
	cmd.Flags().StringVar(&title, "title", "", "title")
	cmd.Flags().StringVar(&content, "content", "", "content")
	cmd.Flags().StringVar(&source, "source", "", "provenance (workflow:<id>/task:<id>/approval:<id>/manual)")
	cmd.Flags().StringVar(&tags, "tags", "", "space-separated tags")
	return cmd
}

func memoryListCmd() *cobra.Command {
	var companyID, mtype string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memory entries (optionally by type)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" {
				return fmt.Errorf("--company is required")
			}
			list, err := svc.ListMemories(cmd.Context(), companyID, mtype)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, m := range list {
				rows = append(rows, []string{
					shortID(m.ID), m.Type, m.Title, firstLine(m.Content), m.Source, fmtTime(m.CreatedAt),
				})
			}
			printTable([]string{"ID", "TYPE", "TITLE", "CONTENT", "SOURCE", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&mtype, "type", "", "filter by memory type")
	return cmd
}

func memorySearchCmd() *cobra.Command {
	var companyID, q string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Full-text search memory (FTS5)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || q == "" {
				return fmt.Errorf("--company and --q are required")
			}
			list, err := svc.SearchMemories(cmd.Context(), companyID, q)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Printf("no memory matches %q\n", q)
				return nil
			}
			rows := [][]string{}
			for _, m := range list {
				rows = append(rows, []string{
					shortID(m.ID), m.Type, m.Title, firstLine(m.Content), m.Source, fmtTime(m.CreatedAt),
				})
			}
			printTable([]string{"ID", "TYPE", "TITLE", "CONTENT", "SOURCE", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&q, "q", "", "full-text query (FTS5)")
	return cmd
}
