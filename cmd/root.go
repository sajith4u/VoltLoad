package cmd

import (
	"fmt"
	"io/fs"
	"os"

	"voltload/internal/gui"

	"github.com/spf13/cobra"
)

var assets fs.FS

var rootCmd = &cobra.Command{
	Use:   "voltload",
	Short: "A load testing tool for APIs",
	// Execute prints the error once; don't dump usage for runtime errors like a bad URL.
	SilenceErrors: true,
	SilenceUsage:  true,
	// No subcommand opens the desktop UI, so a double-clicked app binary and `wails dev` both land here.
	Run: func(cmd *cobra.Command, args []string) {
		if err := gui.Run(assets); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	},
}

func Execute(frontend fs.FS) {
	assets = frontend
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
