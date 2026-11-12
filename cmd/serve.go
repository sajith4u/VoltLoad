package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"voltload/internal/gui"
	"voltload/internal/store"

	"github.com/spf13/cobra"
)

var (
	serveAddr    string
	serveDataDir string
	serveOpen    bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the full VoltLoad app in your browser (no desktop window needed)",
	Long: `Starts the VoltLoad backend and UI as a local web server: same engine, database, reports and
logs as the desktop app. It listens on loopback only. Ctrl+C stops it; a running test is stopped
and its result saved first.`,
	Example: "  voltload serve --open",
	RunE: func(cmd *cobra.Command, args []string) error {
		if serveDataDir == "" {
			dir, err := store.DataDir()
			if err != nil {
				return err
			}
			serveDataDir = dir
		}
		ui, err := fs.Sub(assets, "frontend/dist") // the embed root is the repo; the built UI is in frontend/dist
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return gui.Serve(ctx, ui, serveAddr, serveDataDir, func(url string) {
			fmt.Fprintf(os.Stderr, "VoltLoad is running at %s  (Ctrl+C to stop)\n", url)
			if serveOpen {
				openBrowser(url)
			}
		})
	},
}

func openBrowser(url string) {
	name, args := "xdg-open", []string{url}
	switch runtime.GOOS {
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		name = "open"
	}
	if err := exec.Command(name, args...).Start(); err != nil {
		fmt.Fprintln(os.Stderr, "open the URL above in your browser:", err)
	}
}

func init() {
	f := serveCmd.Flags()
	f.StringVar(&serveAddr, "addr", "127.0.0.1:7878", "address to listen on (loopback only)")
	f.StringVar(&serveDataDir, "data-dir", "", "where the database, logs and reports live (default: the OS app-data folder)")
	f.BoolVar(&serveOpen, "open", false, "open the app in the default browser")
	rootCmd.AddCommand(serveCmd)
}
