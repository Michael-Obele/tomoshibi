// Package version holds the API version the service advertises.
//
// It exists as a package rather than a string literal in the router so that
// docsgen can read it: the generated documentation states which API version it
// describes, and that number must be the one the service actually reports.
package version

// Version is advertised on GET / and stamped into the generated docs manifest.
const Version = "1.1"
