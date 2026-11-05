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

## Images on cards

A card can show a local picture:

```yaml
nodes:
  shot: { title: Order screen, image: img/order.png, width: 420 }
```

It appears on the canvas, in `kreida png`, in `kreida export`, and on share
links. A relative path is resolved against the directory of the board file
(`kreida path <name>`), so a board and its images can move together.

PNG, JPEG, GIF, and WebP are accepted, judged by content rather than by file
name. SVG is refused. A file must be a regular file of at most 5 MiB and 40
megapixels, and a board holds at most 25 MiB of images. A symlink is followed
and its target is checked the same way.

A missing or refused file does not fail the board. The command names the card
and the path on stderr. Your local editor and `kreida png` show why the
picture is absent, while exports and share links show the card without it.

A path never leaves your machine. Exports, share links, and published boards
carry images re-encoded from their decoded pixels, so no byte of the original
file goes out: no EXIF (including GPS), XMP, IPTC, comments, PNG text chunks,
or unused data. JPEG is re-encoded at quality 92. PNG and GIF are re-encoded
losslessly, and an animated PNG becomes its first frame. WebP becomes PNG when
it is lossless or has alpha, and JPEG otherwise. Palettes keep only the colours
in use. No byte of a colour profile goes out: an sRGB, Display P3, or Adobe RGB
profile, recognised by its colour data, is replaced with kreida's own copy, and
any other profile is dropped with a warning that colours may shift. A photo
rotated by its EXIF orientation is rotated in its pixels first, so it keeps the
same appearance.

A file that cannot be read or does not decode, and an animated WebP, are
refused rather than sent unchanged. Your local editor and `kreida png` show the
original file.

A board can name any image you can read, so check its paths before you send an
export or share a link.
