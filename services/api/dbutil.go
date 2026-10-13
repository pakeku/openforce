package main

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
