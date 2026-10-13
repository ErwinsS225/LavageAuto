package main

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Sessions de caisse : lecture et sérialisation.
// ---------------------------------------------------------------------------

const cashSessionColumns = `id, organization_id, station_id, station_name, status,
	opened_by, opened_by_name, opened_at, opening_float_xof, expected_cash_xof,
	counted_cash_xof, difference_xof, cash_in_xof, mobile_money_xof, other_in_xof,
	expenses_xof, operations, payment_count, expense_count, notes, close_notes,
	reopen_count, business_day, closed_by, closed_by_name, closed_at`

// scanCashSession lit une session dont la requête expose organization_id
// en dernière colonne.
func scanCashSession(row scannable) (*CashSession, error) {
	s, _, err := scanCashSessionWithOrg(row)
	return s, err
}

// scanCashSessionWithOrg renvoie aussi l'organisation propriétaire de la session.
func scanCashSessionWithOrg(row scannable) (*CashSession, string, error) {
	var s CashSession
	var orgID string
	err := row.Scan(&s.ID, &orgID, &s.StationID, &s.StationName, &s.Status,
		&s.OpenedBy, &s.OpenedByName, &s.OpenedAt, &s.OpeningFloatXOF, &s.ExpectedCashXOF,
		&s.CountedCashXOF, &s.DifferenceXOF, &s.CashInXOF, &s.MobileMoneyXOF, &s.OtherInXOF,
		&s.ExpensesXOF, &s.Operations, &s.PaymentCount, &s.ExpenseCount, &s.Notes, &s.CloseNotes,
		&s.ReopenCount, &s.BusinessDay, &s.ClosedBy, &s.ClosedByName, &s.ClosedAt)
	if err != nil {
		return nil, "", err
	}
	s.OrganizationID = orgID
	return &s, orgID, nil
}

// cashSessionToJSON sérialise la session en normalisant les horodatages.
func cashSessionToJSON(s *CashSession) map[string]any {
	if s == nil {
		return nil
	}
	return map[string]any{
		"id":                s.ID,
		"organization_id":   s.OrganizationID,
		"station_id":        s.StationID,
		"station_name":      s.StationName,
		"status":            s.Status,
		"opened_by":         s.OpenedBy,
		"opened_by_name":    s.OpenedByName,
		"opened_at":         s.OpenedAt.Format(time.RFC3339),
		"opening_float_xof": s.OpeningFloatXOF,
		"expected_cash_xof": s.ExpectedCashXOF,
		"counted_cash_xof":  s.CountedCashXOF,
		"difference_xof":    s.DifferenceXOF,
		"cash_in_xof":       s.CashInXOF,
		"mobile_money_xof":  s.MobileMoneyXOF,
		"other_in_xof":      s.OtherInXOF,
		"expenses_xof":      s.ExpensesXOF,
		"operations":        s.Operations,
		"payment_count":     s.PaymentCount,
		"expense_count":     s.ExpenseCount,
		"notes":             s.Notes,
		"close_notes":       s.CloseNotes,
		"reopen_count":      s.ReopenCount,
		"business_day":      s.BusinessDay,
		"closed_by":         s.ClosedBy,
		"closed_by_name":    s.ClosedByName,
		"closed_at":         formatTimePtr(s.ClosedAt),
	}
}

// getOwnedCashSession charge une session et vérifie son organisation.
func (a *App) getOwnedCashSession(ctx context.Context, id, orgID string) (*CashSession, error) {
	row := a.Pool.QueryRow(ctx,
		`SELECT `+cashSessionColumns+` FROM cash_sessions WHERE id = $1`, id)
	s, actualOrg, err := scanCashSessionWithOrg(row)
	if err != nil {
		return nil, err
	}
	if orgID != "" && actualOrg != orgID {
		return nil, errNotFound
	}
	return s, nil
}
