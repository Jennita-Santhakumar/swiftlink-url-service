package store

import "embed"

// MigrationsFS embeds the SQL migration files so cmd/migrate can apply them
// without relying on a filesystem path at runtime.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
