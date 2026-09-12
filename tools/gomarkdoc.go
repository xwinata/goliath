package tools

// gomarkdoc generates per-package README.md files from Go doc comments.
//
// Regenerate all package docs from the module root:
//
//	go generate ./tools
//
// or run gomarkdoc directly:
//
//	go tool gomarkdoc --output '{{.Dir}}/README.md' ./pkg/...

//go:generate go tool gomarkdoc --output {{.Dir}}/README.md ../...
