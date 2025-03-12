package client

import (
	"database/sql"
	"log"
	"reacher-cron/config"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	db   *sql.DB
	once sync.Once
)

func ConnectPostgres() *sql.DB {
	once.Do(func() {
		var err error
		db, err = sql.Open("postgres", config.AppConfig.PostgresURI)
		if err != nil {
			log.Fatal(err)
		}

		// Configuração do pool de conexões
		db.SetMaxOpenConns(25)                  // Máximo de conexões abertas
		db.SetMaxIdleConns(5)                   // Máximo de conexões inativas
		db.SetConnMaxLifetime(15 * time.Minute) // Tempo máximo de uso de uma conexão

		if err = db.Ping(); err != nil {
			log.Fatal("Postgres connection failed:", err)
		}
	})

	return db
}
