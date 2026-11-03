package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/notes"
	"github.com/spf13/cobra"
)

func newInboxCmd(a *App) *cobra.Command {
	var all, asJSON bool

	c := &cobra.Command{
		Use: "inbox [name] [--all] [--json]", Short: "Notes the human left for the agent: open ones",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)

			var boards []string

			if name != "" {
				if _, err := rt.need(name); err != nil {
					return err
				}

				boards = []string{name}
			} else if boards, err = notes.Boards(rt.paths); err != nil {
				return err
			}

			list := []notes.Note{}

			for _, board := range boards {
				local, err := notes.List(rt.paths, board, all)
				if err != nil {
					return err
				}

				for _, n := range local {
					n.Board = board
					list = append(list, n)
				}
			}

			// the cloud is a bonus: when it fails the local notes are still the answer
			comments, err := cloudComments(cmd.Context(), rt, name, all)
			if err != nil {
				fmt.Fprintf(a.Err, "kreida: cloud comments: %s\n", hint(err))
			}

			list = append(list, comments...)

			sort.SliceStable(list, func(i, j int) bool { return created(list[i]).Before(created(list[j])) })

			if asJSON {
				enc := json.NewEncoder(a.Out)
				enc.SetEscapeHTML(false)
				enc.SetIndent("", "  ")

				return enc.Encode(list)
			}

			if len(list) == 0 {
				if all {
					fmt.Fprintln(a.Out, "no notes")
				} else {
					fmt.Fprintln(a.Out, "no open notes")
				}

				return nil
			}

			for _, n := range list {
				printNote(a, n)
			}

			return nil
		},
	}

	c.Flags().BoolVar(&all, "all", false, "include the notes already resolved")
	c.Flags().BoolVar(&asJSON, "json", false, "print a JSON array of notes")

	return c
}

// cloudComments asks the cloud for the comments clients left: on one board when name is set, on all the owner's boards
// otherwise. Not logged in is no comments. A board that is not published has none either.
func cloudComments(ctx context.Context, rt session, name string, all bool) ([]notes.Note, error) {
	cfg, err := cloud.Load(rt.paths)
	if err != nil || !cfg.LoggedIn() {
		return nil, err
	}

	cl, err := cloud.ClientFor(cfg)
	if err != nil {
		return nil, err
	}

	if name == "" {
		return cl.AllComments(ctx, all)
	}

	comments, err := cl.Comments(ctx, name, all)
	if cloud.IsStatus(err, 404) {
		return nil, nil
	}

	for i := range comments {
		comments[i].Board = name
	}

	return comments, err
}

// created is when a note was written; a time that does not read sorts first.
func created(n notes.Note) time.Time {
	t, _ := time.Parse(time.RFC3339, n.Created)

	return t
}

// oneLine is a stranger's word for a header: no control characters, no line break.
func oneLine(s string) string { return strings.Join(strings.Fields(notes.Plain(s)), " ") }

// printNote writes one note as a block: a header line, the text indented by two spaces, a blank line. The text is
// printed as it is and every line of it is indented, so no text can pass for a header of its own.
func printNote(a *App, n notes.Note) {
	node, author := oneLine(n.Node), oneLine(n.Author)
	if node == "" {
		node = "board"
	}

	if author == "" {
		author = "you"
	}

	header := fmt.Sprintf("%s %s on %s · %s · %s", oneLine(n.Board), oneLine(n.ID), node, author, stamp(n.Created))
	if n.Resolved != "" {
		header += " · resolved " + stamp(n.Resolved)
	}

	fmt.Fprintln(a.Out, header)

	for _, line := range strings.Split(strings.TrimSpace(notes.Plain(n.Text)), "\n") {
		fmt.Fprintln(a.Out, "  "+line)
	}

	fmt.Fprintln(a.Out)
}

// stamp shows an RFC 3339 time in local time, to the minute; what does not read as a time is shown as it came.
func stamp(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return oneLine(s)
	}

	return t.Local().Format("2006-01-02 15:04")
}

func newResolveCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use: "resolve <name> <id>...", Short: "Close notes once you have acted on them (n… local, c_… comments from your configured server)",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.env()
			if err != nil {
				return err
			}

			name := arg(args, 0)
			if _, err := rt.need(name); err != nil {
				return err
			}

			ids := args[1:]
			if len(ids) == 0 {
				return fmt.Errorf("no note id (kreida inbox %s)", name)
			}

			// no login, no change at all: not half of the ids closed
			var cl *cloud.Client

			if slices.ContainsFunc(ids, isComment) {
				cfg, err := cloud.Load(rt.paths)
				if err != nil {
					return err
				}

				if !cfg.LoggedIn() {
					return cloud.ErrNotLoggedIn
				}

				if cl, err = cloud.ClientFor(cfg); err != nil {
					return err
				}
			}

			now := time.Now()

			for _, id := range ids {
				if isComment(id) {
					if err := cl.ResolveComment(cmd.Context(), name, id); err != nil {
						return hint(err)
					}
				} else if err := notes.Resolve(rt.paths, name, id, now); err != nil {
					return err
				}

				fmt.Fprintf(a.Out, "resolved %s\n", id)
			}

			return nil
		},
	}
}

// isComment tells a comment on kreida cloud (c_…) from a note on this machine (n…).
func isComment(id string) bool { return strings.HasPrefix(id, "c_") }
