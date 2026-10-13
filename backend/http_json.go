package main

import (
	"encoding/json"
	"net/http"
)

// writeJSON sérialise une réponse JSON avec le code HTTP donné.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError adopte la forme d'erreur du serveur Express :
// { "error": { "code": "...", "message": "...", "fields": {...} } }
func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	errObj := map[string]any{"code": code, "message": message}
	if len(fields) > 0 {
		errObj["fields"] = fields
	}
	writeJSON(w, status, map[string]any{"error": errObj})
}

// decodeBody lit le corps JSON d'une requête dans dest.
func decodeBody(r *http.Request, dest any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dest)
}
