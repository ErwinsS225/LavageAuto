package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Moteur de prix — un seul endroit décide, le serveur.
//
// Quatre niveaux, du plus particulier au plus général :
//
//	1. contrat client        (client_contract)
//	2. prestation + catégorie + usage (service_category_usage)
//	3. prestation + catégorie (service_category)  — ou prestation + usage
//	   (service_usage) quand la station ne tarife que par usage
//	4. tarif générique       (service_generic) — `services.price_xof`, si la
//	   station autorise ce niveau
//
// Le niveau retenu est renvoyé avec le montant : l'écran l'affiche et la fiche
// le conserve, pour qu'un prix puisse toujours s'expliquer après coup.
//
// Deux règles tenues ici :
//
//   - **prix absent ≠ prix zéro** : sans tarif utilisable, la vente est refusée
//     (`409 price_required`) ;
//   - **gratuit est une décision** : seule une prestation marquée `is_free`
//     se vend à zéro.
//
// Aucun montant n'est calculé côté navigateur : le front envoie des choix
// (prestation, catégorie, usage, options), le serveur en déduit le prix.
// ---------------------------------------------------------------------------

// Sources de prix — la valeur stockée dans `wash_order_items.price_source`.
const (
	sourceContratClient    = "client_contract"
	sourceServiceCatUsage  = "service_category_usage"
	sourceServiceCategorie = "service_category"
	sourceServiceUsage     = "service_usage"
	sourceGenerique        = "service_generic"
	sourceGratuit          = "free"
)

// libelleSourcePrix : ce que l'interface peut afficher sans traduire elle-même.
func libelleSourcePrix(source string) string {
	switch source {
	case sourceContratClient:
		return "Contrat client"
	case sourceServiceCatUsage:
		return "Tarif prestation + véhicule + usage"
	case sourceServiceCategorie:
		return "Tarif prestation + véhicule"
	case sourceServiceUsage:
		return "Tarif prestation + usage"
	case sourceGenerique:
		return "Tarif du catalogue"
	case sourceGratuit:
		return "Prestation gratuite"
	default:
		return "Aucun tarif"
	}
}

// prixResolu : le montant et son explication.
type prixResolu struct {
	Montant int    `json:"price_xof"`
	Source  string `json:"price_source"`
	Libelle string `json:"price_source_label"`
	RuleID  string `json:"price_rule_id,omitempty"`
	Motif   string `json:"price_reason,omitempty"`
}

// prixRequis : aucune règle utilisable et aucun tarif générique. Le handler la
// traduit en `409 price_required` en nommant la prestation concernée.
type prixRequis struct {
	Service string
}

func (e prixRequis) Error() string {
	return fmt.Sprintf("« %s » n'a pas encore de tarif", e.Service)
}

// estPrixRequis : le handler décide du code HTTP, pas le moteur.
func estPrixRequis(err error) (prixRequis, bool) {
	var manque prixRequis
	return manque, errors.As(err, &manque)
}

// reglePrix : une règle candidate lue en base.
//
// `Service` n'est renseigné que là où la lecture ne filtre pas déjà par
// prestation (la matrice lit toutes les règles d'un coup : sans ce champ, la
// règle de « Lavage extérieur × berline » s'afficherait sur **toutes** les
// lignes).
type reglePrix struct {
	ID        string
	Service   string
	Categorie string
	Usage     string
	Client    string
	Montant   int
	Motif     string
}

// specificite : combien de critères une règle renseigne, et lesquels. Un contrat
// client passe avant tout ; viennent ensuite catégorie + usage, puis catégorie
// seule, puis usage seul.
func (r reglePrix) specificite() int {
	score := 0
	if r.Client != "" {
		score += 100
	}
	if r.Categorie != "" {
		score += 10
	}
	if r.Usage != "" {
		score += 5
	}
	return score
}

func (r reglePrix) source() string {
	switch {
	case r.Client != "":
		return sourceContratClient
	case r.Categorie != "" && r.Usage != "":
		return sourceServiceCatUsage
	case r.Categorie != "":
		return sourceServiceCategorie
	default:
		return sourceServiceUsage
	}
}

