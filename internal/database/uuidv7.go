package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nself-org/cli/internal/config"
)

// uuidV7FallbackSQL defines public.uuidv7() only when nothing already provides
// it, so an image that ships the pg_uuidv7 extension keeps the C implementation
// and this is a no-op.
//
// The body is the standard community implementation, not a bespoke one: take a
// v4 UUID, overlay the first 6 bytes with the big-endian millisecond timestamp,
// then set bits 52 and 53 to turn the version nibble 0100 (v4) into 0111 (v7).
// gen_random_uuid() has already set the RFC 9562 variant bits, which the overlay
// does not touch.
const uuidV7FallbackSQL = `
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE p.proname = 'uuidv7' AND n.nspname = 'public'
  ) THEN
    EXECUTE $fn$
      CREATE FUNCTION public.uuidv7() RETURNS uuid
      LANGUAGE sql VOLATILE
      AS $body$
        SELECT encode(
          set_bit(
            set_bit(
              overlay(
                uuid_send(gen_random_uuid())
                PLACING substring(
                  int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint)
                  FROM 3
                )
                FROM 1 FOR 6
              ),
              52, 1
            ),
            53, 1
          ),
          'hex'
        )::uuid
      $body$;
    $fn$;
  END IF;
END $$;`

// ensureUUIDv7 guarantees that public.uuidv7() exists after initialization.
//
// Why this is not just another entry in createExtensions' list: the base image
// (pgvector/pgvector:pg16) does not ship pg_uuidv7, so `CREATE EXTENSION
// pg_uuidv7` errors and would abort startup for every user. The embedded pglite
// runtime cannot load a C extension at all.
//
// So: try the extension first, because a C implementation is better when it is
// there, and tolerate its absence. Then install the SQL fallback, which no-ops
// when the extension already defined the function. Either way the post-condition
// is the same and is strictly stronger than before — public.uuidv7() exists.
//
// This closes a real trap reported by ummeco/ummat: a migration ran
// `CREATE EXTENSION IF NOT EXISTS pg_uuidv7`, soft-failed with a WARNING, was
// still recorded as applied, and its own header told later DDL to use
// `DEFAULT public.uuidv7()`. Two later migrations then died with
// "function public.uuidv7() does not exist".
func ensureUUIDv7(ctx context.Context, cfg *config.Config) error {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	// Best effort: present in a custom image, absent in the stock one.
	if err := runSQLOnDB(ctx, cfg, db, "CREATE EXTENSION IF NOT EXISTS pg_uuidv7"); err != nil {
		slog.Debug("pg_uuidv7 extension unavailable, using SQL fallback", "err", err)
	}

	// Not best effort: after this, the function must exist.
	if err := runSQLOnDB(ctx, cfg, db, uuidV7FallbackSQL); err != nil {
		return fmt.Errorf("ensure public.uuidv7(): %w", err)
	}

	slog.Info("public.uuidv7() available")
	return nil
}
