# kreida

Turn YAML into interactive boards for flows, diagrams, and presentations.
The local browser view updates as you edit the board.

## Install

Download the archive for your system from the
[latest release](https://github.com/iamnikolie/kreida/releases/latest).
Archives are built for macOS, Linux, and FreeBSD on amd64 and arm64, named
`kreida_<version>_<os>_<arch>.tar.gz`, and listed in `checksums.txt`. Each
holds the `kreida` binary, this README, the license, and the sample boards.

```sh
tar xzf kreida_*_darwin_arm64.tar.gz
./kreida version
```

Move `kreida` to a directory on your `PATH`. On macOS, a binary downloaded
through a browser may need `xattr -d com.apple.quarantine kreida` before the
first run.

## Build

Install Go 1.24 or later, Node.js 24, and pnpm 10, then run `make build`. The
build embeds the web assets in the `kreida` binary.

PNG export uses a local Chromium-based browser.

## Try a board

```sh
./kreida set demo < demo/landing-en.yaml
./kreida open demo
./kreida export demo -o demo.html
```

Run `./kreida skill` for the YAML reference and `./kreida help` for commands.
