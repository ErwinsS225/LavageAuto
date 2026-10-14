package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// openDB crée le pool de connexions PostgreSQL (compatible Neon : SSL requis).
func openDB(databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL manquante")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("analyse DATABASE_URL : %w", err)
	}
	// Neon/Supabase-style pools dégradent parfois le protocole binaire ; le mode
	// texte est plus tolérant (scans string/timestamptz mixtes).
	cfg.ConnConfig.ConnectTimeout = 15 * time.Second
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("création du pool : %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping : %w", err)
	}
	return pool, nil
}
