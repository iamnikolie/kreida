package cmd

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/iamnikolie/kreida/internal/server"
	"github.com/spf13/cobra"
)

func newUpCmd(a *App) *cobra.Command {
	var lan bool

	c := &cobra.Command{
		Use: "up [--lan]", Short: "Start the UI server in the background", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			if err := rt.control(a.Exe).Up(&lan); err != nil {
				return err
			}

			fmt.Fprintln(a.Out, rt.url())

			if lan {
				var urls []string
				for _, ip := range lanIPs() {
					urls = append(urls, fmt.Sprintf("http://%s:%d", ip, rt.port))
				}

				fmt.Fprintf(a.Out, "LAN: share links only — %s\n", strings.Join(urls, "  "))
			}

			return nil
		},
	}

	c.Flags().BoolVar(&lan, "lan", false, "share links reachable from other machines on the network")

	return c
}

func newDownCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "down", Short: "Stop the server", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			if rt.control(a.Exe).Stop() {
				fmt.Fprintln(a.Out, "stopped")
			} else {
				fmt.Fprintln(a.Out, "not running")
			}

			return nil
		},
	}
}

func newOpenCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "open [name]", Short: "Open a board in a browser", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			if err := rt.control(a.Exe).Up(nil); err != nil {
				return err
			}

			name := arg(args, 0)
			if name == "" {
				name = newestBoard(rt)
			}

			target := rt.url()

			if name != "" {
				if _, err := rt.need(name); err != nil {
					return err
				}

				target = rt.boardURL(name, "")
			}

			openURL(a, target)

			return nil
		},
	}
}

// newestBoard is the most recently changed board, or "".
func newestBoard(rt session) string {
	entries, err := os.ReadDir(rt.paths.Dir)
	if err != nil {
		return ""
	}

	type item struct {
		name string
		at   time.Time
	}

	var items []item

	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok {
			continue
		}

		if info, err := e.Info(); err == nil {
			items = append(items, item{name, info.ModTime()})
		}
	}

	if len(items) == 0 {
		return ""
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })

	return items[0].name
}

// openURL shows a page in oko's Chrome and falls back to the system's default browser.
func openURL(a *App, target string) {
	c := exec.Command("oko", "open", target)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, a.Out, a.Err

	if err := c.Run(); err == nil {
		return
	}

	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}

	_ = exec.Command(opener, target).Run()
}

// newServeCmd is the hidden command `up` starts in the background: it runs the server in the foreground.
func newServeCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "serve", Short: "Run the server in the foreground (started by `kreida up`)", Hidden: true, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			srv, err := server.New(server.Config{Paths: rt.paths, Port: rt.port, LAN: rt.lan, Dist: a.Dist,
				Log: slog.New(slog.NewTextHandler(a.Err, nil))})
			if err != nil {
				return err
			}

			host := "127.0.0.1"
			if rt.lan {
				host = "0.0.0.0"
			}

			ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, rt.port))
			if err != nil {
				srv.Close()

				return err
			}

			lanNote := ""
			if rt.lan {
				lanNote = " (LAN: share links only)"
			}

			fmt.Fprintf(a.Out, "kreida on :%d%s  boards: %s\n", rt.port, lanNote, rt.paths.Dir)

			return srv.Serve(cmd.Context(), ln)
		},
	}
}
