package cmd

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/iamnikolie/kreida/internal/render"
	"github.com/spf13/cobra"
)

func newPngCmd(a *App) *cobra.Command {
	var out, theme, step string

	c := &cobra.Command{
		Use: "png <name> [-o file]", Short: "Render a board to PNG (headless Chrome)", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			if err := rt.control(a.Exe).Up(nil); err != nil {
				return err
			}

			if out == "" {
				out = name + ".png"
			}

			abs, err := filepath.Abs(out)
			if err != nil {
				return err
			}

			chrome, err := render.FindChrome()
			if err != nil {
				return err
			}

			extra := "&present=1&static=1&theme=" + url.QueryEscape(theme)
			if step != "" {
				extra += "&step=" + url.QueryEscape(step)
			}

			if err := render.PNG(chrome, rt.boardURL(name, extra), abs); err != nil {
				return err
			}

			fmt.Fprintln(a.Out, abs)

			return nil
		},
	}

	c.Flags().StringVarP(&out, "output", "o", "", "write the PNG here (default <name>.png)")
	c.Flags().StringVar(&theme, "theme", "light", "light|dark")
	c.Flags().StringVar(&step, "step", "", "render presentation step N")

	return c
}
