package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Onboarding d'une entreprise.
//
// Après son inscription, un propriétaire doit décrire sa station avant qu'elle
// puisse exploiter : identité, station, catalogue, tarifs, puis (lots suivants)
// horaires, catégories de véhicules, moyens de paiement, équipe, matériel.
//
// Trois principes tenus par ce fichier :
//
//  1. **Le serveur est la source de vérité.** Une étape est « faite » parce que
//     les données qui la composent existent réellement (une station, un
//     service tarifé…), jamais parce que le front a cliqué sur « Suivant ».
//     `onboarding_steps.data` ne sert qu'à retenir les choix à réafficher.
//  2. **La progression se calcule**, elle ne se déclare pas : `progress` est
//     dérivé des contrôles ci-dessous, à chaque lecture.
//  3. **Une station non activée ne vend pas** : `requireOnboarding` refuse les
//     écritures d'exploitation et indique les étapes à compléter.
//
// Le catalogue d'étapes grandit avec les lots de TODO-ONBOARDING.md : une étape
// n'est déclarée ici que lorsqu'elle est à la fois vérifiable en base **et**
// complétable depuis l'application (pas d'écran annoncé sans effet).
// ---------------------------------------------------------------------------

// onboardingStep décrit une étape du parcours.
type onboardingStep struct {
	Code        string // identifiant stable, stocké dans onboarding_steps.step_code
	Title       string // libellé affiché
	Description string // ce que l'étape recouvre, en une phrase
	Required    bool   // indispensable pour activer la station
	Screen      string // écran qui permet de la compléter (lien du front)
	Hint        string // ce qui manque quand l'étape n'est pas faite
	// Form : formulaire que le parcours affiche pour cette étape. Vide quand
	// l'étape est un constat (l'espace existe ou n'existe pas) — l'interface
	// n'a alors rien à saisir, seulement à montrer.
	Form string
}

