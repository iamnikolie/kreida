package cmd

import (
	"fmt"

	"github.com/iamnikolie/kreida/internal/history"
	"github.com/spf13/cobra"
)

func newUndoCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "undo <name>", Short: "Step back one version (every saved version is kept)", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			if err := (history.Store{Paths: rt.paths}).Undo(name); err != nil {
				return err
			}

			fmt.Fprintln(a.Out, rt.boardURL(name, ""))

			return nil
		},
	}
}

func newHistoryCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "history <name>", Short: "List saved versions", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			versions, err := (history.Store{Paths: rt.paths}).List(name)
			if err != nil {
				return err
			}

			for _, v := range versions {
				fmt.Fprintf(a.Out, "%s  %s\n", v.ID, v.Time)
			}

			return nil
		},
	}
}
