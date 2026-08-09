// ABOUTME: 'envault web' — serve the browser UI on localhost for non-terminal users.
// ABOUTME: Prints a tokenized URL and opens it in the default browser.
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

	"github.com/akhshyganesh/envault/internal/web"
	"github.com/spf13/cobra"
)

const defaultWebPort = 7391

var (
	webPort   int
	webNoOpen bool
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Open the envault UI in your browser",
	Long: `Starts a small local web server and opens it in your browser, giving you
everything the terminal commands do — browse backups, view and restore
versions, run a scan, manage watch directories, control the daemon, export
or import a vault, and peek inside a backup zip.

The server binds to 127.0.0.1 only and every request needs the one-time
token in the URL it prints, so nothing on your network can reach it. It
serves your .env contents, so leave it running only while you're using it.

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

		ln, err := listenLocal(webPort, !cmd.Flags().Changed("port"))
		if err != nil {
			return err
		}
		url := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", ln.Addr().(*net.TCPAddr).Port, token)

		httpServer := &http.Server{
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

		errCh := make(chan error, 1)
		go func() {
			if serveErr := httpServer.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				errCh <- serveErr
			}
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case serveErr := <-errCh:
			return serveErr
		case <-sigCh:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(ctx)
			fmt.Println("\nenvault web stopped")
			return nil
		}
	},
}

// listenLocal binds the loopback interface. When the port came from the
// default rather than the user, a busy port rolls forward instead of failing.
func listenLocal(port int, mayRoll bool) (net.Listener, error) {
	attempts := 1
	if mayRoll {
		attempts = 10
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
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