// catalogueOnboarding : les étapes connues de cette version, dans l'ordre du
// parcours. Les lots 3 à 7 (horaires, véhicules, prestations détaillées,
// tarifs, caisse, équipe, matériel) viendront s'y ajouter — avec, chaque fois,
// le contrôle de données correspondant.
var catalogueOnboarding = []onboardingStep{
	{
		Code:  "espace",
		Title: "Espace de travail",
		Description: "Votre entreprise et sa première station, créées à l'inscription " +
			"(ou rejointes avec une invitation).",
		Required: true,
		Screen:   "",
		Hint:     "Aucune station n'est rattachée à l'entreprise.",
	},
	{
		Code:        "entreprise",
		Title:       "Identité de l'entreprise",
		Description: "Raison sociale, contacts, facturation et mentions administratives.",
		Required:    true,
		Screen:      "/parametres",
		Hint:        "Renseignez le nom, le téléphone et la ville de l'entreprise.",
		Form:        "entreprise",
	},
	{
		Code:        "station",
		Title:       "Identité et capacités de la station",
		Description: "Où elle se trouve, et ce qu'elle peut faire physiquement.",
		Required:    true,
		Screen:      "/parametres",
		Hint:        "Renseignez le nom et la ville de la station.",
		Form:        "station",
	},
	{
		Code:        "horaires",
		Title:       "Horaires d'ouverture",
		Description: "Quand la station reçoit des véhicules — c'est ce qui permet d'afficher « ouvert » ou « fermé ».",
		Required:    true,
		Screen:      "/parametres",
		Hint:        "Aucune journée d'ouverture n'est déclarée.",
		Form:        "horaires",
	},
	{
		Code:        "categories",
		Title:       "Catégories de véhicules",
		Description: "Ce que la station accepte de laver : citadine, berline, moto, gbaka, camion…",
		Required:    true,
		Screen:      "/onboarding",
		Hint:        "Aucune catégorie de véhicule n'est activée pour la station.",
		Form:        "vehicules",
	},
	{
		Code:  "usages",
		Title: "Usages commerciaux",
		Description: "Ce que le client fait de son véhicule — taxi, livraison, flotte… " +
			"Une berline « taxi compteur » reste une berline.",
		// Facultatif : un lavage se vend sans usage déclaré (ONB-8.2 ne le
		// compte pas parmi les prérequis d'activation). Il reste précieux pour
		// savoir qui lave quoi — mais il ne bloque pas l'ouverture.
		Required: false,
		Screen:   "/onboarding",
		Hint:     "Aucun usage commercial n'est activé, ou aucune combinaison véhicule × usage n'est déclarée.",
		Form:     "usages",
	},
	{
		Code:        "prestations",
		Title:       "Catalogue des prestations",
		Description: "Les lavages que la station propose, ce qu'ils comprennent, et pour quels véhicules.",
		Required:    true,
		Screen:      "/prestations",
		Hint:        "Aucune prestation active : ajoutez au moins un lavage au catalogue.",
		Form:        "prestations",
	},
	{
		Code:  "options",
		Title: "Options, formules et suppléments",
		Description: "Ce qui se facture en plus d'une prestation — tapis, cire, désinfection — " +
			"ou se vend en formule.",
		// Facultatif : une station peut vendre uniquement ses formules. Mais sans
		// option déclarée, la caisse n'en propose aucune — et n'en invente pas.
		Required: false,
		Screen:   "/prestations",
		Hint:     "Aucune option ni supplément n'est déclaré pour la station.",
		Form:     "options",
	},
	{
		Code:        "tarifs",
		Title:       "Grille tarifaire",
		Description: "Le prix de chaque prestation — par véhicule, par usage, ou au tarif du catalogue.",
		Required:    true,
		Screen:      "/prestations",
		Hint:        "Aucune prestation n'a de prix : renseignez la grille tarifaire de vos lavages.",
		Form:        "tarifs",
	},
	{
		Code:  "moyens",
		Title: "Moyens de paiement",
		Description: "Ce que la caisse accepte — espèces, Orange Money, Wave, carte… — " +
			"et ce que le reçu porte.",
		// Obligatoire : une caisse sans moyen actif ne peut rien encaisser. Le
		// référentiel du produit est posé à la création de la station (les
		// espèces sont actives d'emblée), donc l'étape est franchie par défaut —
		// elle ne bloque pas une inscription, mais elle ne peut pas être cochée
		// sans qu'un moyen soit réellement actif.
		Required: true,
		Screen:   "/parametres",
		Hint:     "Aucun moyen de paiement n'est actif : activez au moins les espèces.",
		Form:     "moyens",
	},
	{
		Code:  "equipe",
		Title: "Équipe",
		Description: "Qui travaille à la station — un compte par personne, avec sa fonction, " +
			"sa date d'arrivée et ses horaires.",
		// Facultatif à l'activation : une station qui démarre seule, tenue par
		// son propriétaire, est parfaitement viable. Le compte du propriétaire
		// existe déjà, l'étape est donc souvent franchie d'elle-même.
		Required: false,
		Screen:   "/employes",
		Hint:     "Aucun compte d'équipe : la station fonctionne avec votre seul accès.",
		Form:     "equipe",
	},
	{
		Code:  "materiel",
		Title: "Matériel et stock",
		Description: "Ce que la station possède (jets, aspirateurs, groupes) et ce qu'elle " +
			"consomme (produits, consommables) — avec la possibilité de dire « je ne sais pas encore ».",
		Required: false,
		Screen:   "/onboarding",
		Hint:     "Aucun matériel ni article de stock déclaré : l'écran propose le référentiel de la station.",
		Form:     "materiel",
	},
	{
		Code:  "commandes",
		Title: "Règles des commandes",
		Description: "Comment un véhicule circule dans la station : clients de passage, " +
			"laveur obligatoire, contrôle qualité, remise subordonnée au paiement, acomptes.",
		Required: false,
		Screen:   "/onboarding",
		Hint:     "Les règles par défaut s'appliquent : clients de passage acceptés, contrôle qualité facultatif, véhicule remis une fois la fiche soldée.",
		Form:     "commandes",
	},
}

// etapeParCode retrouve une étape du catalogue.
func etapeParCode(code string) (onboardingStep, bool) {
	for _, e := range catalogueOnboarding {
		if e.Code == code {
			return e, true
		}
	}
	return onboardingStep{}, false
}

// ---------------------------------------------------------------------------
// Contrôles de complétion : chaque étape est vérifiée sur les données réelles.
// ---------------------------------------------------------------------------

