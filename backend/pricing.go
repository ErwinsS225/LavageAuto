package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// pricing.go — moteur de prix (Lot 3), partagé par l'aperçu de la caisse
// (GET /price-preview) et l'enregistrement d'une fiche (POST /wash-orders).
//
// Le schéma réel (`price_rules`, 0001_initial.sql) n'a pas de colonne `kind` :
// la spécificité d'une règle se DÉDUIT de ses discriminants. Le `price_source`
// (niveau de la hiérarchie) est donc calculé ici, jamais stocké — c'est la
// lecture qui le nomme, pour qu'une facture reste explicable même si la grille
// change plus tard.
//
// Hiérarchie documentée (docs/api.md § « Prestations, options et grille
// tarifaire »), du plus précis au plus général :
//
//	1. client_contract         prestation + client (contrat, flotte)
//	2. service_category_usage  prestation + catégorie + usage
//	3. service_category | service_usage   prestation + catégorie OU + usage
//	4. service_generic         tarif du catalogue (services.price_xof)
//	-  free                    prestation déclarée gratuite (is_free)
//
// Règles tenues ici :
//   - une règle sans discriminant exigé n'est PAS applicable (une fiche sans
//     catégorie ne déclenche pas un tarif « berline ») ;
//   - deux règles de MÊME spécificité couvrant la même combinaison sont un
//     conflit (`price_rule_conflict`) : le serveur refuse de choisir ;
//   - une prestation `is_free` est gratuite et rien ne prime sur cette
//     décision ; une prestation `requires_quote` (devis) n'est pas vendable
//     en direct ;
//   - rien de tout cela ⇒ `price_required` (la prestation doit être tarifée).

// reglageBooleen lit un réglage booléen de la station (jsonb `stations.settings`),
// avec sa valeur par défaut quand le réglage n'a jamais été posé.
func (a *App) reglageBooleen(ctx context.Context, stationID, cle string, defaut bool) bool {
	var body []byte
	if err := a.db.QueryRow(ctx, `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&body); err != nil {
		return defaut
	}
	if v, ok := decoderObjet(body)[cle].(bool); ok {
		return v
	}
	return defaut
}

// reglageEntier lit un réglage entier de la station, avec sa valeur par défaut.
func (a *App) reglageEntier(ctx context.Context, stationID, cle string, defaut int) int {
	var body []byte
	if err := a.db.QueryRow(ctx, `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&body); err != nil {
		return defaut
	}
	if v, ok := decoderObjet(body)[cle].(float64); ok {
		return int(v)
	}
	return defaut
}

// niveauPrix décrit un niveau de la hiérarchie et son libellé lisible.
type niveauPrix struct {
	source string
	label  string
}

var (
	niveauContract  = niveauPrix{"client_contract", "Contrat client"}
	niveauCatUsage  = niveauPrix{"service_category_usage", "Prestation + véhicule + usage"}
	niveauCategorie = niveauPrix{"service_category", "Prestation + véhicule"}
	niveauUsage     = niveauPrix{"service_usage", "Prestation + usage"}
	niveauGenerique = niveauPrix{"service_generic", "Tarif du catalogue"}
	niveauGratuit   = niveauPrix{"free", "Prestation gratuite"}
)

// contextePrix : ce que la caisse transmet pour faire appliquer un prix.
// Les discriminants sont facultatifs — leur ABSENCE élimine les niveaux qui
// les exigent.
type contextePrix struct {
	stationID    string
	serviceID    string
	categoryCode string // code stable (berline…), vide = non précisé
	usageCode    string
	customerID   string
	date         time.Time // jour de la vente, pour valid_from/valid_to
}

// resultatPrix : ce que le moteur a retenu. `needsPrice` signale l'absence de
// tarif applicable — aucun prix n'est jamais inventé.
type resultatPrix struct {
	priceXOF    int
	source      string
	sourceLabel string
	ruleID      string
	needsPrice  bool
	message     string
}

// serviceVendu décrit la prestation telle que stockée : elle porte `is_free`
// et `requires_quote`, que le client n'a jamais le droit de fixer.
type serviceVendu struct {
	id            string
	name          string
	priceXOF      int
	isFree        bool
	requiresQuote bool
	isActive      bool
}

func (a *App) lireServiceVendu(ctx context.Context, serviceID string) (serviceVendu, error) {
	var s serviceVendu
	err := a.db.QueryRow(ctx, `
		SELECT id, name, price_xof, is_free, requires_quote, is_active
		FROM services WHERE id = $1`, serviceID).
		Scan(&s.id, &s.name, &s.priceXOF, &s.isFree, &s.requiresQuote, &s.isActive)
	return s, err
}

