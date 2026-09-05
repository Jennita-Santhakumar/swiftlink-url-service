// Command migrate applies (or rolls back) database migrations. Run as a
// one-shot Docker Compose service gating api/worker startup.
package main

import (
	"errors"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/KabileshRajaselvan/url-shortener/internal/config"
	"github.com/KabileshRajaselvan/url-shortener/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	src, err := iofs.New(store.MigrationsFS, "migrations")
	if err != nil {
		log.Fatalf("failed to load embedded migrations: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to init migrator: %v", err)
	}

	switch direction {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	default:
		log.Fatalf("unknown migrate direction %q (expected up or down)", direction)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migration failed: %v", err)
	}
	log.Println("migrations applied successfully")
}
