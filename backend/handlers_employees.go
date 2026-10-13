package main

import (
	"math/rand"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Employés (options, liste, activité).
// ---------------------------------------------------------------------------

func (a *App) handleEmployeeOptions(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, full_name, role FROM users
		WHERE organization_id = $1 AND is_active
		  AND ($2 = '' OR station_id = $2 OR station_id IS NULL)
		ORDER BY full_name`, user.OrganizationID, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, fullName, role string
		if err := rows.Scan(&id, &fullName, &role); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "full_name": fullName, "role": role, "role_label": roleLabelFR(role),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleEmployeesList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	today := businessDay(time.Now())
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, full_name, email, phone, role, station_id, is_active, last_login_at, created_at,
		       commission_mode, commission_per_wash_xof, commission_percent,
		       (SELECT count(*) FROM wash_orders w WHERE w.employee_id = u.id AND w.status = 'completed' AND w.business_day = $2) AS washes_today,
		       (SELECT count(*) FROM wash_orders w WHERE w.employee_id = u.id AND w.status = 'completed') AS washes_month,
		       (SELECT COALESCE(sum(total_xof),0) FROM wash_orders w WHERE w.employee_id = u.id AND w.status = 'completed' AND w.business_day = $2) AS revenue_today
		FROM users u
		WHERE organization_id = $1
		ORDER BY full_name`, user.OrganizationID, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, fullName, email, phone, role string
		var stationID *string
		var isActive bool
		var lastLogin, createdAt *time.Time
		var commMode *string
		var commPerWash *int
		var commPercent *float64
		var washesToday, washesMonth int
		var revenueToday int
		if err := rows.Scan(&id, &fullName, &email, &phone, &role, &stationID, &isActive,
			&lastLogin, &createdAt, &commMode, &commPerWash, &commPercent,
			&washesToday, &washesMonth, &revenueToday); err != nil {
			continue
		}
		var revenueMonth int
		_ = a.Pool.QueryRow(r.Context(), `
			SELECT COALESCE(sum(total_xof),0) FROM wash_orders
			WHERE employee_id = $1 AND status = 'completed'`, id).Scan(&revenueMonth)

		items = append(items, map[string]any{
			"id": id, "full_name": fullName, "email": email, "phone": phone,
			"role": role, "role_label": roleLabelFR(role),
			"station_id": stationID, "is_active": isActive, "last_login_at": lastLogin,
			"created_at":              createdAt,
			"washes_today":            washesToday,
			"washes_month":            washesMonth,
			"revenue_today_xof":       revenueToday,
			"revenue_month_xof":       revenueMonth,
			"collected_month_xof":     revenueMonth,
			"commission_mode":         commMode,
			"commission_per_wash_xof": commPerWash,
			"commission_percent":      commPercent,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
		"roles": []map[string]any{
			{"id": "owner", "label": "Propriétaire", "description": "Accès complet à la station et la facturation."},
			{"id": "manager", "label": "Gérant", "description": "Supervision quotidienne, caisse et rapports."},
			{"id": "cashier", "label": "Caissier", "description": "Enregistrement des lavages et encaissement."},
			{"id": "washer", "label": "Laveur", "description": "Prise en charge des véhicules et signalement des incidents."},
		},
	})
}

// businessDay retourne la date AAAA-MM-JJ en heure d'Abidjan.
func businessDay(t time.Time) string {
	loc, err := time.LoadLocation("Africa/Abidjan")
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02")
}

// tempPassword génère un mot de passe provisoire de 8 caractères.
func tempPassword() string {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789"
	out := make([]byte, 8)
	for i := range out {
		out[i] = chars[rand.Intn(len(chars))]
	}
	return string(out)
}
