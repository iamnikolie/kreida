package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/history"
	"github.com/iamnikolie/kreida/internal/images"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/internal/templates"
	"github.com/spf13/cobra"
)

// arg returns the i-th positional argument, or "" when there is none.
func arg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}

	return ""
}

// warnImages tells the owner which cards have no picture and why. It names the paths, which is why it only ever goes
// to the terminal.
func warnImages(a *App, name string, problems []images.Problem) {
	for _, p := range problems {
		fmt.Fprintf(a.Err, "kreida: %s: %s\n", name, p)
	}
}

// warnBoardImages is warnImages for the board in its file as it is now. A board that cannot be read has other news.
func warnBoardImages(a *App, rt session, name string) {
	text, err := os.ReadFile(rt.paths.BoardFile(name))
	if err != nil {
		return
	}

	b, err := board.Parse(string(text))
	if err != nil {
		return
	}

	set := images.Attach(b, rt.paths.Dir)
	warnImages(a, name, slices.Concat(set.Problems, set.Notes))
}

// boardFile checks a board name and returns the path of its file.
func (rt session) boardFile(name string) (string, error) {
	if !paths.ValidName(name) {
		if name == "" {
			name = "(none)"
		}

		return "", fmt.Errorf("bad board name: %s", name)
	}

	return rt.paths.BoardFile(name), nil
}

// need is boardFile for a board that must exist.
func (rt session) need(name string) (string, error) {
	f, err := rt.boardFile(name)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(f); err != nil {
		return "", fmt.Errorf("no board %s (kreida ls)", name)
	}

	return f, nil
}

// boardURL is the address of a board in the page.
func (rt session) boardURL(name, extra string) string {
	return fmt.Sprintf("%s/?b=%s%s", rt.url(), url.QueryEscape(name), extra)
}

// write validates board text and saves it as the board's file. The text before and after goes into the history (the
// server does the same when it runs), which also drops the dragged positions the change displaces.
func (rt session) write(name, text string) error {
	if _, err := board.Parse(text); err != nil {
		return fmt.Errorf("invalid board: %s", err)
	}

	f, err := rt.boardFile(name)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	hist := history.Store{Paths: rt.paths}
	if _, err := hist.Snapshot(name); err != nil {
		return err
	}

	if err := os.WriteFile(f, []byte(text), 0o644); err != nil {
		return err
	}

	_, err = hist.Snapshot(name)

	return err
}

// readSource reads the file named by src, or stdin when src is empty or "-".
func (a *App) readSource(src string) (string, error) {
	var b []byte

	var err error

	if src == "" || src == "-" {
		b, err = io.ReadAll(a.In)
	} else {
		b, err = os.ReadFile(src)
	}

	return string(b), err
}

func newLsCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "ls", Short: "List boards", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			entries, err := os.ReadDir(rt.paths.Dir)
			if err != nil {
				return err
			}

			for _, e := range entries {
				name, ok := strings.CutSuffix(e.Name(), ".yaml")
				if !ok {
					continue
				}

				title := "(invalid)"

				if text, err := os.ReadFile(rt.paths.BoardFile(name)); err == nil {
					if b, err := board.Parse(string(text)); err == nil {
						title = b.Title
					}
				}

				fmt.Fprintf(a.Out, "%-24s %s\n", name, title)
			}

			return nil
		},
	}
}

func newNewCmd(a *App) *cobra.Command {
	var tpl string

	c := &cobra.Command{
		Use: "new <name> [title]", Short: "Create a board from a template", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)

			f, err := rt.boardFile(name)
			if err != nil {
				return err
			}

			t, ok := templates.Get(tpl)
			if !ok {
				return fmt.Errorf("no template %s (kreida templates)", tpl)
			}

			if _, err := os.Stat(f); err == nil {
				return fmt.Errorf("board %s exists", name)
			}

			title := name
			if len(args) > 1 {
				if joined := strings.Join(args[1:], " "); joined != "" {
					title = joined
				}
			}

			if err := rt.write(name, t.YAML(title)); err != nil {
				return err
			}

			fmt.Fprintln(a.Out, f)

			return nil
		},
	}

	c.Flags().StringVar(&tpl, "template", "flow", "flow|lanes|sequence|mindmap|matrix|compare|schema|before-after")

	return c
}

func newTemplatesCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "templates", Short: "List templates", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, t := range templates.All() {
				fmt.Fprintf(a.Out, "%-14s %s\n", t.Name, t.About)
			}

			return nil
		},
	}
}

func newSetCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "set <name> [file|-]", Short: "Replace board YAML (validated); stdin when no file", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.boardFile(name); err != nil {
				return err
			}

			text, err := a.readSource(arg(args, 1))
			if err != nil {
				return err
			}

			if err := rt.write(name, text); err != nil {
				return err
			}

			warnBoardImages(a, rt, name)
			fmt.Fprintln(a.Out, rt.boardURL(name, ""))

			return nil
		},
	}
}

func newCatCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "cat <name>", Short: "Print board YAML", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			f, err := rt.need(arg(args, 0))
			if err != nil {
				return err
			}

			text, err := os.ReadFile(f)
			if err != nil {
				return err
			}

			_, err = a.Out.Write(text)

			return err
		},
	}
}

func newPathCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "path <name>", Short: "Print board file path (edit it directly: the page updates live)", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			f, err := rt.boardFile(arg(args, 0))
			if err != nil {
				return err
			}

			fmt.Fprintln(a.Out, f)

			return nil
		},
	}
}

func newURLCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "url <name>", Short: "Print board URL", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			fmt.Fprintln(a.Out, rt.boardURL(arg(args, 0), ""))

			return nil
		},
	}
}

func newRmCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "rm <name>", Short: "Delete a board", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)

			f, err := rt.need(name)
			if err != nil {
				return err
			}

			if err := os.Remove(f); err != nil {
				return err
			}

			if err := removeIfThere(rt.paths.PosFile(name)); err != nil {
				return err
			}

			return removeIfThere(rt.paths.NotesFile(name))
		},
	}
}

func newResetCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "reset <name>", Short: "Drop manual node positions (back to auto layout)", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			return removeIfThere(rt.paths.PosFile(name))
		},
	}
}

func removeIfThere(file string) error {
	if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
