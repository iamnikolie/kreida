// Package cmd implements the kreida command line: live boards from the shell, YAML in, canvas out.
package cmd

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/daemon"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/web"
	"github.com/spf13/cobra"
)

var version = "dev"

// App holds what commands read from and write to, so tests can run the command tree against a scratch directory
// and captured output.
type App struct {
	Out, Err io.Writer
	In       io.Reader
	// Dist is the built page (index.html at its root); the embedded one unless a test sets another.
	Dist fs.FS
	// Exe is the binary `up` starts as the server; empty means the running executable.
	Exe string
	// Wait sleeps between the polls of a device login; nil means a real timer.
	Wait cloud.Waiter
	// Thumb draws the thumbnail of a published board; nil means headless Chrome showing Dist.
	Thumb cloud.Renderer
}

// NewApp returns an App wired to the process: standard streams and the embedded page.
func NewApp() *App {
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		dist = nil
	}

	return &App{Out: os.Stdout, Err: os.Stderr, In: os.Stdin, Dist: dist}
}

// thumbs is the renderer of thumbnails: the one the App was given, else headless Chrome showing the page.
func (a *App) thumbs(rt session) cloud.Renderer {
	if a.Thumb != nil {
		return a.Thumb
	}

	return cloud.ChromeThumbs(rt.paths, a.Dist)
}

// session is the environment of one run: where boards live, which port the server uses.
type session struct {
	paths paths.Paths
	port  int
	lan   bool
}

func (rt session) url() string { return fmt.Sprintf("http://127.0.0.1:%d", rt.port) }

func (rt session) control(exe string) daemon.Control {
	return daemon.Control{Paths: rt.paths, Port: rt.port, Exe: exe}
}

// env reads the environment (KREIDA_HOME, KREIDA_DIR, KREIDA_PORT, KREIDA_LAN) and makes sure the boards directory
// exists.
func (a *App) env() (session, error) {
	rt, err := a.peek()
	if err != nil {
		return rt, err
	}

	return rt, rt.paths.Ensure()
}

// peek is env without creating anything.
func (a *App) peek() (session, error) {
	p, err := paths.FromEnv()
	if err != nil {
		return session{}, err
	}

	port, err := paths.Port()
	if err != nil {
		return session{}, err
	}

	return session{paths: p, port: port, lan: paths.LAN()}, nil
}

// NewRoot builds the command tree.
func NewRoot(a *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "kreida",
		Short:         "Live boards from the shell (YAML → canvas)",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %s (kreida help)", args[0])
			}

			return cmd.Help()
		},
	}

	root.SetOut(a.Out)
	root.SetErr(a.Err)
	root.CompletionOptions.DisableDefaultCmd = true

	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c != root {
			defaultHelp(c, args)

			return
		}

		fmt.Fprintln(a.Out, a.helpText())
	})

	for _, add := range []func(*App) *cobra.Command{
		newUpCmd, newDownCmd, newOpenCmd, newLsCmd, newNewCmd, newTemplatesCmd, newSetCmd, newCatCmd, newPathCmd,
		newUndoCmd, newHistoryCmd, newExportCmd, newImportCmd, newPngCmd, newResetCmd, newRmCmd, newURLCmd,
		newShareCmd, newUnshareCmd, newSharesCmd, newLiveCmd, newPublishCmd, newUnpublishCmd, newPublishedCmd, newCloudCmd,
		newInboxCmd, newResolveCmd, newSkillCmd, newServeCmd,
	} {
		root.AddCommand(add(a))
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the installed version",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Fprintln(a.Out, "kreida version "+version)
		},
	})

	return root
}

// Execute runs the command line and exits 1 with `kreida: <message>` on an error.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := NewRoot(NewApp()).ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "kreida: %s\n", err)
		os.Exit(1)
	}
}
