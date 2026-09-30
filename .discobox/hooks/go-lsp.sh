#!/usr/bin/env bash
#---
# name: Go LSP
# type: file
# engine: lsp
# pattern: "**/*.go"
# ignore:
#   - "server/providers/vz/internal/vzvm/*.go"
# language_id: go
# min_severity: warning
#---
# vzvm is split on `darwin && cgo` / `!darwin || !cgo`, because Code-Hex/vz is
# an entirely cgo package. Opening either half makes gopls build a darwin view,
# and that view does not resolve the cgo tag the way the go command does: it
# takes both halves as one package and reports every symbol as declared twice.
# The files the halves share pull that view in too: gopls opens the package
# for darwin as well, and there runs the darwin cgo with this machine's C
# compiler, which fails ("unrecognized command-line option '-arch'") on every
# file in the package. The split is correct — `go vet` type-checks darwin with
# cgo on and off, the Linux build and tests type-check the shared files, and
# the darwin CI job compiles the real one — so no file in vzvm is diagnosed
# here. Every other _darwin.go file in the tree still is.
set -euo pipefail

exec go tool gopls serve
