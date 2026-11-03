# kreida

Turn YAML into interactive boards for flows, diagrams, and presentations.
The local browser view updates as you edit the board.

## Build

Install Go 1.24 or later, Node.js 24, and pnpm 10, then run `make build`. The
build embeds the web assets in the `kreida` binary. Release archives include
the binary and sample boards. PNG export uses a local Chromium-based browser.

## Try a board

```sh
./kreida set demo < demo/landing-en.yaml
./kreida open demo
./kreida export demo -o demo.html
```

Run `./kreida skill` for the YAML reference and `./kreida help` for commands.
