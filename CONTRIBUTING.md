# Contributing

Report bugs with your operating system, the output of `kreida version`,
the command you ran, and a minimal board that reproduces the issue. Remove
private data from examples and screenshots before posting.

Install Go 1.24 or later, Node.js 24, and pnpm 10. Run these checks from the
repository root:

```sh
gofmt -w .
make build test lint
```

Build the web assets before running Go tests. The binary embeds the built
assets and integration tests use them. Update `cmd/skill.md` when changing the
board format or CLI behavior it describes.
