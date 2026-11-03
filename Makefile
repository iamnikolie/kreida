VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint clean

# The binary embeds web/dist, so the page is built first. vite empties web/dist, which drops the tracked .keep.
build:
	pnpm --dir web install --frozen-lockfile && pnpm --dir web build
	touch web/dist/.keep
	go build -ldflags "-X github.com/iamnikolie/kreida/cmd.version=$(VERSION)" -o kreida .

test:
	go test ./...
	pnpm --dir web test

lint:
	go vet ./...

clean:
	rm -f kreida
	find web/dist -mindepth 1 ! -name .keep -delete
