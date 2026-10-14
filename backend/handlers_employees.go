package main

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Employés (options, liste, activité).
//
// Toutes les requêtes portent `organization_id`. Un caissier ne voit que les
// comptes de sa station : c'est le périmètre décrit dans docs/securite-acces.
// ---------------------------------------------------------------------------

// GET /employees/options — liste compacte pour les formulaires (affectation
// d'un laveur à un lavage). Ouverte à toute session, mais cloisonnée.
func (a *App) handleEmployeeOptions(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, full_name, role FROM users
		WHERE organization_id = $1 AND is_active
		  AND ($2 = '' OR station_id = $2 OR station_id IS NULL)
		ORDER BY full_name`, deref(user.OrganizationID), stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	options := []map[string]any{}
	for rows.Next() {
		var id, fullName, role string
		if err := rows.Scan(&id, &fullName, &role); err != nil {
			continue
		}
		options = append(options, map[string]any{
			"id": id, "full_name": fullName, "role": role, "role_label": roleLabelFR(role),
		})
	}
	// « options » est le nom attendu par l'interface ; « items » reste exposé
	// pour ne pas casser un appelant plus ancien.
	writeJSON(w, http.StatusOK, map[string]any{"options": options, "items": options, "total": len(options)})
}

func (a *App) handleEmployeesList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	monthStart := time.Now().Format("2006-01") + "-01"
	// Un gérant rattaché ne voit que sa station ; le propriétaire, toute
	// l'entreprise.
	stationFilter, err := a.stationFilter(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// La rémunération n'est lue que pour qui a le droit de la voir : les
	// colonnes ne sortent pas de la base inutilement.
	peutRemunerer := hasPermission(user, "employee:compensation")
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, full_name, email, phone, role, station_id, is_active, last_login_at, created_at,
		       job_title, hired_on::text, work_schedule, employment_status,
		       CASE WHEN $5 THEN commission_mode END,
		       CASE WHEN $5 THEN commission_per_wash_xof END,
		       CASE WHEN $5 THEN commission_percent END,
		       (SELECT count(*) FROM wash_orders w WHERE w.employee_id = u.id AND w.status = 'completed' AND w.business_day = $2) AS washes_today,
		       (SELECT count(*) FROM wash_orders w WHERE w.employee_id = u.id AND w.status = 'completed' AND w.business_day >= $3) AS washes_month,
		       (SELECT COALESCE(sum(total_xof),0) FROM wash_orders w WHERE w.employee_id = u.id AND w.status = 'completed' AND w.business_day >= $3) AS revenue_month,
		       (SELECT COALESCE(sum(p.amount_xof),0) FROM payments p
		         WHERE p.received_by = u.id AND p.status = 'valid' AND p.method <> 'differe'
		           AND p.business_day >= $3) AS collected_month
		FROM users u
		WHERE organization_id = $1
		  AND ($4 = '' OR station_id = $4)
		ORDER BY full_name`, deref(user.OrganizationID), businessDay(time.Now()), monthStart, stationFilter,
		peutRemunerer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, fullName, email, phone, role string
		var jobTitle, workSchedule, employmentStatus string
		var stationID *string
		var isActive bool
		var lastLogin, createdAt *time.Time
		var hiredOn *string
		var commMode *string
		var commPerWash *int
		var commPercent *float64
		var washesToday, washesMonth, revenueMonth, collectedMonth int
		if err := rows.Scan(&id, &fullName, &email, &phone, &role, &stationID, &isActive,
			&lastLogin, &createdAt, &jobTitle, &hiredOn, &workSchedule, &employmentStatus,
			&commMode, &commPerWash, &commPercent,
			&washesToday, &washesMonth, &revenueMonth, &collectedMonth); err != nil {
			continue
		}
		fiche := map[string]any{
			"id": id, "full_name": fullName, "email": email, "phone": phone,
			"role": role, "role_label": roleLabelFR(role),
			"station_id": stationID, "is_active": isActive, "last_login_at": lastLogin,
			"created_at":              createdAt,
			"job_title":               jobTitle,
			"hired_on":                hiredOn,
			"work_schedule":           workSchedule,
			"employment_status":       employmentStatus,
			"employment_status_label": nomSituationEmploye(employmentStatus),
			"washes_today":            washesToday,
			"washes_month":            washesMonth,
			"revenue_today_xof":       washesTodayRevenue(a, r, id),
			"revenue_month_xof":       revenueMonth,
			"collected_month_xof":     collectedMonth,
			// `compensation` est la seule porte d'entrée des données salariales :
			// elle est absente de la réponse (et non vide) sans la permission.
			"compensation": nil,
		}
		if peutRemunerer {
			mode := ""
			if commMode != nil {
				mode = *commMode
			}
			libelle := ""
			if mode != "" {
				libelle = libellesModeRemuneration[mode]
			}
			// Les champs à plat restent exposés : les écrans existants les
			// lisent déjà. `compensation` les regroupe et dit clairement ce que
			// l'appelant a le droit de voir.
			fiche["commission_mode"] = commMode
			fiche["commission_per_wash_xof"] = commPerWash
			fiche["commission_percent"] = commPercent
			fiche["compensation"] = map[string]any{
				"mode": mode, "mode_label": libelle,
				"per_wash_xof": commPerWash, "percent": commPercent,
			}
		}
		items = append(items, fiche)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
		"roles": []map[string]any{
			{"id": "manager", "label": "Gérant", "description": "Supervision quotidienne, caisse et rapports."},
			{"id": "cashier", "label": "Caissier", "description": "Enregistrement des lavages et encaissement."},
			{"id": "washer", "label": "Laveur", "description": "Prise en charge des véhicules et signalement des incidents."},
		},
		// Vocabulaire affiché par l'écran : situations d'un employé, modes de
		// rémunération, et si l'appelant peut voir la rémunération.
		"employment_statuses":   libellesSituationEmploye,
		"compensation_modes":    libellesModeRemuneration,
		"can_view_compensation": peutRemunerer,
	})
}

// washesTodayRevenue : chiffre d'affaires du jour attribué à un employé.
func washesTodayRevenue(a *App, r *http.Request, employeeID string) int {
	var revenue int
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(sum(total_xof),0) FROM wash_orders
		WHERE employee_id = $1 AND status = 'completed' AND business_day = $2`,
		employeeID, businessDay(time.Now())).Scan(&revenue)
	return revenue
}

// businessDay retourne la date AAAA-MM-JJ en heure d'Abidjan.
func businessDay(t time.Time) string {
	loc, err := time.LoadLocation("Africa/Abidjan")
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02")
}

// tempPassword génère un mot de passe provisoire de 10 caractères, sans
// caractères ambigus à recopier (pas de l/1/i/O/0).
func tempPassword() string {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789"
	out := make([]byte, 10)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			out[i] = chars[0]
			continue
		}
		out[i] = chars[n.Int64()]
	}
	return string(out)
}