// resolvePrice applique la hiérarchie pour **une** prestation.
//
// `categorie`, `usage` et `clientID` peuvent être vides : une fiche de caisse
// n'oblige pas à préciser le véhicule, et un client de passage n'existe pas en
// base. Dans ce cas, seules les règles qui ne se prononcent pas sur le critère
// manquant s'appliquent — le tarif générique reste le recours.
func (a *App) resolvePrice(ctx context.Context, stationID string, service Service,
	categorie, usage, clientID string, jour time.Time) (prixResolu, error) {

	// 1. Une prestation explicitement gratuite ne se tarife pas : c'est une
	//    décision de l'exploitant, et elle prime sur toute règle.
	if service.IsFree {
		return prixResolu{Montant: 0, Source: sourceGratuit, Libelle: libelleSourcePrix(sourceGratuit)}, nil
	}

	lignes, err := a.Pool.Query(ctx, `
		SELECT id, category_code, usage_code, COALESCE(customer_id, ''), price_xof, reason
		FROM price_rules
		WHERE station_id = $1 AND service_id = $2 AND is_active
		  AND (category_code = '' OR category_code = $3)
		  AND (usage_code = '' OR usage_code = $4)
		  AND (customer_id IS NULL OR customer_id = $5)
		  AND (valid_from IS NULL OR valid_from <= $6)
		  AND (valid_to IS NULL OR valid_to >= $6)`,
		stationID, service.ID, categorie, usage, clientID, jour)
	if err != nil {
		return prixResolu{}, err
	}
	defer lignes.Close()

	candidats := []reglePrix{}
	for lignes.Next() {
		var r reglePrix
		if err := lignes.Scan(&r.ID, &r.Categorie, &r.Usage, &r.Client, &r.Montant, &r.Motif); err != nil {
			return prixResolu{}, err
		}
		// Le discriminant demandé doit correspondre : une règle « berline »
		// ne s'applique pas à une fiche sans véhicule précisé, et une règle
		// « taxi compteur » ne s'applique pas à une fiche sans usage.
		if (categorie == "" && r.Categorie != "") || (usage == "" && r.Usage != "") {
			continue
		}
		if r.Client != "" && (clientID == "" || r.Client != clientID) {
			continue
		}
		candidats = append(candidats, r)
	}
	if err := lignes.Err(); err != nil {
		return prixResolu{}, err
	}

	if len(candidats) > 0 {
		meilleure := candidats[0]
		egalite := false
		for _, r := range candidats[1:] {
			switch {
			case r.specificite() > meilleure.specificite():
				meilleure, egalite = r, false
			case r.specificite() == meilleure.specificite():
				egalite = true
			}
		}
		// Deux règles aussi précises l'une que l'autre : on ne choisit pas en
		// silence. L'index d'unicité l'interdit déjà à l'écriture ; ce garde-fou
		// couvre les données plus anciennes.
		if egalite {
			return prixResolu{}, errPrixAmbigu
		}
		return prixResolu{
			Montant: meilleure.Montant,
			Source:  meilleure.source(),
			Libelle: libelleSourcePrix(meilleure.source()),
			RuleID:  meilleure.ID,
			Motif:   meilleure.Motif,
		}, nil
	}

	// 2. Tarif générique du catalogue — sauf si la station a décidé de ne
	//    vendre que par règles (réglage `allow_generic_price`).
	if service.PriceXOF > 0 && a.stationAutorisePrixGenerique(ctx, stationID) {
		return prixResolu{
			Montant: service.PriceXOF,
			Source:  sourceGenerique,
			Libelle: libelleSourcePrix(sourceGenerique),
		}, nil
	}

	// 3. Rien : la vente est refusée plutôt que facturée zéro.
	return prixResolu{}, prixRequis{Service: service.Name}
}

