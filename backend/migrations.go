package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Migrations numérotées.
//
// Avant : le schéma complet était rejoué à chaque démarrage
// (CREATE TABLE IF NOT EXISTS). Une colonne renommée, une contrainte ajoutée
// ou une donnée à transformer ne passaient donc pas : il fallait intervenir à
// la main sur la base, sans trace de ce qui avait été appliqué.
//
// Désormais chaque changement est un fichier numéroté, appliqué une seule
// fois et enregistré dans `schema_migrations`. Le fichier 0001 est le schéma
// d'origine (idempotent) : sur une base déjà en service, il ne fait rien et
// la version est simplement marquée comme appliquée.
//
// Ajouter un changement : créer migrations/00NN_description.sql, jamais
// modifier un fichier déjà appliqué (une base en service ne le rejouerait pas).
// ---------------------------------------------------------------------------

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration : un fichier numéroté, exécuté dans l'ordre des noms.
type migration struct {
	version string // « 0002_wash_order_items_organization »
	sql     string
}

// loadMigrations lit les fichiers embarqués, triés par numéro croissant.
func loadMigrations() ([]migration, error) {
	entrees, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("lecture du dossier migrations : %w", err)
	}
	liste := make([]migration, 0, len(entrees))
	for _, entree := range entrees {
		if entree.IsDir() || !strings.HasSuffix(entree.Name(), ".sql") {
			continue
		}
		contenu, err := migrationsFS.ReadFile("migrations/" + entree.Name())
		if err != nil {
			return nil, fmt.Errorf("lecture de %s : %w", entree.Name(), err)
		}
		liste = append(liste, migration{
			version: strings.TrimSuffix(entree.Name(), ".sql"),
			sql:     string(contenu),
		})
	}
	sort.Slice(liste, func(i, j int) bool { return liste[i].version < liste[j].version })
	if len(liste) == 0 {
		return nil, fmt.Errorf("aucune migration embarquée")
	}
	return liste, nil
}

// verrouMigrations : identifiant arbitraire du verrou consultatif PostgreSQL.
// Deux serveurs qui démarrent en même temps ne doivent pas appliquer la même
// migration en parallèle (le second attend, puis constate que c'est fait).
const verrouMigrations = 5_141_054_119

// migrate applique les migrations non encore appliquées, une par une.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	// Une seule connexion pour tout : le verrou consultatif est lié à la
	// session, il ne peut pas être pris puis relâché depuis une autre.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("connexion pour les migrations : %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, verrouMigrations); err != nil {
		return fmt.Errorf("verrou de migration : %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, verrouMigrations)
	}()

	if _, err := conn.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("création de schema_migrations : %w", err)
	}

	appliquees := map[string]bool{}
	lignes, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("lecture des migrations appliquées : %w", err)
	}
	for lignes.Next() {
		var version string
		if err := lignes.Scan(&version); err != nil {
			lignes.Close()
			return fmt.Errorf("lecture des migrations appliquées : %w", err)
		}
		appliquees[version] = true
	}
	lignes.Close()
	if err := lignes.Err(); err != nil {
		return fmt.Errorf("lecture des migrations appliquées : %w", err)
	}

	for _, m := range migrations {
		if appliquees[m.version] {
			continue
		}
		// Chaque migration est atomique : elle passe en entier, ou pas du tout.
		if err := appliquerMigration(ctx, conn.Conn(), m); err != nil {
			return err
		}
		log.Printf("migration appliquée : %s", m.version)
	}
	return nil
}

// appliquerMigration exécute une migration et enregistre sa version dans la
// même transaction : un échec au milieu ne laisse ni schéma à moitié modifié,
// ni version marquée comme appliquée.
func appliquerMigration(ctx context.Context, conn *pgx.Conn, m migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migration %s : %w", m.version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, m.sql); err != nil {
		return fmt.Errorf("migration %s : %w", m.version, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`, m.version); err != nil {
		return fmt.Errorf("migration %s (enregistrement) : %w", m.version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migration %s (validation) : %w", m.version, err)
	}
	return nil
}
