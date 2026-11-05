package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/images"
	"github.com/iamnikolie/kreida/internal/live"
	"github.com/spf13/cobra"
)

// hint adds the way out to a refused token.
func hint(err error) error {
	if cloud.IsStatus(err, 401) {
		return fmt.Errorf("%w (sign in again to your configured server)", err)
	}

	return err
}

func newCloudCmd(a *App) *cobra.Command {
	c := &cobra.Command{
		Use: "cloud", Hidden: true, Short: "Manage access to your own compatible server", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command cloud %s (kreida help)", args[0])
			}

			return cmd.Help()
		},
	}

	c.AddCommand(newCloudLoginCmd(a), newCloudLogoutCmd(a), newCloudStatusCmd(a))

	return c
}

func newCloudLoginCmd(a *App) *cobra.Command {
	var token, url string

	c := &cobra.Command{
		Use: "login [--token kc_…] [--url …]", Short: "Log in to your own compatible server",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.peek()
			if err != nil {
				return err
			}

			// a login file that cannot be read is replaced by this login
			prev, _ := cloud.Load(rt.paths)

			base := prev.URL
			if override := os.Getenv(cloud.EnvURL); override != "" {
				base = override
			}
			if url != "" {
				base = url
			}

			if strings.TrimSpace(base) == "" {
				return fmt.Errorf("cloud endpoint required (set --url or $%s)", cloud.EnvURL)
			}

			if base, err = cloud.NormalizeURL(base); err != nil {
				return err
			}

			cfg := cloud.Config{URL: base}

			if token != "" {
				if !strings.HasPrefix(token, "kc_") {
					return fmt.Errorf("a cloud token starts with kc_")
				}

				cfg.Token = token
			} else {
				tok, err := deviceLogin(cmd.Context(), a, base)
				if err != nil {
					return err
				}

				cfg.Token = tok
			}

			cl, err := cloud.NewClient(base, cfg.Token)
			if err != nil {
				return err
			}

			who, err := cl.Whoami(cmd.Context())
			if err != nil {
				if token != "" {
					return fmt.Errorf("token not accepted: %w", err)
				}

				// the login worked, only the name of the owner is missing: keep the token
				fmt.Fprintf(a.Err, "kreida: logged in, but who am I failed: %s\n", err)
			}

			cfg.Email = who.Email
			cfg.Published = keepPublished(prev, cfg)

			if err := cfg.Save(rt.paths); err != nil {
				return err
			}

			fmt.Fprintf(a.Out, "logged in as %s (%s)\n", cfg.Email, base)

			return nil
		},
	}

	c.Flags().StringVar(&token, "token", "", "use a token issued by your server instead of device login")
	c.Flags().StringVar(&url, "url", "", "your server URL (required on first login unless $"+cloud.EnvURL+" is set)")

	return c
}

// keepPublished is the published list that survives a new login: the same owner on the same cloud keeps it, another
// one starts empty (those boards are not in that account).
func keepPublished(prev, next cloud.Config) []string {
	if prev.Token != "" && next.Email != "" && prev.Email == next.Email && strings.TrimRight(prev.URL, "/") == next.URL {
		return prev.Published
	}

	return nil
}

// deviceLogin shows the owner the page and the code to approve and waits for the token.
func deviceLogin(ctx context.Context, a *App, base string) (string, error) {
	cl, err := cloud.NewClient(base, "")
	if err != nil {
		return "", err
	}

	host, _ := os.Hostname()
	if host == "" {
		host = "kreida"
	}

	tok, err := cl.Login(ctx, host, func(d cloud.Device) {
		fmt.Fprintf(a.Out, "open  %s\ncode  %s\n", d.VerificationURL, d.UserCode)
		fmt.Fprintf(a.Err, "waiting for the approval in the browser (the code is good for %d min); Ctrl-C to cancel\n", max(d.ExpiresIn/60, 1))
	}, a.Wait)
	if err != nil {
		return "", err
	}

	return tok.Token, nil
}

func newCloudLogoutCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "logout", Short: "Forget your server login on this machine", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.peek()
			if err != nil {
				return err
			}

			had, err := cloud.Remove(rt.paths)
			if err != nil {
				return err
			}

			if had {
				fmt.Fprintln(a.Out, "logged out (boards stay published; revoke the token in the cloud account to cut it off for good)")
			} else {
				fmt.Fprintln(a.Out, "not logged in")
			}

			return nil
		},
	}
}

func newCloudStatusCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "status", Short: "Show your server login (never the token)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.peek()
			if err != nil {
				return err
			}

			cfg, err := cloud.Load(rt.paths)
			if err != nil {
				return err
			}

			if !cfg.LoggedIn() {
				fmt.Fprintln(a.Out, "not logged in")

				return nil
			}

			fmt.Fprintf(a.Out, "cloud      %s\nuser       %s\npublished  %s\n", cfg.URL, cfg.Email, listOrNone(cfg.Published))

			return nil
		},
	}
}

func listOrNone(l []string) string {
	if len(l) == 0 {
		return "(none)"
	}

	return strings.Join(l, " ")
}

