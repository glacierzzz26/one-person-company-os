package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func endpointCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "endpoint", Short: "Manage model endpoints (model pool)"}
	cmd.AddCommand(endpointAddCmd())
	cmd.AddCommand(endpointListCmd())
	cmd.AddCommand(endpointShowCmd())
	cmd.AddCommand(endpointModelsCmd())
	cmd.AddCommand(endpointSelectCmd())
	return cmd
}

func endpointAddCmd() *cobra.Command {
	var companyID, name, baseURL, token, proto string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a model endpoint (token is encrypted at rest)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := svc.AddEndpoint(cmd.Context(), companyID, name, baseURL, token, proto)
			if err != nil {
				return err
			}
			fmt.Printf("endpoint added (%s/%s)\nid: %s\n", e.Vendor, e.Proto, e.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&name, "name", "", "endpoint name")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "base url (e.g. https://api.anthropic.com/v1)")
	cmd.Flags().StringVar(&token, "token", "", "auth token (encrypted; needs OS_ENDPOINT_KEY)")
	cmd.Flags().StringVar(&proto, "proto", "auto", "protocol: auto|anthropic|openai")
	return cmd
}

func endpointListCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List endpoints (token never shown)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" {
				return fmt.Errorf("--company is required")
			}
			list, err := svc.ListEndpoints(cmd.Context(), companyID)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, e := range list {
				rows = append(rows, []string{
					shortID(e.ID), e.Name, e.BaseURL, e.Vendor, e.Proto, e.Role, tierCN(e.Tier), e.SelectedModel, e.Status, fmtTime(e.CreatedAt),
				})
			}
			printTable([]string{"ID", "NAME", "BASE_URL", "VENDOR", "PROTO", "ROLE", "TIER", "MODEL", "STATUS", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	return cmd
}

func endpointShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show an endpoint (token redacted)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := svc.GetEndpoint(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			token := "none"
			if e.TokenEnc != "" {
				token = fmt.Sprintf("encrypted (aesgcm, %d chars)", len(e.TokenEnc))
			}
			cache := ""
			if e.ModelsCache != "" {
				cache = fmt.Sprintf("%d chars (last /v1/models)", len(e.ModelsCache))
			}
			rows := [][]string{
				{"ID", e.ID},
				{"COMPANY", e.CompanyID},
				{"NAME", e.Name},
				{"BASE_URL", e.BaseURL},
				{"TOKEN", token},
				{"PROTO", e.Proto},
				{"VENDOR", e.Vendor},
				{"MODEL", e.SelectedModel},
				{"ROLE", e.Role},
				{"TIER", tierCN(e.Tier)},
				{"STATUS", e.Status},
				{"MODELS_CACHE", cache},
				{"WHEN", fmtTime(e.CreatedAt)},
			}
			printTable([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
}

func endpointModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models <id>",
		Short: "Test connection and list available models (/v1/models)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			models, err := svc.FetchEndpointModels(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, m := range models {
				rows = append(rows, []string{m.ID})
			}
			printTable([]string{"MODEL"}, rows)
			fmt.Printf("%d model(s) from endpoint %s; select one: os endpoint select %s --model <model>\n",
				len(models), shortID(args[0]), shortID(args[0]))
			return nil
		},
	}
}

func endpointSelectCmd() *cobra.Command {
	var model, role, tier string
	cmd := &cobra.Command{
		Use:   "select <id>",
		Short: "Select the endpoint model (optionally assign role/tier: pool|planner|standby; frontier|standard|cheap)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := svc.SelectEndpointModel(cmd.Context(), args[0], model, role, tier)
			if err != nil {
				return err
			}
			fmt.Printf("endpoint %s -> model %s (role %s tier %s)\n", e.Name, e.SelectedModel, e.Role, tierCN(e.Tier))
			return nil
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "model id to select")
	cmd.Flags().StringVar(&role, "role", "", "role: pool|planner|standby")
	cmd.Flags().StringVar(&tier, "tier", "", "tier: frontier|standard|cheap (UI 高智/均衡/经济)")
	return cmd
}

// tierCN tier → 中文标注(方向 §十 1);未知/空档原样显示。
func tierCN(t string) string {
	switch t {
	case "frontier":
		return "高智"
	case "standard":
		return "均衡"
	case "cheap":
		return "经济"
	default:
		return t
	}
}
