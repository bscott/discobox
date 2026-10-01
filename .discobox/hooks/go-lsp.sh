#!/usr/bin/env bash
#---
# name: Go LSP
# type: file
# engine: lsp
# pattern: "**/*.go"
# ignore:
#   - "server/providers/vz/internal/vzvm/*_{darwin,other}.go"
#   - "server/providers/vz/internal/vzvm/*_darwin_test.go"
# language_id: go
# min_severity: warning
#---
# vzvm is split on `darwin && cgo` / `!darwin || !cgo`, because Code-Hex/vz is
# an entirely cgo package. Opening either half makes gopls build a darwin view,
# and that view does not resolve the cgo tag the way the go command does: it
# takes both halves as one package and reports every symbol as declared twice.
# The split is correct — `go vet` type-checks darwin with cgo on and off, and
# the darwin CI job compiles the real one — so those platform halves are not
# diagnosed here. Everything else in the package, and every other _darwin.go
# file in the tree, still is.
#
# The files the halves share pull a darwin view in too, and that view must
# build without cgo. Whether the go command enables cgo for a cross-compiled
# target depends on the toolchain: Nix's Go has a C compiler built in and turns
# it on, so the view took in wake_darwin.go and ran this machine's gcc with
# `-arch`, failing every file in vzvm. Nothing else in the tree, and no
# dependency it builds off a Mac, uses cgo, so turning it off costs no
# diagnostics and leaves the view the same on every toolchain.
#
# A running hooks daemon keeps its gopls, so a change here takes effect once
# the daemon restarts, not on rerun-hooks.
set -euo pipefail

CGO_ENABLED=0 exec go tool gopls serve
