// account-admin is an operator-only CLI: public registration never creates admins.
package main

import (
	"buildandbreak/platform/internal/database"
	"context"
	"flag"
	"log"
	"strings"
)

func main() {
	email := flag.String("email", "", "registered account to grant administrator access")
	flag.Parse()
	if *email == "" {
		log.Fatal("usage: account-admin --email registered@example.com")
	}
	ctx := context.Background()
	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatal("database unavailable")
	}
	defer db.Close()
	tag, err := db.Exec(ctx, `UPDATE accounts SET role='admin' WHERE email=$1 AND NOT disabled`, strings.ToLower(strings.TrimSpace(*email)))
	if err != nil || tag.RowsAffected() != 1 {
		log.Fatal("active registered account not found")
	}
	log.Print("Administrator access granted to registered account")
}
