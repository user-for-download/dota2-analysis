package matchpg

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func jsonbOrNull(raw []byte) any {
	s := bytes.TrimSpace(raw)
	if len(s) == 0 || bytes.Equal(s, []byte("null")) {
		return nil
	}
	// Return as string so pgx.CopyFrom sends it as text, avoiding binary jsonb encoding errors.
	return string(raw)
}

func nullIf0_64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIf0_32(v int32) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIf0_16(v int16) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIf0_f32(v float32) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIfStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func bulkUpsert(ctx context.Context, tx pgx.Tx, tempTable, targetTable string, columns []string, conflictClause string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}

	cols := strings.Join(columns, ", ")

	createTemp := fmt.Sprintf(`CREATE TEMP TABLE IF NOT EXISTS %s (LIKE %s INCLUDING DEFAULTS) ON COMMIT DELETE ROWS`, tempTable, targetTable)
	if _, err := tx.Exec(ctx, createTemp); err != nil {
		return fmt.Errorf("create temp table %s: %w", tempTable, err)
	}

	_, err := tx.CopyFrom(ctx,
		pgx.Identifier{tempTable},
		columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("copy %s: %w", tempTable, err)
	}

	upsert := fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s %s`, targetTable, cols, cols, tempTable, conflictClause)
	if _, err := tx.Exec(ctx, upsert); err != nil {
		return fmt.Errorf("upsert %s: %w", targetTable, err)
	}

	return nil
}