package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/marcioaguiar/deploy-pr/internal/kube"
)

func newListCmd(root *rootOpts) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active preview environments",
		RunE: func(c *cobra.Command, _ []string) error {
			previews, err := kube.List(c.Context(), kube.ListInput{
				NamespacePrefix: root.cfg.Namespace.Prefix,
				BaseDomain:      root.cfg.BaseDomain,
				Logger:          root.logger,
			})
			if err != nil {
				return infraErr(err)
			}

			if len(previews) == 0 {
				fmt.Fprintln(root.stdout, "No active previews.")
				return nil
			}

			switch output {
			case "json":
				enc := json.NewEncoder(root.stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(previews)
			case "text", "":
				tw := tabwriter.NewWriter(root.stdout, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "PR\tNAMESPACE\tIMAGE TAG\tAGE\tSTATUS\tURL")
				for _, p := range previews {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n",
						p.PRNumber, p.Namespace, p.ImageTag, p.Age, p.Status, p.URL)
				}
				return tw.Flush()
			default:
				return userErr(fmt.Errorf("unsupported --output %q (want text or json)", output))
			}
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text or json")
	return cmd
}
