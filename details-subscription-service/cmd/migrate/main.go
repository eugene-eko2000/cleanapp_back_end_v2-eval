package main

import (
	"context"
	"log"
	"time"

	"details-subscription-service/config"
	"details-subscription-service/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("ERROR: Failed to load config: %v", err)
	}

	db, err := database.OpenDB(cfg)
	if err != nil {
		log.Fatalf("ERROR: Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := database.RunMigrations(ctx, db); err != nil {
		log.Fatalf("ERROR: Migration failed: %v", err)
	}

	log.Println("details-subscription-service migrations applied successfully")
}