// ligneRegle : une règle active candidate, telle que stockée.
type ligneRegle struct {
	id           string
	priceXOF     int
	customerID   *string
	categoryCode string
	usageCode    string
	validFrom    *time.Time
	validTo      *time.Time
}

// dansLaPeriode : une règle sans borne de validité est toujours applicable.
func (r ligneRegle) dansLaPeriode(jour time.Time) bool {
	if r.validFrom != nil && jour.Before(*r.validFrom) {
		return false
	}
	if r.validTo != nil && jour.After(*r.validTo) {
		return false
	}
	return true
}

// reglesActives lit les règles actives de la prestation sur la station. On
// lit large et on filtre en mémoire : c'est la seule façon fiable de détecter
// un CONFLIT (deux règles de même spécificité), qu'un `LIMIT 1` masquerait.
func (a *App) reglesActives(ctx context.Context, c contextePrix) ([]ligneRegle, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, price_xof, customer_id, category_code, usage_code, valid_from, valid_to
		FROM price_rules
		WHERE station_id = $1 AND service_id = $2 AND is_active = true`,
		c.stationID, c.serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regles []ligneRegle
	for rows.Next() {
		var r ligneRegle
		var customerID pgtype.UUID
		var validFrom, validTo pgtype.Date
		if err := rows.Scan(&r.id, &r.priceXOF, &customerID, &r.categoryCode, &r.usageCode, &validFrom, &validTo); err != nil {
			continue
		}
		if customerID.Valid {
			v := fmtUUID(customerID)
			r.customerID = &v
		}
		if validFrom.Valid {
			t := validFrom.Time
			r.validFrom = &t
		}
		if validTo.Valid {
			t := validTo.Time
			r.validTo = &t
		}
		regles = append(regles, r)
	}
	return regles, rows.Err()
}

type regleTrouvee struct {
	id    string
	price int
}

// fmtUUID formate un UUID pgx en chaîne canonique (minuscules).
func fmtUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

// resoudrePrix applique la hiérarchie. Renvoie `needsPrice` (et non une
// erreur) quand aucun tarif ne s'applique ; renvoie une ERREUR uniquement sur
// un conflit de règles — le seul cas où le serveur refuse de trancher.
func (a *App) resoudrePrix(ctx context.Context, c contextePrix) (resultatPrix, error) {
	service, err := a.lireServiceVendu(ctx, c.serviceID)
	if err != nil {
		return resultatPrix{needsPrice: true}, err
	}

	// Devis ou prestation retirée : pas vendable au comptoir.
	if service.requiresQuote {
		return resultatPrix{needsPrice: true,
			message: "Cette prestation est sur devis : elle ne s'encaissse pas au comptoir."}, nil
	}
	if !service.isActive {
		return resultatPrix{needsPrice: true,
			message: "Cette prestation n'est plus proposée."}, nil
	}
	// La gratuité prime sur tout et ne se discute pas.
	if service.isFree {
		return resultatPrix{priceXOF: 0, source: niveauGratuit.source, sourceLabel: niveauGratuit.label}, nil
	}

	jour := c.date
	if jour.IsZero() {
		jour = time.Now()
	}
	regles, err := a.reglesActives(ctx, c)
	if err != nil {
		return resultatPrix{needsPrice: true}, err
	}

	// retenir applique un niveau : 0 candidat = passe au suivant ; 1 = retenu ;
	// 2+ = conflit (le serveur refuse de choisir entre deux règles aussi
	// précises l'une que l'autre).
	retenir := func(niveau niveauPrix, candidats []regleTrouvee) (resultatPrix, bool, error) {
		switch len(candidats) {
		case 0:
			return resultatPrix{}, false, nil
		case 1:
			return resultatPrix{
				priceXOF: candidats[0].price, source: niveau.source,
				sourceLabel: niveau.label, ruleID: candidats[0].id,
			}, true, nil
		default:
			return resultatPrix{}, false, fmt.Errorf(
				"deux règles de même niveau (%s) couvrent la même combinaison", niveau.label)
		}
	}
	applicables := func(pred func(ligneRegle) bool) []regleTrouvee {
		var out []regleTrouvee
		for _, r := range regles {
			if r.dansLaPeriode(jour) && pred(r) {
				out = append(out, regleTrouvee{id: r.id, price: r.priceXOF})
			}
		}
		return out
	}

	// Niveau 1 — contrat client (client + prestation).
	if c.customerID != "" {
		if res, ok, err := retenir(niveauContract, applicables(func(r ligneRegle) bool {
			return r.customerID != nil && *r.customerID == c.customerID
		})); err != nil {
			return resultatPrix{needsPrice: true}, err
		} else if ok {
			return res, nil
		}
	}

	// Niveau 2 — prestation + catégorie + usage (les deux exigés).
	if c.categoryCode != "" && c.usageCode != "" {
		if res, ok, err := retenir(niveauCatUsage, applicables(func(r ligneRegle) bool {
			return r.categoryCode == c.categoryCode && r.usageCode == c.usageCode
		})); err != nil {
			return resultatPrix{needsPrice: true}, err
		} else if ok {
			return res, nil
		}
	}

	// Niveau 3 — prestation + catégorie SEULE, ou prestation + usage SEUL.
	// (Une règle catégorie+usage est déjà passée au niveau 2 ; ici on ne
	// retient que les règles à un seul discriminant pertinent.)
	var candidatsSeuls []regleTrouvee
	niveauSeul := niveauCategorie
	if c.categoryCode != "" {
		candidatsSeuls = applicables(func(r ligneRegle) bool {
			return r.categoryCode == c.categoryCode && r.usageCode == ""
		})
	}
	if c.usageCode != "" {
		candidatsUsage := applicables(func(r ligneRegle) bool {
			return r.usageCode == c.usageCode && r.categoryCode == ""
		})
		if len(candidatsSeuls) == 0 && len(candidatsUsage) > 0 {
			niveauSeul = niveauUsage
		}
		candidatsSeuls = append(candidatsSeuls, candidatsUsage...)
	}
	if len(candidatsSeuls) > 0 {
		if res, ok, err := retenir(niveauSeul, candidatsSeuls); err != nil {
			return resultatPrix{needsPrice: true}, err
		} else if ok {
			return res, nil
		}
	}

	// Niveau 4 — tarif du catalogue (repli), sauf si la station l'interdit
	// (`allow_generic_price: false`). La gratuité est déjà traitée plus haut.
	generiqueAutorise := a.reglageBooleen(ctx, c.stationID, "allow_generic_price", true)
	if generiqueAutorise && service.priceXOF > 0 {
		return resultatPrix{
			priceXOF: service.priceXOF, source: niveauGenerique.source, sourceLabel: niveauGenerique.label,
		}, nil
	}

	return resultatPrix{needsPrice: true,
		message: "Aucun tarif applicable : posez un prix dans la grille tarifaire pour cette combinaison."}, nil
}

// calculerPrixLavage calcule le prix d'un lavage pour une commande.
// Elle prend le vehicleID et packageID pour déduire category_code, usage_code et service_id.
func (a *App) calculerPrixLavage(ctx context.Context, stationID, vehicleID, packageID string, options map[string]interface{}) (map[string]interface{}, error) {
	var categoryCode, usageCode, serviceID string
	
	// Récupérer le véhicule pour obtenir category_code et usage_code
	if vehicleID != "" {
		a.db.QueryRow(ctx, `SELECT category_code, usage_code FROM vehicles WHERE id = $1 AND station_id = $2`, vehicleID, stationID).Scan(&categoryCode, &usageCode)
	}
	
	// Récupérer le package pour obtenir le service_id
	if packageID != "" {
		a.db.QueryRow(ctx, `SELECT service_id FROM wash_packages WHERE id = $1 AND station_id = $2`, packageID, stationID).Scan(&serviceID)
	}
	
	// Si pas de vehicleID/packageID, essayer de lire dans options
	if serviceID == "" {
		if v, ok := options["service_id"].(string); ok {
			serviceID = v
		}
	}
	if categoryCode == "" {
		if v, ok := options["category_code"].(string); ok {
			categoryCode = v
		}
	}
	if usageCode == "" {
		if v, ok := options["usage_code"].(string); ok {
			usageCode = v
		}
	}
	
	if serviceID == "" {
		return nil, fmt.Errorf("service_id manquant (ni dans package_id ni dans options)")
	}
	
	res, err := a.resoudrePrix(ctx, contextePrix{
		stationID:    stationID,
		serviceID:    serviceID,
		categoryCode: categoryCode,
		usageCode:    usageCode,
		customerID:   "",
		date:         time.Now(),
	})
	if err != nil {
		return nil, err
	}
	
	prix := map[string]interface{}{
		"unit_price":    res.priceXOF,
		"total":         res.priceXOF,
		"source":        res.source,
		"source_label":  res.sourceLabel,
	}
	if res.ruleID != "" {
		prix["rule_id"] = res.ruleID
	}
	if res.needsPrice {
		prix["needs_price"] = true
		if res.message != "" {
			prix["message"] = res.message
		}
	}
	
	return prix, nil
}
