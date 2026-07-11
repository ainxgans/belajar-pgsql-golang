// Package migrations embeds the numbered .sql migration files.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
