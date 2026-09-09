package cli

import (
	"fmt"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/spf13/cobra"
)

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Manage projects (project directory = one git repo container)"}
	cmd.AddCommand(projectListCmd())
	cmd.AddCommand(projectShowCmd())
	cmd.AddCommand(projectCreateCmd())
	cmd.AddCommand(projectRefreshCodeCmd())
	cmd.AddCommand(projectDeleteCmd())
	return cmd
}

func projectListCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects in a company",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListProjects(cmd.Context(), companyID)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Println("no projects")
				return nil
			}
			rows := [][]string{}
			for _, p := range list {
				rows = append(rows, []string{shortID(p.ID), p.Name, p.RootPath, strOrDashEmpty(p.Description), fmtTime(p.CreatedAt)})
			}
			printTable([]string{"ID", "NAME", "ROOT_PATH", "DESCRIPTION", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	return cmd
}

func projectShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := svc.GetProject(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			rows := [][]string{
				{"ID", p.ID},
				{"COMPANY", p.CompanyID},
				{"NAME", p.Name},
				{"ROOT_PATH", p.RootPath},
				{"DESCRIPTION", p.Description},
			}
			// D7:code source 行(项目详情看代码源;无 → CODE_SOURCE -,刷新用 project refresh-code)。
			if src, serr := svc.CodeSourceFor(cmd.Context(), p.ID); serr != nil {
				return serr
			} else if src != nil {
				owner, repo, ok := github.ParseOwnerRepo(src.RepoURL)
				code := src.RepoURL
				if ok {
					code = owner + "/" + repo
				}
				rows = append(rows, []string{"CODE_SOURCE", code})
			} else {
				rows = append(rows, []string{"CODE_SOURCE", "-"})
			}
			rows = append(rows, []string{"WHEN", fmtTime(p.CreatedAt)})
			printTable([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
	return cmd
}

func projectRefreshCodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh-code <id>",
		Short: "Re-adopt / refresh a project's code source from its git origin remote",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := svc.RefreshProjectCodeSourceAs(cmd.Context(), args[0], "human:cli")
			if err != nil {
				return err
			}
			fmt.Printf("code source for project %s: %s (workspace: %s)\n", shortID(args[0]), r.RepoURL, r.WorkspacePath)
			return nil
		},
	}
	return cmd
}

func projectCreateCmd() *cobra.Command {
	var companyID, name, rootPath, description, repoURL string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project (empty/nonexistent root + --repo-url → OS auto-clones the code; else local git-inited)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := svc.CreateProject(cmd.Context(), companyID, name, rootPath, description, repoURL)
			if err != nil {
				return err
			}
			fmt.Printf("project created (%s)\nid: %s\nroot_path: %s\n", p.Name, p.ID, p.RootPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&name, "name", "", "project name (unique per company)")
	cmd.Flags().StringVar(&rootPath, "root-path", "", "absolute path to the project git directory (existing git, or empty/nonexistent; empty/nonexistent + --repo-url → auto-clone)")
	cmd.Flags().StringVar(&repoURL, "repo-url", "", "GitHub repo URL to clone into an empty/nonexistent root (bind: project ⇄ github repo)")
	cmd.Flags().StringVar(&description, "description", "", "project description")
	return cmd
}

func projectDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a project (no active run; disk directory untouched)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.DeleteProject(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Printf("project %s deleted (metadata only; directory untouched)\n", shortID(args[0]))
			return nil
		},
	}
	return cmd
}
