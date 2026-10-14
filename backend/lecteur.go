package main

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// lecteur : lire la base depuis un pool **ou** depuis une transaction.
//
// Les contrôles de mise en route doivent pouvoir tourner à l'intérieur de la
// transaction d'activation : vérifier hors transaction puis écrire dedans
// laisserait passer une configuration modifiée entre les deux lectures — un
// tarif supprimé, une catégorie désactivée — et l'activation validerait un état
// qui n'existe plus.
//
// Le type est volontairement minuscule : il ne remplace pas `a.Pool`, il
// autorise à passer `pgx.Tx` là où le pool était attendu. Les requêtes, elles,
// restent écrites une seule fois.
// ---------------------------------------------------------------------------

type lecteur interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}
