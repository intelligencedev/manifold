// manifold/initialize.go

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"manifold/internal/sefii"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
	"github.com/pterm/pterm"
)

// InitializeApplication performs necessary setup tasks, such as creating the data directory.
func InitializeApplication(config *Config) error {
	hostInfo, err := GetHostInfo()
	if err != nil {
		pterm.Error.Printf("Failed to get host information: %+v\n", err)
	} else {
		pterm.DefaultTable.WithData(pterm.TableData{
			{"Key", "Value"},
			{"OS", hostInfo.OS},
			{"Arch", hostInfo.Arch},
			{"CPUs", fmt.Sprintf("%d", hostInfo.CPUs)},
			{"Total Memory (GB)", fmt.Sprintf("%.2f", float64(hostInfo.Memory.Total)/(1024*1024*1024))},
			{"GPU Model", hostInfo.GPUs[0].Model},
			{"GPU Cores", hostInfo.GPUs[0].TotalNumberOfCores},
			{"Metal Support", hostInfo.GPUs[0].MetalSupport},
		}).Render()
	}

	if config.DataPath != "" {
		if _, err := os.Stat(config.DataPath); os.IsNotExist(err) {
			pterm.Info.Printf("Data directory '%s' does not exist, creating it...\n", config.DataPath)
			if err := os.MkdirAll(config.DataPath, 0755); err != nil {
				return fmt.Errorf("failed to create data directory: %w", err)
			}
			pterm.Success.Printf("Data directory '%s' created successfully.\n", config.DataPath)
		} else if err != nil {
			return fmt.Errorf("failed to stat data directory: %w", err)
		}
	}

	ctx := context.Background()
	db, err := Connect(ctx, config.Database.ConnectionString)
	if err != nil {
		pterm.Fatal.Println(err)
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
				pterm.Error.Printf("Failed to read directory %s: %v\n", modelDir, err)
				continue
			}

			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".gguf") {
					fullPath := filepath.Join(modelDir, file.Name())
					ggufModels = append(ggufModels, LanguageModel{
						Name:              modelName,
						Path:              fullPath,
						ModelType:         "gguf",
						Temperature:       0.6,
						TopP:              0.9,
						TopK:              50,
						RepetitionPenalty: 1.1,
						Ctx:               4096,
					})
					break
				}
			}
		}
	}

	return ggufModels, nil
}

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
				pterm.Error.Printf("Failed to read directory %s: %v\n", modelDir, err)
				continue
			}

			var safetensorsPath string
			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".safetensors") {
					fullPath := filepath.Join(modelDir, file.Name())
					safetensorsPath = fullPath
					break
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
