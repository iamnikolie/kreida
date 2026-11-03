package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/mermaid"
	"github.com/iamnikolie/kreida/internal/render"
	"github.com/spf13/cobra"
)

func newExportCmd(a *App) *cobra.Command {
	var out string

	var asMermaid bool

	c := &cobra.Command{
		Use: "export <name> [-o file]", Short: "Standalone read-only HTML, or a Mermaid flowchart with --mermaid", Args: cobra.ArbitraryArgs,
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

			text, err := os.ReadFile(f)
			if err != nil {
				return err
			}

			b, err := board.Parse(string(text))
			if err != nil {
				return err
			}

			if asMermaid {
				m, err := mermaid.ToMermaid(b)
				if err != nil {
					return err
				}

				if out == "" {
					_, err = fmt.Fprint(a.Out, m)

					return err
				}

				return writeOut(a, out, []byte(m))
			}

			pos := json.RawMessage("{}")
			if data, err := os.ReadFile(rt.paths.PosFile(name)); err == nil && json.Valid(data) {
				pos = data
			}

			html, err := render.HTML(a.Dist, render.Data{Name: name, Board: b, Pos: pos})
			if err != nil {
				return err
			}

			if out == "" {
				out = name + ".html"
			}

			return writeOut(a, out, []byte(html))
		},
	}

	c.Flags().StringVarP(&out, "output", "o", "", "write to this file (default <name>.html; stdout for --mermaid)")
	c.Flags().BoolVar(&asMermaid, "mermaid", false, "export a Mermaid flowchart instead of HTML")

	return c
}

// writeOut writes a file and prints its absolute path.
func writeOut(a *App, file string, data []byte) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}

	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return err
	}

	fmt.Fprintln(a.Out, abs)

	return nil
}

func newImportCmd(a *App) *cobra.Command {
	var force bool

	c := &cobra.Command{
		Use: "import <name> [file|-]", Short: "Mermaid flowchart/graph or sequenceDiagram -> board", Args: cobra.ArbitraryArgs,
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

			if _, err := os.Stat(f); err == nil && !force {
				return fmt.Errorf("board %s exists (--force to replace; kreida undo restores)", name)
			}

			text, err := a.readSource(arg(args, 1))
			if err != nil {
				return err
			}

			yaml, err := mermaid.FromMermaid(text)
			if err != nil {
				return err
			}

			if err := rt.write(name, yaml); err != nil {
				return err
			}

			fmt.Fprintln(a.Out, rt.boardURL(name, ""))

			return nil
		},
	}

	c.Flags().BoolVar(&force, "force", false, "replace an existing board")

	return c
}
