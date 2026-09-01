package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func companyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "company", Short: "Manage companies"}
	cmd.AddCommand(companyCreateCmd())
	cmd.AddCommand(companyListCmd())
	cmd.AddCommand(companyShowCmd())
	return cmd
}

func companyCreateCmd() *cobra.Command {
	var name, vision string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a company",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			c, err := svc.CreateCompany(cmd.Context(), name, vision)
			if err != nil {
				return err
			}
			fmt.Printf("created company %q\nid: %s\n", c.Name, c.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "company name")
	cmd.Flags().StringVar(&vision, "vision", "", "company vision")
	return cmd
}

func companyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List companies",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListCompanies(cmd.Context())
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, c := range list {
				rows = append(rows, []string{shortID(c.ID), c.Name, c.Vision, fmtTime(c.CreatedAt)})
			}
			printTable([]string{"ID", "NAME", "VISION", "CREATED"}, rows)
			return nil
		},
	}
}

func companyShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show company details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := svc.GetCompany(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("ID:      %s\nName:    %s\nVision:  %s\nCreated: %s\n",
				c.ID, c.Name, c.Vision, fmtTime(c.CreatedAt))
			return nil
		},
	}
}
