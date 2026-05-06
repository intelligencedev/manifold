package agentd

import "github.com/joho/godotenv"

func loadEnv() error {
	if err := godotenv.Load(".env"); err != nil {
		return godotenv.Load("example.env")
	}
	return nil
}
