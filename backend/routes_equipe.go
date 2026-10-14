package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// routes_equipe.go — Équipe et invitations REST (Lot 4).
//
// Contrat : docs/api.md § « Équipe » et § « Invitations » ; types front
// src/types/index.ts (Employee, Invitation, InvitationPreview) et appels
// src/features/employees/api.ts, src/features/invitations/api.ts.
// Schéma : tables `users`, `employees`, `invitations` de 0001_initial.sql.
//
// Deux règles produit structurantes :
//   - l'activité d'un employé (GET /employees/{id}/activity) est ce qu'il a
//     réellement produit : ses fiches de lavage, pas un journal interne ;
//   - la création d'employé rend un mot de passe provisoire affiché une
//     seule fois (must_change_password = true côté serveur).

// libellesRoles : libellés français des rôles, dans l'ordre du front
// (src/lib/constants.ts ROLES). Le serveur les fournit : le front affiche
// ce qu'il reçoit.
var libellesRoles = map[string]string{
	"owner":   "Propriétaire",
	"manager": "Gérant",
	"cashier": "Caissier",
	"washer":  "Laveur",
}

func libelleRole(role string) string {
	if l, ok := libellesRoles[role]; ok {
		return l
	}
	return role
}

// libellesEtatsInvitation : états affichés par l'écran Invitations.
var libellesEtatsInvitation = map[string]string{
	"pending":  "En attente",
	"accepted": "Acceptée",
	"expired":  "Expirée",
	"revoked":  "Révoquée",
}

// etatInvitation calcule l'état visible à la lecture : `state` est stocké,
// mais l'expiration se déduit de la date à chaque lecture — pas de tâche de
// fond, la lecture suffit, et rien n'est réécrit pour autant.
func etatInvitation(state string, expiresAt time.Time, acceptedAt, revokedAt *time.Time) string {
	if acceptedAt != nil {
		return "accepted"
	}
	if revokedAt != nil {
		return "revoked"
	}
	if state == "expired" {
		return "expired"
	}
	if time.Now().After(expiresAt) {
		return "expired"
	}
	if state != "" && state != "pending" {
		return state
	}
	return "pending"
}

// quotaEmployes : plafond du plan et employés actifs de l'entreprise —
// logique identique à lireAbonnement (routes_auth.go) : abonnement le plus
// récent. (max = 0 veut dire « plafond non connu » → pas de refus.)
func (a *App) quotaEmployes(ctx context.Context, organizationID string) (max int, actuels int) {
	_ = a.db.QueryRow(ctx, `
		SELECT p.max_employees FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		WHERE s.organization_id = $1 ORDER BY s.created_at DESC LIMIT 1`, organizationID).Scan(&max)
	_ = a.db.QueryRow(ctx,
		`SELECT count(*) FROM employees WHERE organization_id = $1 AND is_active`,
		organizationID).Scan(&actuels)
	return max, actuels
}

// motDePasseProvisoire : 16 caractères hex (jeterJeton, main.go) — à dicter
// une fois ; le compte est créé avec must_change_password = true.
func motDePasseProvisoire() string {
	return jeterJeton(8)
}

// hacherMotDePasse : bcrypt via pgcrypto, comme routes_auth.go — le serveur
// Go reste sans dépendance de cryptographie applicative.
func (a *App) hacherMotDePasse(ctx context.Context, motDePasse string) (string, error) {
	var hash string
	err := a.db.QueryRow(ctx,
		`SELECT crypt($1, gen_salt('bf', 10))`, motDePasse).Scan(&hash)
	return hash, err
}

// Employee répond au front (src/types/index.ts) : identité, barème, et les
// compteurs affichés par l'écran Employés. Les compteurs sont lus sur les
// vraies tables (wash_orders, payments) — vides tant que le Lot 5 n'est pas
// livré, donc 0, ce qui est juste.
type Employee struct {
	ID               string     `json:"id"`
	StationID        *string    `json:"station_id"`
	StationName      string     `json:"station_name,omitempty"`
	FullName         string     `json:"full_name"`
	Email            string     `json:"email"`
	Phone            string     `json:"phone"`
	Role             string     `json:"role"`
	RoleLabel        string     `json:"role_label"`
	JobTitle         string     `json:"job_title"`
	HiredOn          *time.Time `json:"hired_on,omitempty"`
	WorkSchedule     string     `json:"work_schedule"`
	EmploymentStatus string     `json:"employment_status"`
	IsActive         bool       `json:"is_active"`
	CreatedAt        time.Time  `json:"created_at"`
	LastLoginAt      *time.Time `json:"last_login_at"`

	// Barème propre à l'employé ; null = il suit celui de la station
	// (src/types/index.ts : aucune valeur n'est recopiée d'un côté à l'autre).
	CommissionMode       *string  `json:"commission_mode"`
	CommissionPerWashXOF *int     `json:"commission_per_wash_xof"`
	CommissionPercent    *float64 `json:"commission_percent"`

	// Production (voir en-tête) : fiches du jour, du mois, chiffre du mois,
	// encaissements du mois.
	WashesToday       int `json:"washes_today"`
	WashesMonth       int `json:"washes_month"`
	RevenueMonthXOF   int `json:"revenue_month_xof"`
	CollectedMonthXOF int `json:"collected_month_xof"`
}