// pointOnboarding : état calculé d'une étape.
type pointOnboarding struct {
	Step onboardingStep
	Done bool
	// Detail : ce qui est constaté (nombre de prestations, de tarifs…) — sert
	// au récapitulatif et évite un « fait » sans justification.
	Detail string
}

// etapesOnboarding calcule l'état de chaque étape pour une entreprise.
func (a *App) etapesOnboarding(ctx context.Context, orgID string) ([]pointOnboarding, error) {
	if orgID == "" {
		return nil, nil
	}
	var (
		orgName, orgPhone, orgCity string
		stations                   int
		proprietaires              int
		equipe                     int
		equipementsDeclares        int
		articlesStock              int
		articlesNonComptes         int
		prestations                int
		prestationsTarifees        int
		optionsDeclarees           int
		nomStation, villeStation   string
		stationID                  string
		joursOuverts               int
		capacitesDeclarees         int
		localisationRenseignee     int
		categoriesActives          int
		usagesActifs               int
		couplesActifs              int
		moyensActifs               int
	)

	err := a.Pool.QueryRow(ctx, `
		SELECT name, phone, city,
		       (SELECT count(*) FROM stations s WHERE s.organization_id = o.id AND s.is_active),
		       (SELECT count(*) FROM users u WHERE u.organization_id = o.id AND u.role = 'owner' AND u.is_active)
		FROM organizations o WHERE o.id = $1`, orgID).
		Scan(&orgName, &orgPhone, &orgCity, &stations, &proprietaires)
	if err != nil {
		return nil, err
	}

	// La station de référence : la station par défaut si elle existe, sinon la
	// première — l'écran d'onboarding doit montrer un lieu précis.
	err = a.Pool.QueryRow(ctx, `
		SELECT id, name, city FROM stations
		WHERE organization_id = $1 AND is_active
		ORDER BY is_default DESC, created_at LIMIT 1`, orgID).Scan(&stationID, &nomStation, &villeStation)
	if err != nil && err.Error() != "no rows in result set" {
		return nil, err
	}

	// Les journées réellement décrites (et non « un jour existe en base ») :
	// un jour ouvert sans heures renseignées ne compte pas.
	if stationID != "" {
		_ = a.Pool.QueryRow(ctx, `
			SELECT count(*) FROM station_opening_hours
			WHERE station_id = $1 AND is_open
			  AND btrim(opens_at) <> '' AND btrim(closes_at) <> ''`, stationID).Scan(&joursOuverts)

		// Catégories et usages : ce que la station propose réellement. Le
		// référentiel du produit peut contenir trente entrées — ce qui compte
		// pour l'exploitation, c'est ce que **cette** station a activé.
		_ = a.Pool.QueryRow(ctx, `
			SELECT
			  (SELECT count(*) FROM station_vehicle_categories
			    WHERE station_id = $1 AND is_active),
			  (SELECT count(*) FROM station_vehicle_usages
			    WHERE station_id = $1 AND is_active),
			  (SELECT count(*) FROM station_vehicle_usage_categories
			    WHERE station_id = $1 AND is_active)`,
			stationID).Scan(&categoriesActives, &usagesActifs, &couplesActifs)

		// Moyens de paiement actifs : sans eux, la caisse ne peut rien encaisser.
		_ = a.Pool.QueryRow(ctx, `
			SELECT count(*) FROM payment_methods
			WHERE station_id = $1 AND is_active`, stationID).Scan(&moyensActifs)

		// La fiche de site et la localisation ne conditionnent pas l'activation
		// (elles ne se devinent pas et ne bloquent aucune vente), mais l'écran
		// doit dire qu'elles sont vides : sans ce constat, le formulaire des
		// capacités ne serait jamais ouvert.
		_ = a.Pool.QueryRow(ctx, `
			SELECT
			  (SELECT count(*) FROM station_site_profile p
			    WHERE p.station_id = $1 AND (
			      cardinality(p.wash_types) > 0
			      OR p.bays IS NOT NULL OR p.simultaneous_vehicles IS NOT NULL
			      OR p.has_vacuum IS NOT NULL OR p.has_drying IS NOT NULL
			      OR p.has_detailing IS NOT NULL OR p.has_waiting_room IS NOT NULL
			      OR p.has_storage IS NOT NULL OR p.has_generator IS NOT NULL
			      OR btrim(p.water_source) <> '' OR p.water_reserve_liters IS NOT NULL
			      OR btrim(p.power_supply) <> '' OR btrim(p.notes) <> ''
			      OR btrim(p.constraints) <> '')),
			  (SELECT count(*) FROM stations s
			    WHERE s.id = $1 AND (
			      btrim(s.commune) <> '' OR btrim(s.district_area) <> ''
			      OR btrim(s.street) <> '' OR btrim(s.landmark) <> ''
			      OR s.latitude IS NOT NULL OR s.longitude IS NOT NULL))`,
			stationID).Scan(&capacitesDeclarees, &localisationRenseignee)
	}

	err = a.Pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (
		         WHERE s.is_free
		            OR (s.price_xof > 0
		                AND COALESCE(st.settings ->> 'allow_generic_price', 'true') <> 'false')
		            OR EXISTS (SELECT 1 FROM price_rules r
		                        WHERE r.service_id = s.id AND r.is_active
		                          AND (r.valid_from IS NULL OR r.valid_from <= CURRENT_DATE)
		                          AND (r.valid_to IS NULL OR r.valid_to >= CURRENT_DATE)))
		FROM services s
		LEFT JOIN stations st ON st.id = s.station_id
		WHERE s.organization_id = $1 AND s.is_active`,
		orgID).Scan(&prestations, &prestationsTarifees)
	if err != nil {
		return nil, err
	}

	// L'équipe : les comptes rattachés à l'entreprise, propriétaire compris
	// (c'est la première personne de l'équipe).
	err = a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM users
		WHERE organization_id = $1 AND is_active AND role <> 'superadmin'`, orgID).Scan(&equipe)
	if err != nil {
		return nil, err
	}

	// Matériel et stock de la station de référence. Un article « non compté »
	// compte comme déclaré : l'exploitant a bien répondu, sa réponse est
	// « je ne sais pas encore ».
	if stationID != "" {
		_ = a.Pool.QueryRow(ctx, `
			SELECT
			  (SELECT count(*) FROM equipments WHERE station_id = $1 AND is_active),
			  (SELECT count(*) FROM stock_items WHERE station_id = $1 AND is_active),
			  (SELECT count(*) FROM stock_items WHERE station_id = $1 AND is_active AND NOT quantity_known)`,
			stationID).Scan(&equipementsDeclares, &articlesStock, &articlesNonComptes)
	}

	// Les options et suppléments : facultatifs, mais l'étape doit dire ce qui
	// est réellement déclaré plutôt que d'afficher « terminée » sans rien.
	err = a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM service_options
		WHERE organization_id = $1 AND is_active`, orgID).Scan(&optionsDeclarees)
	if err != nil {
		return nil, err
	}

	// Les règles de commande de la station de référence : l'étape les résume
	// au lieu d'afficher « fait » sans dire lesquelles s'appliquent.
	reglesCommandesStation := reglesParDefaut()
	if stationID != "" {
		if regles, err := a.lireReglesCommandes(ctx, stationID); err == nil {
			reglesCommandesStation = regles
		}
	}

	points := make([]pointOnboarding, 0, len(catalogueOnboarding))
	for _, etape := range catalogueOnboarding {
		point := pointOnboarding{Step: etape}
		switch etape.Code {
		case "espace":
			point.Done = strings.TrimSpace(orgName) != "" && stations > 0 && proprietaires > 0
			point.Detail = plural(stations, "station", "stations")
		case "entreprise":
			point.Done = strings.TrimSpace(orgName) != "" &&
				strings.TrimSpace(orgPhone) != "" && strings.TrimSpace(orgCity) != ""
			point.Detail = constate(
				champ{libelle: "nom", valeur: orgName},
				champ{libelle: "téléphone", valeur: orgPhone},
				champ{libelle: "ville", valeur: orgCity},
			)
		case "station":
			// L'étape est satisfaite par le nom et la ville (l'inscription les
			// demande). Le constat signale en plus ce qui reste facultatif, pour
			// que « Terminée » ne laisse pas croire que tout est décrit.
			point.Done = strings.TrimSpace(nomStation) != "" && strings.TrimSpace(villeStation) != ""
			point.Detail = constate(
				champ{libelle: "nom", valeur: nomStation},
				champ{libelle: "ville", valeur: villeStation},
			)
			facultatif := make([]string, 0, 2)
			if capacitesDeclarees == 0 {
				facultatif = append(facultatif, "capacités")
			}
			if localisationRenseignee == 0 {
				facultatif = append(facultatif, "localisation")
			}
			if len(facultatif) > 0 {
				point.Detail += " — facultatif à compléter : " + strings.Join(facultatif, ", ")
			}
		case "horaires":
			point.Done = joursOuverts > 0
			if joursOuverts == 0 {
				point.Detail = "Aucune journée d'ouverture déclarée"
			} else {
				point.Detail = plural(joursOuverts, "jour d'ouverture", "jours d'ouverture")
			}
		case "categories":
			point.Done = categoriesActives > 0
			point.Detail = plural(categoriesActives, "catégorie active", "catégories actives")
		case "usages":
			// Deux conditions : des usages activés **et** au moins une
			// combinaison catégorie × usage déclarée. Un usage sans véhicule
			// associé ne serait proposable à aucune fiche de caisse.
			point.Done = usagesActifs > 0 && couplesActifs > 0
			if usagesActifs == 0 {
				point.Detail = "Aucun usage commercial activé"
			} else if couplesActifs == 0 {
				point.Detail = plural(usagesActifs, "usage activé", "usages activés") +
					" — aucune combinaison véhicule × usage déclarée"
			} else {
				point.Detail = plural(usagesActifs, "usage activé", "usages activés") +
					" · " + plural(couplesActifs, "combinaison", "combinaisons")
			}
		case "prestations":
			point.Done = prestations > 0
			point.Detail = plural(prestations, "prestation active", "prestations actives")
		case "options":
			point.Done = optionsDeclarees > 0
			point.Detail = plural(optionsDeclarees, "option déclarée", "options déclarées")
		case "tarifs":
			point.Done = prestationsTarifees > 0
			point.Detail = plural(prestationsTarifees, "prestation tarifée", "prestations tarifées")
		case "moyens":
			point.Done = moyensActifs > 0
			point.Detail = plural(moyensActifs, "moyen de paiement actif", "moyens de paiement actifs")
		case "equipe":
			point.Done = equipe > 0
			point.Detail = plural(equipe, "compte actif", "comptes actifs")
			if equipe <= 1 {
				point.Detail += " — vous seul pour l'instant"
			}
		case "materiel":
			point.Done = equipementsDeclares > 0 || articlesStock > 0
			morceaux := []string{}
			if equipementsDeclares > 0 {
				morceaux = append(morceaux, plural(equipementsDeclares, "équipement", "équipements"))
			}
			if articlesStock > 0 {
				texte := plural(articlesStock, "article de stock", "articles de stock")
				if articlesNonComptes > 0 && articlesNonComptes < articlesStock {
					texte += " (dont " + itoa(articlesNonComptes) + " non compté(s))"
				} else if articlesNonComptes == articlesStock {
					texte = plural(articlesStock, "article de stock non compté", "articles de stock non comptés")
				}
				morceaux = append(morceaux, texte)
			}
			if len(morceaux) == 0 {
				point.Detail = "Rien de déclaré : ni matériel, ni article de stock"
			} else {
				point.Detail = strings.Join(morceaux, " · ")
			}
		case "commandes":
			// L'étape est un choix, pas une obligation : les règles par défaut
			// ont toujours une valeur, donc l'étape est toujours satisfaite.
			// Ce qui compte, c'est de dire **lesquelles** s'appliquent.
			point.Done = true
			point.Detail = resumeReglesCommandes(reglesCommandesStation)
		}
		points = append(points, point)
	}
	return points, nil
}

// resumeReglesCommandes : les règles en vigueur, dites simplement, pour la
// ligne « Constaté » de l'étape. Une règle absente de ce résumé serait une
// règle que l'exploitant ne saurait pas voir.
func resumeReglesCommandes(regles reglesCommandes) string {
	morceaux := []string{}
	if regles.AllowWalkIn {
		morceaux = append(morceaux, "clients de passage acceptés")
	} else {
		morceaux = append(morceaux, "client enregistré obligatoire")
	}
	if regles.RequireWasher {
		morceaux = append(morceaux, "laveur obligatoire dès la création")
	}
	if regles.QualityCheckRequired {
		morceaux = append(morceaux, "contrôle qualité obligatoire")
	}
	if regles.ReleaseRequiresPayment {
		morceaux = append(morceaux, "véhicule remis une fois la fiche soldée")
	} else {
		morceaux = append(morceaux, "remise possible avant paiement")
	}
	if regles.AllowPartialPayments {
		morceaux = append(morceaux, "acomptes acceptés")
	} else {
		morceaux = append(morceaux, "règlement en une fois")
	}
	if regles.MaxOpenOrdersPerWasher > 0 {
		morceaux = append(morceaux, "au plus "+itoa(regles.MaxOpenOrdersPerWasher)+" fiche(s) par laveur")
	}
	return strings.Join(morceaux, " · ")
}

// champ : un libellé et sa valeur, pour la ligne « Constaté » de l'interface.
type champ struct {
	libelle string
	valeur  string
}

// constate décrit ce que l'utilisateur voit sous une étape : les valeurs
// renseignées quand tout l'est (« Abidjan · +225 … »), et sinon la liste de ce
// qu'il reste à saisir (« Manque : téléphone, ville »). Sans cela, la ligne
// afficherait « Abidjan » pour une étape incomplète — et ne dirait pas pourquoi.
func constate(champs ...champ) string {
	presents := make([]string, 0, len(champs))
	absents := make([]string, 0, len(champs))
	for _, c := range champs {
		if valeur := strings.TrimSpace(c.valeur); valeur == "" {
			absents = append(absents, c.libelle)
		} else {
			presents = append(presents, valeur)
		}
	}
	if len(absents) > 0 {
		return "Manque : " + strings.Join(absents, ", ")
	}
	return strings.Join(presents, " · ")
}

// plural : « 1 station », « 3 stations ».
func plural(n int, singulier, pluriel string) string {
	mot := singulier
	if n > 1 {
		mot = pluriel
	}
	return itoa(n) + " " + mot
}

// ---------------------------------------------------------------------------
// Lecture : état, progression, éléments obligatoires manquants.
// ---------------------------------------------------------------------------

// etatOnboarding lit l'état enregistré d'une entreprise.
func (a *App) etatOnboarding(ctx context.Context, orgID string) (string, error) {
	var etat string
	err := a.Pool.QueryRow(ctx,
		`SELECT onboarding_state FROM organizations WHERE id = $1`, orgID).Scan(&etat)
	if err != nil {
		return "", err
	}
	if etat == "" {
		etat = "pending"
	}
	return etat, nil
}

// donneesEtapes lit les données descriptives enregistrées (choix à réafficher).
func (a *App) donneesEtapes(ctx context.Context, orgID string) (map[string]map[string]any, map[string]string, error) {
	donnees := map[string]map[string]any{}
	etats := map[string]string{}
	lignes, err := a.Pool.Query(ctx, `
		SELECT step_code, state, data FROM onboarding_steps
		WHERE organization_id = $1`, orgID)
	if err != nil {
		return donnees, etats, err
	}
	defer lignes.Close()
	for lignes.Next() {
		var code, etat string
		var brut []byte
		if err := lignes.Scan(&code, &etat, &brut); err != nil {
			continue
		}
		etats[code] = etat
		contenu := map[string]any{}
		parseJSONB(brut, &contenu)
		donnees[code] = contenu
	}
	return donnees, etats, lignes.Err()
}

// payloadOnboarding construit la réponse de l'API : l'état, la progression
// calculée, chaque étape et ce qui manque pour activer.
func (a *App) payloadOnboarding(ctx context.Context, orgID string) (map[string]any, error) {
	etat, err := a.etatOnboarding(ctx, orgID)
	if err != nil {
		return nil, err
	}
	points, err := a.etapesOnboarding(ctx, orgID)
	if err != nil {
		return nil, err
	}
	donnees, enregistres, err := a.donneesEtapes(ctx, orgID)
	if err != nil {
		return nil, err
	}

	var faites, requises, requisesFaites int
	etapes := make([]map[string]any, 0, len(points))
	manquants := make([]map[string]any, 0, len(points))
	for _, point := range points {
		siFait := point.Done
		if point.Step.Required {
			requises++
			if siFait {
				requisesFaites++
			}
		}
		if siFait {
			faites++
		} else if point.Step.Required {
			manquants = append(manquants, map[string]any{
				"code":        point.Step.Code,
				"title":       point.Step.Title,
				"hint":        point.Step.Hint,
				"screen":      point.Step.Screen,
				"description": point.Step.Description,
			})
		}
		// L'état enregistré sert à distinguer « pas commencé » de « en cours » ;
		// il ne remplace jamais le contrôle de données pour dire « fait ».
		enregistre := enregistres[point.Step.Code]
		if enregistre == "" {
			enregistre = "not_started"
		}
		if siFait {
			enregistre = "completed"
		}
		etapes = append(etapes, map[string]any{
			"code":        point.Step.Code,
			"title":       point.Step.Title,
			"description": point.Step.Description,
			"required":    point.Step.Required,
			"done":        siFait,
			"state":       enregistre,
			"detail":      point.Detail,
			"screen":      point.Step.Screen,
			"hint":        point.Step.Hint,
			"form":        point.Step.Form,
			"data":        donnees[point.Step.Code],
		})
	}

	pourcent := 0
	if requises > 0 {
		pourcent = requisesFaites * 100 / requises
	}

	// Dates du parcours : vides tant que l'entreprise n'a ni commencé ni activé.
	var demarre, activeLe string
	if err := a.Pool.QueryRow(ctx, `
		SELECT COALESCE(onboarding_started_at::text, ''), COALESCE(onboarding_completed_at::text, '')
		FROM organizations WHERE id = $1`, orgID).Scan(&demarre, &activeLe); err != nil {
		return nil, err
	}

	payload := map[string]any{
		"organization_id": orgID,
		"state":           etat,
		"progress": map[string]any{
			"done":        requisesFaites,
			"total":       requises,
			"percent":     pourcent,
			"steps_done":  faites,
			"steps_total": len(points),
		},
		"steps":        etapes,
		"missing":      manquants,
		"can_activate": requisesFaites == requises && requises > 0,
		"started_at":   demarre,
		"activated_at": activeLe,
	}
	return payload, nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// GET /api/v1/onboarding — état du parcours pour l'entreprise de la session.
func (a *App) handleOnboardingGet(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil || user.OrganizationID == nil {
		writeError(w, http.StatusNotFound, "not_found", "Aucune entreprise associée à votre compte.", nil)
		return
	}
	payload, err := a.payloadOnboarding(r.Context(), *user.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la lecture de la configuration.", nil)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// PATCH /api/v1/onboarding/steps/{code} — enregistre les choix d'une étape.
//
// Les données enregistrées sont descriptives (par exemple le choix « créer mon
// entreprise » ou « rejoindre une invitation »). Elles ne rendent jamais
// l'étape « faite » : c'est le contrôle de données qui décide, à la lecture.
func (a *App) handleOnboardingStep(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	code := r.PathValue("code")
	etape, connu := etapeParCode(code)
	if !connu {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Étape inconnue : « "+code+" ».", map[string]string{"code": "Étape inconnue"})
		return
	}
	if user == nil || user.OrganizationID == nil {
		writeError(w, http.StatusNotFound, "not_found", "Aucune entreprise associée à votre compte.", nil)
		return
	}

	var body struct {
		Data  map[string]any `json:"data"`
		State string         `json:"state"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if body.State != "" && body.State != "in_progress" && body.State != "not_started" {
		// « completed » est calculé par le serveur : un front ne peut pas
		// déclarer une étape terminée.
		writeError(w, http.StatusBadRequest, "validation_error",
			"Un seul état peut être déclaré ici : « in_progress » ou « not_started » (l'état « fait » est calculé).",
			map[string]string{"state": "État non autorisé"})
		return
	}
	if body.Data == nil {
		body.Data = map[string]any{}
	}
	if len(body.Data) > 100 {
		writeError(w, http.StatusBadRequest, "validation_error", "Trop d'informations pour une seule étape.", nil)
		return
	}

	ctx := r.Context()
	encodes, _ := json.Marshal(body.Data)
	etapeEtat := orDefault(body.State, "in_progress")

	// La progression reste calculée : on ne stocke que les choix et l'horodatage.
	_, err := a.Pool.Exec(ctx, `
		INSERT INTO onboarding_steps (organization_id, step_code, state, data, updated_by, updated_at)
		VALUES ($1,$2,$3,$4::jsonb,$5, now())
		ON CONFLICT (organization_id, step_code) DO UPDATE
		   SET state = EXCLUDED.state,
		       data = onboarding_steps.data || EXCLUDED.data,
		       updated_by = EXCLUDED.updated_by,
		       updated_at = now()`,
		*user.OrganizationID, etape.Code, etapeEtat, string(encodes), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement de l'étape.", nil)
		return
	}

	// Première étape touchée : la configuration passe « en cours ».
	if _, err := a.Pool.Exec(ctx, `
		UPDATE organizations SET onboarding_started_at = COALESCE(onboarding_started_at, now()),
		       onboarding_state = CASE WHEN onboarding_state = 'pending' THEN 'in_progress' ELSE onboarding_state END
		 WHERE id = $1`, *user.OrganizationID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, *user.OrganizationID, "", user.FullName, "onboarding.step", "onboarding", etape.Code,
		map[string]any{"state": etapeEtat, "fields": clefsDe(body.Data)})

	payload, err := a.payloadOnboarding(ctx, *user.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la lecture de la configuration.", nil)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// POST /api/v1/onboarding/activate — active la station si tout est prêt.
//
// L'activation n'est jamais déclenchée par l'interface seule : le serveur
// revérifie chaque prérequis obligatoire et liste précisément ce qui manque.
func (a *App) handleOnboardingActivate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil || user.OrganizationID == nil {
		writeError(w, http.StatusNotFound, "not_found", "Aucune entreprise associée à votre compte.", nil)
		return
	}
	ctx := r.Context()
	points, err := a.etapesOnboarding(ctx, *user.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la vérification des prérequis.", nil)
		return
	}

	champs := map[string]string{}
	for _, point := range points {
		if point.Step.Required && !point.Done {
			champs[point.Step.Code] = point.Step.Hint
		}
	}
	if len(champs) > 0 {
		writeError(w, http.StatusConflict, "onboarding_incomplete",
			"Il reste des informations à renseigner avant d'activer la station.", champs)
		return
	}

	if _, err := a.Pool.Exec(ctx, `
		UPDATE organizations
		   SET onboarding_state = 'active',
		       onboarding_started_at = COALESCE(onboarding_started_at, now()),
		       onboarding_completed_at = now()
		 WHERE id = $1`, *user.OrganizationID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'activation.", nil)
		return
	}

	a.addAudit(ctx, *user.OrganizationID, "", user.FullName, "onboarding.activated", "organization",
		*user.OrganizationID, map[string]any{"steps": len(points)})

	payload, err := a.payloadOnboarding(ctx, *user.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la lecture de la configuration.", nil)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// clefsDe : les noms de champs d'un objet (journalisation lisible, sans
// recopier des valeurs qui peuvent être personnelles).
func clefsDe(objet map[string]any) []string {
	clefs := make([]string, 0, len(objet))
	for clef := range objet {
		clefs = append(clefs, clef)
	}
	sort.Strings(clefs)
	return clefs
}

// ---------------------------------------------------------------------------
// Refus des écritures d'exploitation tant que la station n'est pas activée.
// ---------------------------------------------------------------------------

// requireOnboarding encadre une route d'exploitation : tant que l'entreprise
// n'a pas activé sa station, l'action est refusée avec la liste des étapes à
// compléter. Sans ce garde, une station sans tarif ni moyen de paiement
// encaisserait des lavages — exactement ce que l'onboarding doit empêcher.
func (a *App) requireOnboarding(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		if user == nil || user.OrganizationID == nil {
			writeError(w, http.StatusForbidden, "forbidden", "Aucune entreprise associée à votre compte.", nil)
			return
		}
		etat, err := a.etatOnboarding(r.Context(), *user.OrganizationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if etat == "active" {
			next(w, r)
			return
		}

		champs := map[string]string{}
		if points, err := a.etapesOnboarding(r.Context(), *user.OrganizationID); err == nil {
			for _, point := range points {
				if point.Step.Required && !point.Done {
					champs[point.Step.Code] = point.Step.Hint
				}
			}
		}
		writeError(w, http.StatusConflict, "onboarding_incomplete",
			"La station n'est pas encore activée : terminez la configuration avant d'exploiter.", champs)
	}
}