// stationAutorisePrixGenerique lit le réglage de la station. Par défaut, le
// tarif du catalogue s'applique (c'est le comportement historique) ; une
// station peut le couper pour n'accepter que des règles explicites.
func (a *App) stationAutorisePrixGenerique(ctx context.Context, stationID string) bool {
	if stationID == "" {
		return true
	}
	var brut []byte
	if err := a.Pool.QueryRow(ctx, `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&brut); err != nil {
		return true
	}
	reglages := map[string]any{}
	parseJSONB(brut, &reglages)
	if valeur, ok := reglages["allow_generic_price"].(bool); ok {
		return valeur
	}
	return true
}

// prestationSansTarif : cette prestation est-elle encore « à tarifer » ?
//
// Vrai quand ni une règle active, ni le tarif du catalogue (s'il est autorisé)
// ne donnent de montant — une prestation gratuite, par décision, n'est jamais
// « à tarifer ». L'écran s'en sert pour afficher l'état réel plutôt qu'un zéro
// trompeur, et la caisse refuse la vente (`409 price_required`).
func (a *App) prestationSansTarif(ctx context.Context, service Service, stationID string) bool {
	if service.IsFree {
		return false
	}
	var reglee bool
	if err := a.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM price_rules
		                WHERE service_id = $1 AND station_id = $2 AND is_active
		                  AND (valid_from IS NULL OR valid_from <= CURRENT_DATE)
		                  AND (valid_to IS NULL OR valid_to >= CURRENT_DATE))`,
		service.ID, stationID).Scan(&reglee); err != nil {
		// En cas de doute, on n'annonce pas « à tarifer » à tort : le refus
		// éventuel viendra de la caisse, avec son message précis.
		return false
	}
	if reglee {
		return false
	}
	if service.PriceXOF > 0 && a.stationAutorisePrixGenerique(ctx, stationID) {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Options et suppléments
// ---------------------------------------------------------------------------

// optionDemandee : ce que le poste de caisse envoie (identifiant + quantité).
// Les noms et les prix ne viennent jamais du navigateur.
type optionDemandee struct {
	OptionID string `json:"option_id"`
	Quantity int    `json:"quantity"`
}

// optionRetenue : une option validée, avec son prix au moment de la vente.
type optionRetenue struct {
	ID       string `json:"option_id"`
	Name     string `json:"name"`
	PriceXOF int    `json:"price_xof"`
	Quantity int    `json:"quantity"`
	Total    int    `json:"total_xof"`
}

// ligneRecalculee : le prix d'une ligne après changement de véhicule.
type ligneRecalculee struct {
	ItemID      string
	UnitPrice   int
	LineTotal   int
	PriceSource string
}

// recalculerLignes : refait le prix de chaque ligne d'une fiche quand sa
// catégorie ou son usage commercial change.
//
// Sans ce recalcul, une fiche passée de « berline » à « poids lourd » garderait
// le tarif de la berline : le total affiché cesserait de correspondre à la
// grille. Les suppléments d'options sont conservés tels quels — ce sont des
// montants en FCFA, pas des pourcentages à réappliquer.
func (a *App) recalculerLignes(ctx context.Context, order *WashOrder, lignes []WashOrderItem,
	categorie, usage string) ([]ligneRecalculee, int, error) {
	resultat := []ligneRecalculee{}
	subtotal := 0
	for _, ligne := range lignes {
		service, err := scanService(a.Pool.QueryRow(ctx, `
			SELECT `+serviceColumnsSansAlias+`
			FROM services WHERE id = $1 AND organization_id = $2`,
			ligne.ServiceID, order.OrganizationID))
		if err != nil {
			return nil, 0, prixRequis{Service: serviceNomDeLaLigne(ligne)}
		}
		prix, err := a.resolvePrice(ctx, order.StationID, service, categorie, usage,
			deref(order.CustomerID), time.Now())
		if err != nil {
			return nil, 0, err
		}
		qty := ligne.Quantity
		if qty <= 0 {
			qty = 1
		}
		total := prix.Montant*qty + ligne.OptionsTotalXOF
		subtotal += total
		resultat = append(resultat, ligneRecalculee{
			ItemID: ligne.ID, UnitPrice: prix.Montant, LineTotal: total, PriceSource: prix.Source,
		})
	}
	return resultat, subtotal, nil
}

func serviceNomDeLaLigne(ligne WashOrderItem) string {
	if ligne.ServiceName != "" {
		return ligne.ServiceName
	}
	return "Cette prestation"
}

// chargerOptions valide les options demandées pour une prestation et calcule
// leur total **par unité** (les options se facturent comme la prestation : par
// lavage, pas par fiche).
func (a *App) chargerOptions(ctx context.Context, stationID, serviceID, categorie string,
	demandes []optionDemandee) ([]optionRetenue, int, error) {

	if len(demandes) == 0 {
		return []optionRetenue{}, 0, nil
	}
	retenues := []optionRetenue{}
	total := 0
	for _, demande := range demandes {
		if strings.TrimSpace(demande.OptionID) == "" {
			continue
		}
		quantite := demande.Quantity
		if quantite <= 0 {
			quantite = 1
		}
		var (
			id, nom      string
			prix         int
			serviceLie   *string
			categorieLie string
		)
		err := a.Pool.QueryRow(ctx, `
			SELECT id, name, price_xof, service_id, category_code
			FROM service_options
			WHERE id = $1 AND station_id = $2 AND is_active`,
			demande.OptionID, stationID).Scan(&id, &nom, &prix, &serviceLie, &categorieLie)
		if err != nil {
			return nil, 0, &apiError{
				Status: 400, Code: "validation_error",
				Message: "Option inconnue ou désactivée pour cette station.",
				Fields:  map[string]string{"items": "Option inconnue"},
			}
		}
		// Une option rattachée à une prestation ne s'applique qu'à celle-ci ;
		// une option rattachée à une catégorie ne s'applique qu'à ces véhicules.
		if serviceLie != nil && *serviceLie != "" && *serviceLie != serviceID {
			return nil, 0, &apiError{
				Status: 400, Code: "validation_error",
				Message: "Cette option ne s'applique pas à la prestation choisie.",
				Fields:  map[string]string{"items": "Option non applicable"},
			}
		}
		if categorieLie != "" && categorie != "" && categorieLie != categorie {
			return nil, 0, &apiError{
				Status: 400, Code: "validation_error",
				Message: "Cette option ne s'applique pas à ce type de véhicule.",
				Fields:  map[string]string{"items": "Option non applicable"},
			}
		}
		ligne := optionRetenue{ID: id, Name: nom, PriceXOF: prix, Quantity: quantite,
			Total: prix * quantite}
		retenues = append(retenues, ligne)
		total += ligne.Total
	}
	return retenues, total, nil
}

// optionsDisponibles : ce que la caisse peut proposer pour une prestation et un
// véhicule donnés. Sans compatibilité déclarée, tout est proposé.
func (a *App) optionsDisponibles(ctx context.Context, stationID, serviceID, categorie string) ([]map[string]any, error) {
	lignes, err := a.Pool.Query(ctx, `
		SELECT id, name, description, price_xof, service_id, category_code, duration_minutes
		FROM service_options
		WHERE station_id = $1 AND is_active
		  AND (service_id IS NULL OR service_id = $2)
		  AND (category_code = '' OR $3 = '' OR category_code = $3)
		ORDER BY sort_order, name`, stationID, serviceID, categorie)
	if err != nil {
		return nil, err
	}
	defer lignes.Close()
	items := []map[string]any{}
	for lignes.Next() {
		var (
			id, nom, description string
			prix, duree          int
			serviceLie           *string
			categorieLie         string
		)
		if err := lignes.Scan(&id, &nom, &description, &prix, &serviceLie, &categorieLie, &duree); err != nil {
			return nil, err
		}
		portee := "station"
		if serviceLie != nil && *serviceLie != "" {
			portee = "prestation"
		} else if categorieLie != "" {
			portee = "vehicule"
		}
		items = append(items, map[string]any{
			"id": id, "name": nom, "description": description, "price_xof": prix,
			"service_id": serviceLie, "category_code": categorieLie,
			"duration_minutes": duree, "scope": portee,
		})
	}
	return items, lignes.Err()
}
