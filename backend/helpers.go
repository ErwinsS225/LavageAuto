package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// Helpers partagés : payload /auth/me, slug, JSONB.
// ---------------------------------------------------------------------------

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// strPtrOrNil renvoie un pointeur sur s, ou nil si s est vide.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// slugify transforme un nom en slug URL.
func slugify(s string) string {
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range lowerRunes(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
			prevDash = false
		} else if !prevDash && len(out) > 0 {
			out = append(out, '-')
			prevDash = true
		}
	}
	return string(out)
}

func lowerRunes(s string) []rune {
	rs := []rune(s)
	for i, r := range rs {
		if r >= 'A' && r <= 'Z' {
			rs[i] = r + 32
		}
	}
	return rs
}

// buildMePayload recompose la réponse /auth/me : user, organization, station,
// subscription (plan + usage), permissions.
func (a *App) buildMePayload(ctx context.Context, user *User) (map[string]any, error) {
	payload := map[string]any{
		"user": map[string]any{
			"id":              user.ID,
			"full_name":       user.FullName,
			"email":           user.Email,
			"phone":           user.Phone,
			"role":            user.Role,
			"role_label":      roleLabelFR(user.Role),
			"organization_id": deref(user.OrganizationID),
			"station_id":      deref(user.StationID),
		},
		"organization":         nil,
		"station":              nil,
		"subscription":         nil,
		"permissions":          rolePermissions[user.Role],
		"must_change_password": user.MustChangePassword,
	}

	if user.OrganizationID != nil {
		org, err := a.getOrganization(ctx, *user.OrganizationID)
		if err == nil {
			payload["organization"] = org
		}
	}
	stationID := deref(user.StationID)
	if stationID == "" && user.OrganizationID != nil {
		stationID, _ = a.defaultStationID(ctx, *user.OrganizationID)
	}
	if stationID != "" {
		st, err := a.getStation(ctx, stationID)
		if err == nil {
			payload["station"] = st
		}
	}

	if user.OrganizationID != nil {
		subPayload, err := a.buildSubscriptionPayload(ctx, *user.OrganizationID)
		if err == nil {
			payload["subscription"] = subPayload
		}
	}
	return payload, nil
}

// jsonOrNull sérialise une map en JSON pour les colonnes JSONB.
func jsonOrNull(m map[string]any) string {
	if m == nil {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// parseJSONB décode une colonne JSONB dans dest (tolère le vide).
func parseJSONB(raw []byte, dest any) {
	if len(raw) == 0 {
		return
	}
	_ = json.Unmarshal(raw, dest)
}

// setClause construit « colonne = $N » pour une clause SET de UPDATE.
func setClause(column string, n int) string {
	return fmt.Sprintf("%s = $%d", column, n)
}
