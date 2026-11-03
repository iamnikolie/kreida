package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/iamnikolie/kreida/internal/live"
	"github.com/spf13/cobra"
)

func newLiveCmd(a *App) *cobra.Command {
	var allow, revoke, run bool

	c := &cobra.Command{
		Use:   "live <name> [node]",
		Short: "List live-card sources and whether each is allowed; --allow [node], --revoke, --run",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			r := live.New(rt.paths)

			switch {
			case allow:
				added, err := r.Allow(name, arg(args, 1))
				if err != nil {
					return err
				}

				if len(added) == 0 {
					fmt.Fprintln(a.Out, "nothing new to allow")
				}

				for _, s := range added {
					fmt.Fprintf(a.Out, "allowed  %s: %s\n", s.Node, s.Source)
				}
			case revoke:
				n, err := r.Revoke(name)
				if err != nil {
					return err
				}

				fmt.Fprintf(a.Out, "revoked %d\n", n)
			case run:
				vs, err := r.Values(name)
				if err != nil {
					return err
				}

				ids := make([]string, 0, len(vs))
				for id := range vs {
					ids = append(ids, id)
				}

				sort.Strings(ids)

				for _, id := range ids {
					fmt.Fprintf(a.Out, "%-14s %s\n", id, showValue(vs[id]))
				}
			default:
				srcs, err := r.Sources(name)
				if err != nil {
					return err
				}

				if len(srcs) == 0 {
					fmt.Fprintln(a.Out, "no live cards")
				}

				for _, s := range srcs {
					state := "BLOCKED"
					if s.Allowed {
						state = "allowed"
					}

					src := s.Source
					for _, p := range []string{"cmd:", "url:"} {
						if rest, ok := strings.CutPrefix(src, p); ok {
							src = p[:3] + "  " + rest

							break
						}
					}

					fmt.Fprintf(a.Out, "%s  %-14s every %3ss  %s\n", state, s.Node,
						strconv.FormatFloat(s.Every, 'f', -1, 64), src)
				}
			}

			return nil
		},
	}

	c.Flags().BoolVar(&allow, "allow", false, "allow the board's sources (or only node's) to run; prints each one")
	c.Flags().BoolVar(&revoke, "revoke", false, "drop the board's allowed sources")
	c.Flags().BoolVar(&run, "run", false, "run the allowed sources now and print the values")

	return c
}

func showValue(v live.Value) string {
	switch {
	case v.Blocked:
		return "BLOCKED"
	case v.Error != nil && *v.Error != "":
		return "error: " + *v.Error
	case v.Value != nil:
		return *v.Value
	}

	return ""
}
