package cmd

import (
	"fmt"
	"os"

	"voltload/internal/loader"
	"voltload/reports"

	"github.com/spf13/cobra"
)

// Version is set at build time: -ldflags "-X voltload/cmd.Version=1.2.3"
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run:   func(cmd *cobra.Command, args []string) { fmt.Println("voltload", Version) },
}

var validateCmd = &cobra.Command{
	Use:   "validate <scenario.yaml>",
	Short: "Check a scenario file without running it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loader.LoadScenario(args[0])
		if err != nil {
			return fmt.Errorf("%s: %w", args[0], err)
		}
		fmt.Printf("%s: OK (%s %s, peak %d VUs, %ds, %d stages, %d thresholds)\n",
			args[0], c.Method, c.URL, c.VUs, c.DurationSec, len(c.Stages), len(c.Thresholds))
		return nil
	},
}

var reportOut string

var reportCmd = &cobra.Command{
	Use:     "report <result.json>",
	Short:   "Convert a saved JSON result into a PDF, HTML or CSV report",
	Example: "  voltload report run-001.json -o run-001.pdf",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := reports.ReadJSON(args[0])
		if err != nil {
			return fmt.Errorf("%s: %w", args[0], err)
		}
		if reportOut == "" {
			printResult(r)
			return nil
		}
		if err := reports.WriteFile(reportOut, r); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "report written to", reportOut)
		return nil
	},
}

func init() {
	reportCmd.Flags().StringVarP(&reportOut, "out", "o", "", "output file (.pdf, .html, .csv or .json); prints a summary if omitted")
	rootCmd.AddCommand(versionCmd, validateCmd, reportCmd)
}
