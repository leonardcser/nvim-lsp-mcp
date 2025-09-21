SHELL := /bin/sh

.PHONY: all server clean

all: server

server:
	go build -o nvim-lsp-mcp ./cmd/server

clean:
	rm -f nvim-lsp-mcp
