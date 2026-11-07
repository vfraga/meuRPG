// Package archtest holds the backend's architecture tests.
// boundaries_test.go checks the module boundaries and the table of how the modules depend on each other.
// See docs/architecture/overview.md#module-boundaries-are-a-test.
// Five more conventions each have a test and a plain-text exception list that can only shrink.
// nogo_test.go allows no bare go statement outside internal/platform/safego.
// table_owner_test.go allows no query that touches another module's table.
// playv1_test.go allows no playv1 import outside the play module and the API wiring.
// pg_errors_test.go allows no database-error code read by hand in identity and authz.
// query_order_test.go allows no :many query without an ORDER BY in its outermost statement.
// The SQL tests read the query files that backend/sqlc.yaml lists.
// The tests read the Go source with go/parser and the SQL files as text, so they need no database and no build of the other packages.
// The package has no production code.
package archtest
