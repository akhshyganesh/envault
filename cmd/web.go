package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/akhshyganesh/envault/internal/web"
)

const defaultWebPort = 7391

var (
	webPort   int
	webNoOpen bool
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Open the envault UI in your browser",
	Long: `Serves the same things the terminal does — browse backups, view and restore
versions, scan, manage watch directories, control the daemon, export or import
a vault, and read a backup zip.

It binds 127.0.0.1 and every request needs the one-time token in the URL it
prints, so nothing else on your network can reach it. It does serve your .env
contents in the clear over local HTTP, so leave it running only while you are
using it.

Press Ctrl+C to stop.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := web.NewToken()
		if err != nil {
			return err
		}
		srv, err := web.NewServer(token)
		if err != nil {
			return err
		}

		// A busy default port rolls forward; a port the user asked for does not,
		// because silently using a different one would be worse than failing.
		ln, err := listenLocal(webPort, !cmd.Flags().Changed("port"))
		if err != nil {
			return err
		}
		url := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", ln.Addr().(*net.TCPAddr).Port, token)

		server := &http.Server{
			Handler:           srv.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
		}

		fmt.Println("envault is running in your browser")
		fmt.Printf("   %s\n\n", url)
		fmt.Println("   Local only — the link works on this machine and nowhere else.")
		fmt.Println("   Press Ctrl+C to stop.")

		if !webNoOpen {
			openBrowser(url)
		}

		serveErr := make(chan error, 1)
		go func() {
			if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- err
			}
		}()

		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		select {
		case err := <-serveErr:
			return err
		case <-stop:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
			fmt.Println("\nenvault web stopped")
			return nil
		}
	},
}

func listenLocal(port int, mayRoll bool) (net.Listener, error) {
	attempts := 1
	if mayRoll {
		attempts = 10
	}
	var lastErr error
	for i := range attempts {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port+i))
		if err == nil {
			return ln, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("cannot listen on 127.0.0.1:%d — %w (try --port)", port, lastErr)
}

// openBrowser is best-effort: the URL is printed either way.
func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "linux":
		c = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = c.Start()
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().IntVarP(&webPort, "port", "p", defaultWebPort, "Port to serve on")
	webCmd.Flags().BoolVar(&webNoOpen, "no-open", false, "Print the URL without opening a browser")
}
