// Package testutil provides isolated PostgreSQL schemas for opt-in integration tests.
package testutil

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("BNB_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("BNB_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal("test database configuration failed")
	}
	schema := fmt.Sprintf("bnb_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		admin.Close()
		t.Fatal("create isolated test schema failed")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("test database configuration failed")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("remove own test schema: %v", err)
		}
		admin.Close()
	})
	root := "../.."
	files, err := filepath.Glob(filepath.Join(root, "database/migrations/*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatal("migration files not found")
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(body)); err != nil {
			t.Fatalf("fresh schema migration %s: %v", filepath.Base(path), err)
		}
	}
	return db
}
