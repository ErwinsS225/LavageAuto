package main

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Références uniques : FICHE-2026-0001, CAISSE-2026-0007.
//
// L'ancien calcul (count(*) + 1) réutilisait un numéro dès qu'une fiche était
// supprimée, et ignorait l'entreprise. On lit le plus grand numéro déjà
// utilisé, dans une transaction, et on repart de là.
// ---------------------------------------------------------------------------

// nextReference lit le prochain numéro d'une série, ou 1 si la table est vide.
// La requête doit tourner dans une transaction (FOR UPDATE) pour que deux
// ouvertures simultanées ne prennent pas le même numéro.
func nextReference(ctx context.Context, tx pgx.Tx, table, column, prefix, orgID string) (string, error) {
	// table et column viennent du code, jamais d'une entrée utilisateur.
	var maxNum *int
	err := tx.QueryRow(ctx, `
		SELECT max(NULLIF(regexp_replace(reference, '^[A-Z]+-[0-9]{4}-', ''), '')::int)
		FROM `+table+`
		WHERE organization_id = $1 AND reference LIKE $2`, orgID, prefix+"-%",
	).Scan(&maxNum)
	if err != nil {
		return "", err
	}
	next := 1
	if maxNum != nil {
		next = *maxNum + 1
	}
	return prefix + "-" + pad4(next), nil
}

// pad4 complète un numéro sur quatre chiffres.
func pad4(n int) string {
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	for len(digits) < 4 {
		digits = append([]byte{'0'}, digits...)
	}
	return string(digits)
}
