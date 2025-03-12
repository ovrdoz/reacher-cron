package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	PostgresURI string
	RedisURI    string
}

var AppConfig *Config

func LoadConfig() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warn loading .env file")
	}

	AppConfig = &Config{
		Port:        getEnv("PORT", "8081"),
		PostgresURI: getEnv("POSTGRES_URI", "localhost"),
		RedisURI:    getEnv("REDIS_URI", "localhost"),
	}
}

func getEnv(key string, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}