// linkLine is a link's address with what the visitor needs to know: a passcode, an end date.
func linkLine(l cloud.Link) string {
	var notes []string

	if l.Passcode {
		notes = append(notes, "passcode")
	}

	if l.ExpiresAt != nil && *l.ExpiresAt != "" {
		notes = append(notes, "expires "+*l.ExpiresAt)
	}

	if len(notes) == 0 {
		return l.URL
	}

	return l.URL + " (" + strings.Join(notes, ", ") + ")"
}

func newPublishCmd(a *App) *cobra.Command {
	var watch bool

	c := &cobra.Command{
		Use: "publish <name> [--watch]", Hidden: true, Short: "Upload a board to your own configured compatible server", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			log := slog.New(slog.DiscardHandler)
			if watch {
				log = slog.New(slog.NewTextHandler(a.Err, nil))
			}

			sy := cloud.NewSyncer(rt.paths, live.New(rt.paths), log)
			sy.Only = name
			sy.Thumb = a.thumbs(rt)

			defer sy.Close()

			pub, err := sy.Publish(cmd.Context(), name)
			if err != nil {
				return hint(err)
			}

			warnBoardImages(a, rt, name)

			if len(pub.Links) == 0 {
				fmt.Fprintln(a.Err, "kreida: published, but the board has no link yet (make one in the cloud account)")
			}

			for _, l := range pub.Links {
				fmt.Fprintln(a.Out, linkLine(l))
			}

			// the values are on the page from the first look
			if err := sy.PushLive(cmd.Context(), name); err != nil {
				fmt.Fprintf(a.Err, "kreida: live values not pushed: %s\n", err)
			}

			// the preview on the owner's page is a nicety: no Chrome is a note, never a failed publish
			if err := sy.PushThumb(cmd.Context(), name); err != nil {
				fmt.Fprintf(a.Err, "kreida: thumbnail not uploaded: %s\n", err)
			}

			if !watch {
				return nil
			}

			fmt.Fprintf(a.Err, "watching %s; Ctrl-C to stop\n", name)

			return watchBoard(cmd.Context(), a.Err, rt, sy, name)
		},
	}

	c.Flags().BoolVar(&watch, "watch", false, "stay and push every change of the board and its positions, and its live values")

	return c
}

// watchBoard feeds the syncer with the changes of one board's file, position file and image files until ctx ends.
func watchBoard(ctx context.Context, errw io.Writer, rt session, sy *cloud.Syncer, name string) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	defer w.Close()

	// the directory, not the files: editors replace a file instead of writing into it
	if err := w.Add(rt.paths.Dir); err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	sy.Start()

	// image files may live anywhere: they are looked at, not watched
	tracker := images.NewTracker()
	tracker.Changed(name, rt.paths.BoardFile(name), rt.paths.Dir)

	poll := time.NewTicker(images.PollEvery)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-poll.C:
			if tracker.Changed(name, rt.paths.BoardFile(name), rt.paths.Dir) {
				sy.Changed(name)
			}
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}

			if file := filepath.Base(ev.Name); ev.Op != fsnotify.Chmod && (file == name+".yaml" || file == name+".pos.json") {
				sy.Changed(name)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}

			fmt.Fprintf(errw, "kreida: watch: %s\n", err)
		}
	}
}

func newUnpublishCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "unpublish <name>", Hidden: true, Short: "Remove a board from your own configured compatible server", Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.peek()
			if err != nil {
				return err
			}

			// the board may be gone from this machine and still be up there
			name := arg(args, 0)
			if _, err := rt.boardFile(name); err != nil {
				return err
			}

			if err := cloud.ValidName(name); err != nil {
				return err
			}

			cfg, err := cloud.Load(rt.paths)
			if err != nil {
				return err
			}

			cl, err := loggedIn(cfg)
			if err != nil {
				return err
			}

			switch err := cl.Unpublish(cmd.Context(), name); {
			case err == nil:
				fmt.Fprintln(a.Out, "unpublished")
			case cloud.IsStatus(err, 404):
				fmt.Fprintln(a.Out, "not published")
			default:
				return hint(err)
			}

			// read again: another process may have changed the list meanwhile
			cfg, err = cloud.Load(rt.paths)
			if err != nil {
				return err
			}

			return cfg.Drop(name).Save(rt.paths)
		},
	}
}

func newPublishedCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "published", Hidden: true, Short: "List boards on your own configured compatible server", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.peek()
			if err != nil {
				return err
			}

			cfg, err := cloud.Load(rt.paths)
			if err != nil {
				return err
			}

			cl, err := loggedIn(cfg)
			if err != nil {
				return err
			}

			boards, err := cl.Boards(cmd.Context())
			if err != nil {
				return hint(err)
			}

			if len(boards) == 0 {
				fmt.Fprintln(a.Out, "nothing published")
			}

			for _, b := range boards {
				links := make([]string, 0, len(b.Links))
				for _, l := range b.Links {
					links = append(links, linkLine(l))
				}

				if len(links) == 0 {
					links = append(links, "(no links)")
				}

				fmt.Fprintf(a.Out, "%-24s %s\n", b.Name, strings.Join(links, "  "))
			}

			return nil
		},
	}
}

// loggedIn is the Client of a login, or the way to get one.
func loggedIn(cfg cloud.Config) (*cloud.Client, error) {
	if !cfg.LoggedIn() {
		return nil, cloud.ErrNotLoggedIn
	}

	return cloud.ClientFor(cfg)
}
