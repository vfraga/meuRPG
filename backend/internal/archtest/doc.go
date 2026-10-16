// Package archtest holds the backend's architecture tests: the module
// boundaries and the table of how the modules depend on each other (see
// boundaries_test.go and docs/architecture.md#module-boundaries-are-a-test).
// The tests read the source with go/parser, so they need no database and no
// build of the other packages. The package has no production code.
package archtest
