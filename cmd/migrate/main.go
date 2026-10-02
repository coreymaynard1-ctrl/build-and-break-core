// migrate applies ordered SQL files in transactions, recording checksums.
package main

import (
	"buildandbreak/platform/internal/database"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	ctx := context.Background()
	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatal("database connection failed (check DATABASE_URL)")
	}
	defer db.Close()
	conn, err := db.Acquire(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(7261842)`); err != nil {
		log.Fatal(err)
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock(7261842)`)
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		log.Fatal(err)
	}
	// Existing databases predate migration bookkeeping. Baseline only after inspecting schema.
	baseline := len(os.Args) == 3 && os.Args[1] == "--baseline-through"
	through := ""
	if baseline {
		through = os.Args[2]
	}
	if len(os.Args) > 1 && !baseline {
		log.Fatal("usage: migrate [--baseline-through 006]")
	}
	files, err := filepath.Glob("database/migrations/*.sql")
	if err != nil {
		log.Fatal(err)
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		name := filepath.Base(path)
		checksum := fmt.Sprintf("%x", sha256.Sum256(body))
		var existing string
		err = conn.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, name).Scan(&existing)
		if err == nil {
			if existing != checksum {
				log.Fatalf("migration checksum changed: %s", name)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Fatal("failed to read migration ledger")
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if baseline && strings.SplitN(name, "_", 2)[0] <= through {
			fmt.Printf("Baseline %s (SQL not executed)\n", name)
		} else {
			if _, err = tx.Exec(ctx, string(body)); err != nil {
				tx.Rollback(ctx)
				log.Fatalf("migration %s failed: %v", name, err)
			}
			fmt.Printf("Applied %s\n", name)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, name, checksum); err != nil {
			tx.Rollback(ctx)
			log.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			log.Fatal(err)
		}
	}
}
