// Package tools centralizes build-time tooling for this module.
//
// Tool binaries are pinned in go.mod via `tool` directives (see `go get -tool`)
// so everyone runs the same versions. Each tool has its own file in this
// package holding its go:generate directive and usage notes (for example,
// gomarkdoc.go for documentation generation).
package tools
