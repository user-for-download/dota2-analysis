package ingester

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker"
)

type DBConstraintError struct {
	Code    string
	Detail  string
	Message string
}

func (e *DBConstraintError) Error() string {
	return fmt.Sprintf("db constraint error %s: %s", e.Code, e.Message)
}

func ClassifyDBError(err error) *DBConstraintError {
	var pgErr *pgconn.PgError
	if err == nil || !errors.As(err, &pgErr) {
		return nil
	}
	return &DBConstraintError{
		Code:    pgErr.Code,
		Detail:  pgErr.Detail,
		Message: pgErr.Message,
	}
}

func IsRetryable(err error) bool {
	ce := ClassifyDBError(err)
	if ce == nil {
		return false
	}
	return ce.Code == "40001" || ce.Code == "40P01" || ce.Code == "57014"
}

func IsValidation(err error) bool {
	ce := ClassifyDBError(err)
	if ce == nil {
		return false
	}
	return ce.Code == "23505" || ce.Code == "23503"
}

func (e *DBConstraintError) IsForeignKey() bool { return e.Code == "23503" }

func IsAlreadySeen(err error) bool {
	return errors.Is(err, worker.ErrAlreadySeen)
}