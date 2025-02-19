// manifold/initialize.go

package main

import (
	"context"
	"fmt"
	"log"
	"manifold/internal/sefii"
	"os"

	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

// InitializeApplication performs necessary setup tasks, such as creating the data directory.
func InitializeApplication(config *Config) error {
	// Check if the data directory exists. If not, create it.
	if config.DataPath != "" {
		if _, err := os.Stat(config.DataPath); os.IsNotExist(err) {
			log.Printf("Data directory '%s' does not exist, creating it...", config.DataPath)
			if err := os.MkdirAll(config.DataPath, 0755); err != nil {
				return fmt.Errorf("failed to create data directory: %w", err)
			}
			log.Printf("Data directory '%s' created successfully.", config.DataPath)
		} else if err != nil {
			return fmt.Errorf("failed to stat data directory: %w", err)
		}
	}

	// Bootstrap sefii engine.
	ctx := context.Background()
	db, err := Connect(ctx, config.Database.ConnectionString)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close(ctx)

	_, err = db.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		panic(err)
	}
	err = pgxvector.RegisterTypes(ctx, db)
	if err != nil {
		panic(err)
	}

	engine := sefii.NewEngine(db)
	engine.EnsureTable(ctx, config.Embeddings.EmbeddingVectors)
	engine.EnsureInvertedIndexTable(ctx)

	return nil
}
