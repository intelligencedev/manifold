// manifold/initialize.go

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"manifold/internal/sefii"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
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
	engine.EnsureTable(ctx, config.Embeddings.Dimensions)
	engine.EnsureInvertedIndexTable(ctx)

	// Create a database table for models and their configurations
	// err = CreateModelsTable(ctx, db)
	// if err != nil {
	// 	return fmt.Errorf("failed to create models table: %w", err)
	// }

	// modelsDir := fmt.Sprintf("%s/models", config.DataPath)

	// // Scan the models directories and insert the models into the database
	// ggufModels, err := ScanGGUFModels(modelsDir)
	// if err != nil {
	// 	return fmt.Errorf("failed to scan GGUF models: %w", err)
	// }

	// mlxModels, err := ScanMLXModels(modelsDir)
	// if err != nil {
	// 	return fmt.Errorf("failed to scan MLX models: %w", err)
	// }

	// // Insert the gguf models into the database with a gguf model type, do not use engine!
	// for _, model := range ggufModels {
	// 	_, err := db.Exec(ctx, `
	// 	INSERT INTO models (name, path, model_type, temperature, top_p, top_k, repetition_penalty, ctx)
	// 	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	// `, model.Name, model.Path, model.ModelType, model.Temperature, model.TopP, model.TopK, model.RepetitionPenalty, model.Ctx)
	// 	if err != nil {
	// 		return fmt.Errorf("failed to insert model into database: %w", err)
	// 	}

	// 	log.Printf("Inserted GGUF model '%s' into the database", model.Name)
	// }

	// // Insert the mlx models into the database with a mlx model type, do not use engine!
	// for _, model := range mlxModels {
	// 	_, err := db.Exec(ctx, `
	// 	INSERT INTO models (name, path, model_type, temperature, top_p, top_k, repetition_penalty, ctx)
	// 	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	// `, model.Name, model.Path, model.ModelType, model.Temperature, model.TopP, model.TopK, model.RepetitionPenalty, model.Ctx)
	// 	if err != nil {
	// 		return fmt.Errorf("failed to insert model into database: %w", err)
	// 	}

	// 	log.Printf("Inserted MLX model '%s' into the database", model.Name)
	// }

	return nil
}

func CreateModelsTable(ctx context.Context, db *pgx.Conn) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS models (
			id SERIAL PRIMARY KEY,
			name TEXT UNIQUE,
			path TEXT UNIQUE,
			model_type TEXT,
			temperature FLOAT,
			top_p FLOAT,
			top_k INT,
			repetition_penalty FLOAT,
			ctx INT
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create models table: %w", err)
	}

	return nil
}

// ScanGGUFModels scans the "models-gguf" directory and returns a list of models.
func ScanGGUFModels(modelsDir string) ([]LanguageModel, error) {
	var ggufModels []LanguageModel

	ggufPath := filepath.Join(modelsDir, "models-gguf")
	entries, err := os.ReadDir(ggufPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read models-gguf directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			modelName := entry.Name()
			modelDir := filepath.Join(ggufPath, modelName)

			files, err := ioutil.ReadDir(modelDir)
			if err != nil {
				log.Printf("Failed to read directory %s: %v", modelDir, err)
				continue
			}

			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".gguf") {
					fullPath := filepath.Join(modelDir, file.Name())
					ggufModels = append(ggufModels, LanguageModel{
						Name:              modelName,
						Path:              fullPath,
						ModelType:         "gguf",
						Temperature:       0.5,
						TopP:              0.9,
						TopK:              50,
						RepetitionPenalty: 1.1,
						Ctx:               4096,
					})
					break // Only first gguf file per model
				}
			}
		}
	}

	return ggufModels, nil
}

// ScanMLXModels scans the "models-mlx" directory and returns a list of models.
func ScanMLXModels(modelsDir string) ([]LanguageModel, error) {
	var mlxModels []LanguageModel

	mlxPath := filepath.Join(modelsDir, "models-mlx")
	entries, err := os.ReadDir(mlxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read models-mlx directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			modelName := entry.Name()
			modelDir := filepath.Join(mlxPath, modelName)

			files, err := os.ReadDir(modelDir)
			if err != nil {
				log.Printf("Failed to read directory %s: %v", modelDir, err)
				continue
			}

			var safetensorsPath string
			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".safetensors") {
					fullPath := filepath.Join(modelDir, file.Name())
					safetensorsPath = fullPath
					break // Only first safetensors file per model
				}
			}

			if safetensorsPath != "" {
				mlxModels = append(mlxModels, LanguageModel{
					Name:              modelName,
					Path:              safetensorsPath,
					ModelType:         "mlx",
					Temperature:       0.5,
					TopP:              0.9,
					TopK:              50,
					RepetitionPenalty: 1.1,
					Ctx:               4096,
				})
			}
		}
	}

	return mlxModels, nil
}
