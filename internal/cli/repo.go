package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func repoCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "repo", Short: "Manage R&D repositories (issue -> workspace mapping)"}
	cmd.AddCommand(repoAddCmd())
	cmd.AddCommand(repoListCmd())
	return cmd
}

func repoAddCmd() *cobra.Command {
	var companyID, name, repoURL, workspace string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register an R&D repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := svc.AddRepo(cmd.Context(), companyID, name, repoURL, workspace)
			if err != nil {
				return err
			}
			fmt.Printf("repo registered (%s)\nid: %s\nworkspace: %s\n", r.Name, r.ID, strOrDashEmpty(r.WorkspacePath))
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&name, "name", "", "repo name (unique per company)")
	cmd.Flags().StringVar(&repoURL, "repo-url", "", "repo url (github.com/owner/repo; channel B parses owner/repo from it)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "repo workspace directory (git/file tool boundary)")
	return cmd
}

func repoListCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered repositories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListRepos(cmd.Context(), companyID)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, r := range list {
				rows = append(rows, []string{shortID(r.ID), r.Name, r.RepoURL, strOrDashEmpty(r.WorkspacePath), fmtTime(r.CreatedAt)})
			}
			printTable([]string{"ID", "NAME", "REPO_URL", "WORKSPACE", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	return cmd
}
