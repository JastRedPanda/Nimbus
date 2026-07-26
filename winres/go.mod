// Build-time-only module. It is deliberately separate from the application
// module so that goversioninfo and rsrc never enter the dependency graph of
// the binary users install: `go build ./...` and `go mod tidy` in the parent
// directory skip any subtree that has its own go.mod.
module github.com/JastRedPanda/Nimbus/winres

go 1.23

require (
	github.com/akavel/rsrc v0.10.2
	github.com/josephspurrier/goversioninfo v1.7.0
)
