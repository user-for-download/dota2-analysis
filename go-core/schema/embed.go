// Package schema embeds the canonical SQL migration files so they are
// compiled into any binary that imports this package.
//
// Downstream projects call migrator.Run(ctx, dsn, schema.Migrations, log)
// to apply all pending migrations.
package schema

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
