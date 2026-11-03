package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed skill.md
var skillDoc string

func newSkillCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "skill", Short: "Agent reference (format, kinds, examples)", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(a.Out, skillDoc)

			return err
		},
	}
}