// Invitation répond au front (types/index.ts Invitation) : l'état et son
// libellé sont calculés à la lecture (etatInvitation), jamais relus tels
// quels.
type Invitation struct {
	ID            string     `json:"id"`
	StationID     *string    `json:"station_id"`
	Email         string     `json:"email"`
	Phone         string     `json:"phone"`
	Role          string     `json:"role"`
	RoleLabel     string     `json:"role_label"`
	Code          string     `json:"code"`
	State         string     `json:"state"`
	StateLabel    string     `json:"state_label"`
	ExpiresAt     time.Time  `json:"expires_at"`
	AcceptedAt    *time.Time `json:"accepted_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	InvitedByName string     `json:"invited_by_name"`
}

// --- Requêtes ---------------------------------------------------------------

type CreerEmployeRequest struct {
	StationID            string   `json:"station_id"`
	FullName             string   `json:"full_name"`
	Email                string   `json:"email"`
	Phone                string   `json:"phone"`
	Role                 string   `json:"role"`
	JobTitle             string   `json:"job_title"`
	HiredOn              string   `json:"hired_on"`
	WorkSchedule         string   `json:"work_schedule"`
	EmploymentStatus     string   `json:"employment_status"`
	CommissionMode       *string  `json:"commission_mode"`
	CommissionPerWashXOF *int     `json:"commission_per_wash_xof"`
	CommissionPercent    *float64 `json:"commission_percent"`
}

type ModifierEmployeRequest struct {
	FullName             *string  `json:"full_name"`
	Phone                *string  `json:"phone"`
	Role                 *string  `json:"role"`
	StationID            *string  `json:"station_id"`
	JobTitle             *string  `json:"job_title"`
	HiredOn              *string  `json:"hired_on"`
	WorkSchedule         *string  `json:"work_schedule"`
	EmploymentStatus     *string  `json:"employment_status"`
	IsActive             *bool    `json:"is_active"`
	CommissionMode       *string  `json:"commission_mode"`
	CommissionPerWashXOF *int     `json:"commission_per_wash_xof"`
	CommissionPercent    *float64 `json:"commission_percent"`
}

type CreerInvitationRequest struct {
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Role         string `json:"role"`
	StationID    string `json:"station_id"`
	ValidityDays int    `json:"validity_days"`
}

// roleInvitable : un propriétaire ne se joint jamais par invitation (il est
// créé à l'inscription) — règle déjà posée par routes_auth.go.
func roleInvitable(role string) bool {
	return role == "manager" || role == "cashier" || role == "washer"
}

// --------------------------------------------------------------------------------
// Employés — lecture
// --------------------------------------------------------------------------------

// GET /employees/options — liste courte pour les formulaires (affectation
// d'un laveur, sélecteurs). Session suffit (docs/api.md) : n'importe quel
// rôle connecté peut alimenter un formulaire.
func (a *App) listerOptionsEmployes(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	stationID := strings.TrimSpace(r.URL.Query().Get("station_id"))
	args := []interface{}{*sess.OrganizationID}
	requete := `SELECT e.id, e.full_name, e.role FROM employees e
		WHERE e.organization_id = $1 AND e.is_active = true AND e.employment_status = 'active'`
	if stationID != "" {
		requete += ` AND e.station_id = $2`
		args = append(args, stationID)
	}
	requete += ` ORDER BY e.full_name`

	rows, err := a.db.Query(r.Context(), requete, args...)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les employés.")
		return
	}
	defer rows.Close()

	options := []map[string]string{}
	for rows.Next() {
		var id, nom, role string
		if err := rows.Scan(&id, &nom, &role); err == nil {
			options = append(options, map[string]string{
				"id": id, "full_name": nom, "role": role, "role_label": libelleRole(role),
			})
		}
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"options": options})
}

// lireProductionEmployes : compteurs du jour et du mois par employé, en
// deux requêtes groupées (pas de N+1). Hypothèses de liaison, à confirmer
// au Lot 5 : wash_orders.washer_id = employees.id (laveur affecté),
// payments.received_by = users.id de l'employé (celui qui encaisse). Les
// tables sont vides aujourd'hui : les compteurs rendent 0, ce qui est juste.
func (a *App) lireProductionEmployes(ctx context.Context, organizationID string) map[string]*Employee {
	parLaveur := map[string]*Employee{}
	lier := func(employeeID string) *Employee {
		if parLaveur[employeeID] == nil {
			parLaveur[employeeID] = &Employee{}
		}
		return parLaveur[employeeID]
	}

	rows, err := a.db.Query(ctx, `
		SELECT washer_id,
		       count(*) FILTER (WHERE created_at >= date_trunc('day', now())),
		       count(*) FILTER (WHERE created_at >= date_trunc('month', now())),
		       coalesce(sum(total_xof) FILTER (WHERE created_at >= date_trunc('month', now())), 0)
		FROM wash_orders
		WHERE organization_id = $1 AND washer_id IS NOT NULL
		GROUP BY washer_id`, organizationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var jour, mois, ca int
			if rows.Scan(&id, &jour, &mois, &ca) == nil {
				e := lier(id)
				e.WashesToday, e.WashesMonth, e.RevenueMonthXOF = jour, mois, ca
			}
		}
	}

	rows2, err := a.db.Query(ctx, `
		SELECT e.id, coalesce(sum(p.amount_xof), 0)
		FROM payments p
		JOIN employees e ON e.user_id = p.received_by
		WHERE p.organization_id = $1 AND p.status = 'valid'
		  AND p.occurred_at >= date_trunc('month', now())
		GROUP BY e.id`, organizationID)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var id string
			var encaisse int
			if rows2.Scan(&id, &encaisse) == nil {
				lier(id).CollectedMonthXOF = encaisse
			}
		}
	}
	return parLaveur
}

// GET /employees — liste de l'entreprise. Filtres : station_id et
// status = active | inactive | all (emploi du front EmployeesPage).
// Note : users n'a pas de colonne last_login_at — le champ front reste null
// tant que la table ne l'aura pas (aucune valeur inventée).
func (a *App) listerEmployes(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	organizationID := *sess.OrganizationID
	filtres := []string{"e.organization_id = $1"}
	args := []interface{}{organizationID}

	if stationID := strings.TrimSpace(r.URL.Query().Get("station_id")); stationID != "" {
		args = append(args, stationID)
		filtres = append(filtres, fmt.Sprintf("e.station_id = $%d", len(args)))
	}
	switch strings.TrimSpace(r.URL.Query().Get("status")) {
	case "active":
		filtres = append(filtres, "e.is_active = true")
	case "inactive":
		filtres = append(filtres, "e.is_active = false")
	}
	onde := strings.Join(filtres, " AND ")

	rows, err := a.db.Query(r.Context(), `
		SELECT e.id, e.station_id, st.name, e.full_name, e.email, e.phone, e.role,
		       e.job_title, e.hired_on, e.work_schedule, e.employment_status,
		       e.is_active, e.created_at,
		       e.commission_mode, e.commission_per_wash_xof, e.commission_percent
		FROM employees e
		LEFT JOIN stations st ON st.id = e.station_id
		WHERE `+onde+`
		ORDER BY e.is_active DESC, e.full_name`, args...)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les employés.")
		return
	}
	defer rows.Close()

	production := a.lireProductionEmployes(r.Context(), organizationID)
	items := []Employee{}
	for rows.Next() {
		var e Employee
		var stationID, nomStation, email, phone, jobTitle, workSchedule, statut *string
		var hiredOn *time.Time
		var commissionMode *string
		var commissionPerWash *int
		var commissionPercent *float64
		var role string
		if err := rows.Scan(&e.ID, &stationID, &nomStation, &e.FullName, &email, &phone, &role,
			&jobTitle, &hiredOn, &workSchedule, &statut,
			&e.IsActive, &e.CreatedAt,
			&commissionMode, &commissionPerWash, &commissionPercent); err != nil {
			continue
		}
		e.StationID, e.StationName = stationID, deref(nomStation)
		e.Email, e.Phone, e.Role = deref(email), deref(phone), role
		e.RoleLabel = libelleRole(role)
		e.JobTitle, e.WorkSchedule, e.EmploymentStatus = deref(jobTitle), deref(workSchedule), deref(statut)
		e.HiredOn = hiredOn
		// Le barème n'est pas un champ comme les autres : sans
		// employee:compensation, il reste masqué (jamais null-inventé,
		// simplement absent de la réponse).
		if sess.aPermission("employee:compensation") {
			e.CommissionMode = commissionMode
			e.CommissionPerWashXOF, e.CommissionPercent = commissionPerWash, commissionPercent
		}
		if p := production[e.ID]; p != nil {
			e.WashesToday = p.WashesToday
			e.WashesMonth = p.WashesMonth
			e.RevenueMonthXOF = p.RevenueMonthXOF
			e.CollectedMonthXOF = p.CollectedMonthXOF
		}
		items = append(items, e)
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

// deref : string pointer → valeur (les colonnes NOT NULL DEFAULT sont
// scannées en pointeur pour rester uniformes avec les nullable).
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GET /employees/{id}/activity?from=&to= — ce que l'employé a réellement
// produit : ses fiches de lavage et les totaux de la période (contrat
// EmployeeActivityResponse, features/employees/api.ts). C'est la raison
// d'être de ce module : un journal d'écritures internes ne dirait rien de
// la station.
func (a *App) lireActiviteEmploye(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	employeeID := cheminParam(r, "/api/v1/employees/")
	if employeeID == "" || employeeID == "options" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant d'employé invalide.")
		return
	}
	// Le routeur matche par préfixe : on n'accepte que le sous-chemin
	// /activity, pas n'importe quel identifiant isolé.
	if !strings.HasSuffix(r.URL.Path, "/activity") {
		repondreErreur(w, http.StatusNotFound, "not_found", "Ressource introuvable.")
		return
	}

	// Appartenance : l'employé doit vivre dans l'entreprise de la session.
	var nom string
	err := a.db.QueryRow(r.Context(),
		`SELECT full_name FROM employees WHERE id = $1 AND organization_id = $2`,
		employeeID, *sess.OrganizationID).Scan(&nom)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Employé introuvable.")
		return
	}

	// Période : from/to en YYYY-MM-DD, bornes incluses à minuit local
	// (défaut : le mois en cours, ce que l'écran demande à l'ouverture).
	debut := strings.TrimSpace(r.URL.Query().Get("from"))
	fin := strings.TrimSpace(r.URL.Query().Get("to"))
	if debut == "" {
		debut = time.Now().Format("2006-01-02")
	}
	if fin == "" {
		fin = time.Now().Format("2006-01-02")
	}
	dateDebut, errD1 := time.Parse("2006-01-02", debut)
	dateFin, errD2 := time.Parse("2006-01-02", fin)
	if errD1 != nil || errD2 != nil {
		ecrireValidation(w, map[string]string{"from": "Format de date attendu YYYY-MM-DD."})
		return
	}
	finIncluse := dateFin.Add(24 * time.Hour)

	// Fiches de la période (200 maximum : l'écran est un aperçu, pas un
	// export — les exports sont la tâche des rapports, Lot 6).
	rows, err := a.db.Query(r.Context(), `
		SELECT wo.id, wo.code, wo.created_at, COALESCE(c.name, ''),
		       wo.vehicle_category_name, wo.vehicle_plate, wo.status,
		       wo.total_xof, wo.due_xof
		FROM wash_orders wo
		LEFT JOIN customers c ON c.id = wo.customer_id
		WHERE wo.organization_id = $1 AND wo.washer_id = $2
		  AND wo.created_at >= $3 AND wo.created_at < $4
		ORDER BY wo.created_at DESC
		LIMIT 200`, *sess.OrganizationID, employeeID, dateDebut, finIncluse)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger l'activité.")
		return
	}
	defer rows.Close()

	orders := []map[string]interface{}{}
	var washCount, completed, canceled, revenue, discount int
	var idsFiches []string
	for rows.Next() {
		var id, code, categorie, plaque, statut string
		var cree time.Time
		var client string
		var total, du int
		if err := rows.Scan(&id, &code, &cree, &client, &categorie, &plaque, &statut, &total, &du); err != nil {
			continue
		}
		orders = append(orders, map[string]interface{}{
			"id": id, "reference": code, "created_at": cree.UTC().Format(time.RFC3339),
			"customer_name": client, "vehicle_category_name": categorie,
			"plate": plaque, "status": statut, "total_xof": total, "due_xof": du,
		})
		washCount++
		revenue += total
		switch statut {
		case "completed":
			completed++
		case "canceled":
			canceled++
		}
		idsFiches = append(idsFiches, id)
	}

	// Remises et encaissements de la période : deux agrégats séparés (les
	// remises vivent sur la fiche, les encaissements sur payments).
	var discountTotal int
	_ = a.db.QueryRow(r.Context(), `
		SELECT COALESCE(sum(wo.discount_xof), 0) FROM wash_orders wo
		WHERE wo.organization_id = $1 AND wo.washer_id = $2
		  AND wo.created_at >= $3 AND wo.created_at < $4`,
		*sess.OrganizationID, employeeID, dateDebut, finIncluse).Scan(&discountTotal)
	discount = discountTotal

	var collected int
	if len(idsFiches) > 0 {
		_ = a.db.QueryRow(r.Context(), `
			SELECT COALESCE(sum(p.amount_xof), 0) FROM payments p
			WHERE p.organization_id = $1 AND p.order_id = ANY($2) AND p.status = 'valid'`,
			*sess.OrganizationID, idsFiches).Scan(&collected)
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"employee": map[string]string{"id": employeeID, "full_name": nom},
		"range":    map[string]string{"from": debut, "to": fin},
		"totals": map[string]interface{}{
			"wash_count": washCount, "completed_count": completed,
			"canceled_count": canceled, "revenue_xof": revenue,
			"collected_xof": collected, "discount_xof": discount,
		},
		"orders": orders,
	})
}

// --------------------------------------------------------------------------------
// Employés — écritures (employee:manage)
// --------------------------------------------------------------------------------

// POST /employees — crée le compte (users) et la fiche (employees) dans la
// même transaction, avec un mot de passe provisoire rendu une seule fois.
// Refus : station étrangère 404, quota 403, station non activée 409.
func (a *App) creerEmploye(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	organizationID := *sess.OrganizationID
	if !a.stationActive(r.Context(), organizationID) {
		repondreErreur(w, http.StatusConflict, "onboarding_incomplete",
			"La station n'est pas encore activée : terminez la mise en route.")
		return
	}

	corps, ok := decoderCorps[CreerEmployeRequest](w, r)
	if !ok {
		return
	}

	champs := map[string]string{}
	if len(strings.TrimSpace(corps.FullName)) < 2 {
		champs["full_name"] = "Nom complet requis (2 caractères minimum)."
	}
	if !roleInvitable(corps.Role) {
		champs["role"] = "Rôle invalide : utilisez manager, cashier ou washer."
	}
	if corps.Email == "" && corps.Phone == "" {
		champs["email"] = "Indiquez un e-mail ou un téléphone : c'est la connexion du compte."
	}
	if corps.CommissionPercent != nil && (*corps.CommissionPercent < 0 || *corps.CommissionPercent > 100) {
		champs["commission_percent"] = "Le pourcentage doit être compris entre 0 et 100."
	}
	if corps.CommissionPerWashXOF != nil && *corps.CommissionPerWashXOF < 0 {
		champs["commission_per_wash_xof"] = "Le montant ne peut pas être négatif."
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}

	// Station : celle du formulaire, sinon celle de la session. Station
	// d'une autre entreprise → 404, jamais d'indice sur son existence.
	stationID := strings.TrimSpace(corps.StationID)
	if stationID == "" && sess.StationID != nil {
		stationID = *sess.StationID
	}
	if stationID == "" {
		ecrireValidation(w, map[string]string{"station_id": "Station requise."})
		return
	}
	if err := a.verifierAppartenanceStation(r.Context(), stationID, organizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	if max, actuels := a.quotaEmployes(r.Context(), organizationID); max > 0 && actuels >= max {
		repondreErreur(w, http.StatusForbidden, "plan_limit",
			fmt.Sprintf("Quota d'employés atteint (%d/%d) : passez à une formule supérieure.", actuels, max))
		return
	}

	var hiredOn *time.Time
	if brut := strings.TrimSpace(corps.HiredOn); brut != "" {
		d, err := time.Parse("2006-01-02", brut)
		if err != nil {
			ecrireValidation(w, map[string]string{"hired_on": "Format de date attendu YYYY-MM-DD."})
			return
		}
		hiredOn = &d
	}
	statut := corps.EmploymentStatus
	if statut == "" {
		statut = "active"
	}
	if statut != "active" && statut != "on_leave" && statut != "departed" {
		ecrireValidation(w, map[string]string{"employment_status": "Statut invalide."})
		return
	}

	email := strings.ToLower(strings.TrimSpace(corps.Email))
	motDePasse := motDePasseProvisoire()
	hash, err := a.hacherMotDePasse(r.Context(), motDePasse)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de préparer le compte.")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Base indisponible.")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	// users.email est UNIQUE — un e-mail déjà pris est traduit en champ
	// lisible plutôt qu'en 500.
	var userID string
	var emailSQL interface{}
	if email != "" {
		emailSQL = email
	}
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (organization_id, station_id, full_name, email, phone, role,
		                   password_hash, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true)
		RETURNING id`,
		organizationID, stationID, corps.FullName, emailSQL, corps.Phone, corps.Role, hash).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			ecrireValidation(w, map[string]string{"email": "Un compte utilise déjà cet e-mail."})
			return
		}
		repondreErreur(w, http.StatusInternalServerError, "creation_failed",
			fmt.Sprintf("Impossible de créer le compte : %v", err))
		return
	}

	var employeeID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO employees (organization_id, station_id, user_id, full_name, email, phone,
		                       role, job_title, hired_on, work_schedule, employment_status,
		                       commission_mode, commission_per_wash_xof, commission_percent, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, true)
		RETURNING id`,
		organizationID, stationID, userID, corps.FullName, email, corps.Phone,
		corps.Role, corps.JobTitle, hiredOn, corps.WorkSchedule, statut,
		corps.CommissionMode, corps.CommissionPerWashXOF, corps.CommissionPercent).Scan(&employeeID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "creation_failed",
			fmt.Sprintf("Impossible de créer la fiche : %v", err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "creation_failed", "Enregistrement interrompu.")
		return
	}

	station := stationID
	a.journaliser(r.Context(), &organizationID, &station, &sess.UserID,
		"employee_created", map[string]interface{}{"employee_id": employeeID, "role": corps.Role})

	ecrireJSON(w, http.StatusCreated, map[string]interface{}{
		"id": employeeID, "email": email, "temporary_password": motDePasse,
	})
}

// PATCH /employees/{id} — « champ présent = champ modifié ». Le barème
// (commission_*) exige employee:compensation : un gérant ne tarife pas la
// paie d'un collègue (règle déjà posée côté front, EmployeesPage).
func (a *App) modifierEmploye(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	employeeID := cheminParam(r, "/api/v1/employees/")
	if employeeID == "" || employeeID == "options" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant d'employé invalide.")
		return
	}

	corps, ok := decoderCorps[ModifierEmployeRequest](w, r)
	if !ok {
		return
	}

	var stationActuelle *string
	err := a.db.QueryRow(r.Context(),
		`SELECT station_id FROM employees WHERE id = $1 AND organization_id = $2`,
		employeeID, *sess.OrganizationID).Scan(&stationActuelle)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Employé introuvable.")
		return
	}

	if (corps.CommissionMode != nil || corps.CommissionPerWashXOF != nil || corps.CommissionPercent != nil) &&
		!sess.aPermission("employee:compensation") {
		repondreErreur(w, http.StatusForbidden, "forbidden",
			"Permission employee:compensation requise pour modifier la rémunération.")
		return
	}
	champs := map[string]string{}
	if corps.FullName != nil && len(strings.TrimSpace(*corps.FullName)) < 2 {
		champs["full_name"] = "Nom complet requis (2 caractères minimum)."
	}
	if corps.Role != nil && !roleInvitable(*corps.Role) {
		champs["role"] = "Rôle invalide : utilisez manager, cashier ou washer."
	}
	if corps.CommissionPercent != nil && (*corps.CommissionPercent < 0 || *corps.CommissionPercent > 100) {
		champs["commission_percent"] = "Le pourcentage doit être compris entre 0 et 100."
	}
	if corps.CommissionPerWashXOF != nil && *corps.CommissionPerWashXOF < 0 {
		champs["commission_per_wash_xof"] = "Le montant ne peut pas être négatif."
	}
	if corps.EmploymentStatus != nil &&
		*corps.EmploymentStatus != "active" && *corps.EmploymentStatus != "on_leave" &&
		*corps.EmploymentStatus != "departed" {
		champs["employment_status"] = "Statut invalide."
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}
	a.appliquerModificationEmploye(w, r, sess, employeeID, stationActuelle, corps)
}

// appliquerModificationEmploye écrit les champs présents, tient le miroir
// du compte users (nom, téléphone, rôle, activation — l'e-mail reste
// l'identifiant de connexion et n'est pas modifié ici), journalise, puis
// relit l'employé pour répondre avec l'état réel.
func (a *App) appliquerModificationEmploye(w http.ResponseWriter, r *http.Request,
	sess *sessionUtilisateur, employeeID string, stationActuelle *string, corps *ModifierEmployeRequest) {

	modifications := []string{}
	valeurs := []interface{}{}
	ajouter := func(colonne string, valeur interface{}) {
		valeurs = append(valeurs, valeur)
		modifications = append(modifications, fmt.Sprintf("%s = $%d", colonne, len(valeurs)))
	}
	if corps.FullName != nil {
		ajouter("full_name", strings.TrimSpace(*corps.FullName))
	}
	if corps.Phone != nil {
		ajouter("phone", strings.TrimSpace(*corps.Phone))
	}
	if corps.JobTitle != nil {
		ajouter("job_title", strings.TrimSpace(*corps.JobTitle))
	}
	if corps.WorkSchedule != nil {
		ajouter("work_schedule", *corps.WorkSchedule)
	}
	if corps.EmploymentStatus != nil {
		ajouter("employment_status", *corps.EmploymentStatus)
	}
	if corps.IsActive != nil {
		ajouter("is_active", *corps.IsActive)
	}
	if corps.CommissionMode != nil {
		ajouter("commission_mode", *corps.CommissionMode)
	}
	if corps.CommissionPerWashXOF != nil {
		ajouter("commission_per_wash_xof", *corps.CommissionPerWashXOF)
	}
	if corps.CommissionPercent != nil {
		ajouter("commission_percent", *corps.CommissionPercent)
	}
	if corps.HiredOn != nil && strings.TrimSpace(*corps.HiredOn) != "" {
		d, err := time.Parse("2006-01-02", strings.TrimSpace(*corps.HiredOn))
		if err != nil {
			ecrireValidation(w, map[string]string{"hired_on": "Format de date attendu YYYY-MM-DD."})
			return
		}
		ajouter("hired_on", d)
	}
	if corps.Role != nil {
		ajouter("role", *corps.Role)
	}

	// Changement de station = nouvelle appartenance : revérifiée.
	if corps.StationID != nil && *corps.StationID != "" && *corps.StationID != deref(stationActuelle) {
		if err := a.verifierAppartenanceStation(r.Context(), *corps.StationID, *sess.OrganizationID); err != nil {
			repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
			return
		}
		ajouter("station_id", *corps.StationID)
	}
	if len(modifications) == 0 {
		ecrireValidation(w, map[string]string{"body": "Aucun champ à modifier."})
		return
	}

	valeurs = append(valeurs, employeeID)
	requete := fmt.Sprintf("UPDATE employees SET %s WHERE id = $%d",
		strings.Join(modifications, ", "), len(valeurs))
	if _, err := a.db.Exec(r.Context(), requete, valeurs...); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "update_failed",
			fmt.Sprintf("Impossible de mettre à jour : %v", err))
		return
	}

	// Miroir users.
	var compteID *string
	_ = a.db.QueryRow(r.Context(),
		`SELECT user_id FROM employees WHERE id = $1`, employeeID).Scan(&compteID)
	if compteID != nil {
		miroir := []string{}
		argsM := []interface{}{}
		ajouterMiroir := func(colonne string, valeur interface{}) {
			argsM = append(argsM, valeur)
			miroir = append(miroir, fmt.Sprintf("%s = $%d", colonne, len(argsM)))
		}
		if corps.FullName != nil {
			ajouterMiroir("full_name", strings.TrimSpace(*corps.FullName))
		}
		if corps.Phone != nil {
			ajouterMiroir("phone", strings.TrimSpace(*corps.Phone))
		}
		if corps.Role != nil {
			ajouterMiroir("role", *corps.Role)
		}
		if corps.IsActive != nil {
			ajouterMiroir("is_active", *corps.IsActive)
		}
		if corps.StationID != nil && *corps.StationID != "" {
			ajouterMiroir("station_id", *corps.StationID)
		}
		if len(miroir) > 0 {
			argsM = append(argsM, *compteID)
			_, _ = a.db.Exec(r.Context(),
				fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", strings.Join(miroir, ", "), len(argsM)), argsM...)
		}
	}

	a.journaliser(r.Context(), sess.OrganizationID, stationActuelle, &sess.UserID,
		"employee_updated", map[string]interface{}{"employee_id": employeeID})

	// Réponse : l'état réel relu (le corps PATCH peut être partiel).
	a.repondreEmploye(w, r, sess, employeeID)
}

// repondreEmploye relit une fiche et la renvoie au format Employee (avec
// ses compteurs). Évite de renvoyer le corps reçu, qui peut être partiel.
func (a *App) repondreEmploye(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur, employeeID string) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	var e Employee
	var stationID, nomStation, email, phone, jobTitle, workSchedule, statut *string
	var hiredOn *time.Time
	var commissionMode *string
	var commissionPerWash *int
	var commissionPercent *float64
	var role string
	err := a.db.QueryRow(r.Context(), `
		SELECT e.id, e.station_id, st.name, e.full_name, e.email, e.phone, e.role,
		       e.job_title, e.hired_on, e.work_schedule, e.employment_status,
		       e.is_active, e.created_at,
		       e.commission_mode, e.commission_per_wash_xof, e.commission_percent
		FROM employees e
		LEFT JOIN stations st ON st.id = e.station_id
		WHERE e.id = $1 AND e.organization_id = $2`,
		employeeID, *sess.OrganizationID).Scan(
		&e.ID, &stationID, &nomStation, &e.FullName, &email, &phone, &role,
		&jobTitle, &hiredOn, &workSchedule, &statut,
		&e.IsActive, &e.CreatedAt,
		&commissionMode, &commissionPerWash, &commissionPercent)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Employé introuvable.")
		return
	}
	e.StationID, e.StationName = stationID, deref(nomStation)
	e.Email, e.Phone, e.Role = deref(email), deref(phone), role
	e.RoleLabel = libelleRole(role)
	e.JobTitle, e.WorkSchedule, e.EmploymentStatus = deref(jobTitle), deref(workSchedule), deref(statut)
	e.HiredOn = hiredOn
	if sess.aPermission("employee:compensation") {
		e.CommissionMode = commissionMode
		e.CommissionPerWashXOF, e.CommissionPercent = commissionPerWash, commissionPercent
	}
	if p := a.lireProductionEmployes(r.Context(), *sess.OrganizationID)[e.ID]; p != nil {
		e.WashesToday, e.WashesMonth = p.WashesToday, p.WashesMonth
		e.RevenueMonthXOF, e.CollectedMonthXOF = p.RevenueMonthXOF, p.CollectedMonthXOF
	}
	ecrireJSON(w, http.StatusOK, e)
}

// POST /employees/{id}/reset-password — le propriétaire (ou le gérant)
// réinitialise le mot de passe depuis l'écran Employés : c'est la seule
// voie réelle de récupération (README : aucun e-mail n'est envoyé).
// Le front n'envoie aucun corps : il est facultatif.
func (a *App) reinitialiserMotDePasseEmploye(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	employeeID := cheminParam(r, "/api/v1/employees/")
	if employeeID == "" || employeeID == "options" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant d'employé invalide.")
		return
	}
	// Le routeur matche par préfixe : on n'accepte que le sous-chemin
	// /reset-password.
	if !strings.HasSuffix(r.URL.Path, "/reset-password") {
		repondreErreur(w, http.StatusNotFound, "not_found", "Ressource introuvable.")
		return
	}

	var email, userID *string
	err := a.db.QueryRow(r.Context(), `
		SELECT e.email, e.user_id FROM employees e
		WHERE e.id = $1 AND e.organization_id = $2`,
		employeeID, *sess.OrganizationID).Scan(&email, &userID)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Employé introuvable.")
		return
	}

	motDePasse := motDePasseProvisoire()
	hash, err := a.hacherMotDePasse(r.Context(), motDePasse)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de préparer le compte.")
		return
	}

	if userID == nil {
		// Employé sans compte rattaché (ancien import) : on crée le compte
		// avec l'e-mail de sa fiche.
		if deref(email) == "" {
			ecrireValidation(w, map[string]string{"email": "Cet employé n'a pas d'e-mail : ajoutez-en un avant de créer son compte."})
			return
		}
		var stationID *string
		var nom, role string
		_ = a.db.QueryRow(r.Context(),
			`SELECT station_id, full_name, role FROM employees WHERE id = $1`,
			employeeID).Scan(&stationID, &nom, &role)
		err := a.db.QueryRow(r.Context(), `
			INSERT INTO users (organization_id, station_id, full_name, email, phone, role,
			                   password_hash, must_change_password)
			VALUES ($1, $2, $3, $4, '', $5, $6, true)
			RETURNING id`,
			*sess.OrganizationID, stationID, nom, deref(email), role, hash).Scan(&userID)
		if err != nil {
			repondreErreur(w, http.StatusInternalServerError, "creation_failed",
				fmt.Sprintf("Impossible de créer le compte : %v", err))
			return
		}
		_, _ = a.db.Exec(r.Context(),
			`UPDATE employees SET user_id = $1 WHERE id = $2`, *userID, employeeID)
	} else {
		if _, err := a.db.Exec(r.Context(), `
			UPDATE users SET password_hash = $1, must_change_password = true WHERE id = $2`,
			hash, *userID); err != nil {
			repondreErreur(w, http.StatusInternalServerError, "update_failed", "Réinitialisation impossible.")
			return
		}
	}

	a.journaliser(r.Context(), sess.OrganizationID, nil, &sess.UserID,
		"employee_password_reset", map[string]interface{}{"employee_id": employeeID})

	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"id": employeeID, "email": deref(email), "temporary_password": motDePasse,
	})
}

// --------------------------------------------------------------------------------
// Invitations (docs/api.md § « Invitations »)
// --------------------------------------------------------------------------------
// Deux identifiants désignent la même invitation : un `code` de 16
// caractères hexadécimaux à dicter au téléphone, et le `token` long dont
// seul le sha256 (`token_hash`) est stocké. La réponse de création le rend
// une seule fois ; le code reste consultable.

// GET /invitations — liste de l'entreprise (le front n'envoie pas de
// station_id), avec état et libellés calculés à la lecture.
func (a *App) listerInvitations(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}

	rows, err := a.db.Query(r.Context(), `
		SELECT i.id, i.station_id, i.email, i.phone, i.role, i.code, i.state,
		       i.expires_at, i.accepted_at, i.revoked_at, i.created_at, u.full_name
		FROM invitations i
		LEFT JOIN users u ON u.id = i.invited_by
		WHERE i.organization_id = $1
		ORDER BY i.created_at DESC`, *sess.OrganizationID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les invitations.")
		return
	}
	defer rows.Close()

	items := []Invitation{}
	for rows.Next() {
		var inv Invitation
		var stationID, email, phone, nomInvitant *string
		var acceptedAt, revokedAt *time.Time
		var state string
		var code string
		if err := rows.Scan(&inv.ID, &stationID, &email, &phone, &inv.Role, &code, &state,
			&inv.ExpiresAt, &acceptedAt, &revokedAt, &inv.CreatedAt, &nomInvitant); err != nil {
			continue
		}
		inv.StationID, inv.Email, inv.Phone = stationID, deref(email), deref(phone)
		inv.RoleLabel, inv.Code = libelleRole(inv.Role), code
		inv.AcceptedAt, inv.RevokedAt = acceptedAt, revokedAt
		inv.State = etatInvitation(state, inv.ExpiresAt, acceptedAt, revokedAt)
		inv.StateLabel = libellesEtatsInvitation[inv.State]
		inv.InvitedByName = deref(nomInvitant)
		items = append(items, inv)
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

// POST /invitations — email OU téléphone, rôle invitables, station de
// l'entreprise si fournie, validity_days ≤ 90 (docs/api.md). Le jeton long
// n'existe nulle part en base : seul son sha256 est stocké.
func (a *App) creerInvitation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	organizationID := *sess.OrganizationID
	if !a.stationActive(r.Context(), organizationID) {
		repondreErreur(w, http.StatusConflict, "onboarding_incomplete",
			"La station n'est pas encore activée : terminez la mise en route.")
		return
	}

	corps, ok := decoderCorps[CreerInvitationRequest](w, r)
	if !ok {
		return
	}

	champs := map[string]string{}
	if corps.Email == "" && corps.Phone == "" {
		champs["email"] = "Indiquez un e-mail ou un téléphone : la personne a besoin d'un identifiant."
	}
	if !roleInvitable(corps.Role) {
		champs["role"] = "Rôle invalide : utilisez manager, cashier ou washer (jamais owner)."
	}
	duree := corps.ValidityDays
	if duree <= 0 {
		duree = 7
	}
	if duree > 90 {
		champs["validity_days"] = "La validité ne peut pas dépasser 90 jours."
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}

	var stationID *string
	if brut := strings.TrimSpace(corps.StationID); brut != "" {
		if err := a.verifierAppartenanceStation(r.Context(), brut, organizationID); err != nil {
			repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
			return
		}
		stationID = &brut
	}

	code := fmt.Sprintf("%x", time.Now().UnixNano())[:16]
	jeton := jeterJeton(32)
	var empreinte string
	if err := a.db.QueryRow(r.Context(),
		`SELECT encode(digest($1, 'sha256'), 'hex')`, jeton).Scan(&empreinte); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de préparer l'invitation.")
		return
	}
	expiration := time.Now().Add(time.Duration(duree) * 24 * time.Hour)

	var nouvelID string
	err := a.db.QueryRow(r.Context(), `
		INSERT INTO invitations (organization_id, station_id, invited_by, email, phone, role,
		                         code, token_hash, state, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', $9)
		RETURNING id`,
		organizationID, stationID, sess.UserID, corps.Email, corps.Phone,
		corps.Role, code, empreinte, expiration).Scan(&nouvelID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "creation_failed",
			fmt.Sprintf("Impossible de créer l'invitation : %v", err))
		return
	}

	a.journaliser(r.Context(), &organizationID, stationID, &sess.UserID,
		"invitation_created", map[string]interface{}{"invitation_id": nouvelID, "role": corps.Role})

	destinataire := corps.Email
	if destinataire == "" {
		destinataire = corps.Phone
	}
	ecrireJSON(w, http.StatusCreated, map[string]interface{}{
		"id": nouvelID, "code": code, "token": jeton, "role": corps.Role,
		"expires_at": expiration.UTC().Format(time.RFC3339),
		"message":    fmt.Sprintf("Invitation de %s créée pour %s.", libelleRole(corps.Role), destinataire),
		"notice": "Le lien complet ne s'affiche qu'une fois : il n'est conservé nulle part. " +
			"Le code, lui, reste consultable dans la liste.",
	})
}

// GET /invitations/{code} — aperçu PUBLIC lu par la page « Rejoindre »
// (l'invité n'a pas de compte). Le chemin porte le code court ou le jeton
// long (le front envoie jeton || code, JoinPage), et `?jeton=` reste
// accepté (docs/api.md). Ne renvoie AUCUN identifiant : un code inventé
// répond 404 sans rien révéler.
func (a *App) apercuInvitation(w http.ResponseWriter, r *http.Request, _ *sessionUtilisateur) {
	segment := cheminParam(r, "/api/v1/invitations/")
	if segment == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Lien d'invitation invalide.")
		return
	}
	jeton := strings.TrimSpace(r.URL.Query().Get("jeton"))
	if jeton == "" && len(segment) >= 40 {
		jeton = segment
	}

	// Par code d'abord (dicté au téléphone), par empreinte ensuite.
	var (
		nomEntreprise, ville, role, state string
		expiresAt                         time.Time
		acceptedAt, revokedAt             *time.Time
	)
	trouve := false
	err := a.db.QueryRow(r.Context(), `
		SELECT o.name, o.city, i.role, i.state, i.expires_at, i.accepted_at, i.revoked_at
		FROM invitations i JOIN organizations o ON o.id = i.organization_id
		WHERE i.code = $1`, segment).Scan(
		&nomEntreprise, &ville, &role, &state, &expiresAt, &acceptedAt, &revokedAt)
	if err == nil {
		trouve = true
	} else if jeton != "" {
		err = a.db.QueryRow(r.Context(), `
			SELECT o.name, o.city, i.role, i.state, i.expires_at, i.accepted_at, i.revoked_at
			FROM invitations i JOIN organizations o ON o.id = i.organization_id
			WHERE i.token_hash = encode(digest($1, 'sha256'), 'hex')`, jeton).Scan(
			&nomEntreprise, &ville, &role, &state, &expiresAt, &acceptedAt, &revokedAt)
		trouve = err == nil
	}
	if !trouve {
		repondreErreur(w, http.StatusNotFound, "invitation_unknown", "Cette invitation n'existe pas.")
		return
	}

	etat := etatInvitation(state, expiresAt, acceptedAt, revokedAt)
	utilisable := etat == "pending"
	message := ""
	switch etat {
	case "pending":
		message = fmt.Sprintf("%s vous invite comme %s. Créez votre compte pour rejoindre l'équipe.",
			nomEntreprise, libelleRole(role))
	case "accepted":
		message = "Cette invitation a déjà été utilisée : connectez-vous directement."
	case "revoked":
		message = "Cette invitation a été retirée : demandez-en une nouvelle."
	default:
		message = "Cette invitation a expiré : demandez-en une nouvelle."
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"organization_name": nomEntreprise, "city": ville,
		"role": role, "role_label": libelleRole(role),
		"state": etat, "state_label": libellesEtatsInvitation[etat],
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
		"usable":     utilisable, "message": message,
	})
}

// POST /invitations/{id}/revoke — passe l'état à revoked (jamais de
// suppression : l'historique reste lisible).
func (a *App) revoquerInvitation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if sess.OrganizationID == nil {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Session sans entreprise.")
		return
	}
	reste := cheminParam(r, "/api/v1/invitations/")
	invitationID := reste
	if idx := strings.Index(reste, "/"); idx >= 0 {
		invitationID = reste[:idx]
	}
	if invitationID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant d'invitation invalide.")
		return
	}

	var stationID *string
	err := a.db.QueryRow(r.Context(), `
		SELECT station_id FROM invitations WHERE id = $1 AND organization_id = $2`,
		invitationID, *sess.OrganizationID).Scan(&stationID)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Invitation introuvable.")
		return
	}

	if _, err := a.db.Exec(r.Context(),
		`UPDATE invitations SET state = 'revoked', revoked_at = NOW() WHERE id = $1`, invitationID); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "revoke_failed", "Impossible de révoquer l'invitation.")
		return
	}

	a.journaliser(r.Context(), sess.OrganizationID, stationID, &sess.UserID,
		"invitation_revoked", map[string]interface{}{"invitation_id": invitationID})

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"message": "Invitation révoquée."})
}
