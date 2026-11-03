package cmd

import "fmt"

// helpText is what `kreida`, `kreida help` and `kreida --help` print.
func (a *App) helpText() string {
	url, dir := "http://127.0.0.1:7788", "~/.kreida/boards"

	if rt, err := a.peek(); err == nil {
		url, dir = rt.url(), rt.paths.Dir
	}

	return fmt.Sprintf(`kreida — live boards from the shell (YAML → canvas)

  kreida up [--lan]             start the UI server (%s) in background; --lan: share links reachable on the LAN
  kreida open [name]            open a board in a browser
  kreida ls                     list boards
  kreida new <name> [title]     create a board from a template  [--template flow|lanes|sequence|mindmap|matrix|compare|schema|before-after]
  kreida templates              list templates
  kreida set <name> [file|-]    replace board YAML (validated); stdin when no file
  kreida cat <name>             print board YAML
  kreida path <name>            print board file path (edit it directly — UI updates live)
  kreida undo <name>            step back one version (every saved version is kept)
  kreida history <name>         list saved versions
  kreida export <name> [-o f]   standalone read-only HTML (send it, or host it anywhere)
  kreida export <name> --mermaid [-o f]   Mermaid flowchart (stdout without -o)
  kreida import <name> [file|-] Mermaid flowchart/graph or sequenceDiagram -> board [--force replaces]
  kreida png <name> [-o file]   render to PNG (headless Chrome)  [--theme light|dark] [--step N]
  kreida reset <name>           drop manual node positions (back to auto layout)
  kreida rm <name>              delete a board
  kreida url <name>             print board URL
  kreida share <name> [--lan]   read-only live link for a client (--lan: reachable from other machines on the network)
  kreida unshare <name>         revoke the board's share link
  kreida shares                 list share links
  kreida live <name>            list live-card sources (cmd/url) and whether each is allowed
  kreida live <name> --allow [node]   allow the board's sources to run (prints each one); --revoke; --run (values now)
  kreida inbox [name] [--all] [--json]   notes the human left for the agent (open ones)
  kreida resolve <name> <id>...   close notes once you have acted on them
  kreida down                   stop the server
  kreida version                print the installed version
  kreida skill                  agent reference (format, kinds, examples)

boards: %s`, url, dir)
}
