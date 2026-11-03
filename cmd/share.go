package cmd

import (
	"fmt"
	"net"

	"github.com/iamnikolie/kreida/internal/share"
	"github.com/spf13/cobra"
)

// lanIPs lists this machine's IPv4 addresses on non-loopback interfaces that are up.
func lanIPs() []string {
	var out []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}

		for _, ad := range addrs {
			if ipn, ok := ad.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, ipn.IP.String())
			}
		}
	}

	return out
}

func newShareCmd(a *App) *cobra.Command {
	var lan bool

	c := &cobra.Command{
		Use: "share <name> [--lan]", Short: "Read-only live link for a client", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			token, err := (share.Store{Paths: rt.paths}).Share(name)
			if err != nil {
				return err
			}

			ctl := rt.control(a.Exe)

			var want *bool
			if lan {
				want = &lan
			}

			if err := ctl.Up(want); err != nil {
				return err
			}

			h, _ := ctl.Health()

			fmt.Fprintf(a.Out, "%s/s/%s\n", rt.url(), token)

			if h.LAN {
				for _, ip := range lanIPs() {
					fmt.Fprintf(a.Out, "http://%s:%d/s/%s\n", ip, rt.port, token)
				}
			} else {
				fmt.Fprintln(a.Out, "(this machine only; kreida share <name> --lan to reach it from the local network)")
			}

			return nil
		},
	}

	c.Flags().BoolVar(&lan, "lan", false, "make the link reachable from other machines on the network")

	return c
}

func newUnshareCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "unshare <name>", Short: "Revoke the board's share link", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			n, err := (share.Store{Paths: rt.paths}).Unshare(name)
			if err != nil {
				return err
			}

			if n > 0 {
				fmt.Fprintln(a.Out, "revoked")
			} else {
				fmt.Fprintln(a.Out, "not shared")
			}

			return nil
		},
	}
}

func newSharesCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "shares", Short: "List share links", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			for _, l := range (share.Store{Paths: rt.paths}).All() {
				fmt.Fprintf(a.Out, "%-24s /s/%s  %s\n", l.Board, l.Token, l.Created)
			}

			return nil
		},
	}
}
