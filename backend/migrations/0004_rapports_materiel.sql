-- 0004_rapports_materiel.sql — tables et colonnes pour Lots 6 et 7
-- Idempotent : applicable à chaque démarrage.

-- ============================================================
-- LOT 6 — Dépenses, rapports, commissions
-- ============================================================

-- Les tables expenses, employees (avec commission_*), payments, wash_orders
-- existent déjà dans 0001_initial.sql. Il manque seulement :
-- - Colonnes pour le suivi des exports (audit)
-- - Vue matérialisée pour le dashboard (optionnel, on calcule à la volée)

-- ============================================================
-- LOT 7 — Matériel, stock, règles de commandes
-- ============================================================

-- equipment, stock_items, stock_movements existent déjà dans 0001_initial.sql
-- Il faut ajouter :
-- - Table station_order_rules (réglages par station)
-- - Colonnes manquantes sur equipment si nécessaires

-- Table des règles de commandes par station (7 réglages)
CREATE TABLE IF NOT EXISTS station_order_rules (
  station_id uuid PRIMARY KEY REFERENCES stations(id) ON DELETE CASCADE,
  allow_walk_in boolean NOT NULL DEFAULT true,
  require_washer boolean NOT NULL DEFAULT false,
  max_open_orders_per_washer integer NOT NULL DEFAULT 0,
  quality_check_required boolean NOT NULL DEFAULT false,
  release_requires_payment boolean NOT NULL DEFAULT true,
  allow_partial_payments boolean NOT NULL DEFAULT true,
  require_cancel_reason boolean NOT NULL DEFAULT true,
  auto_close_on_payment boolean NOT NULL DEFAULT true,
  require_open_cash_session boolean NOT NULL DEFAULT true,
  max_cashier_discount_xof integer NOT NULL DEFAULT 0,
  default_float_xof integer NOT NULL DEFAULT 0,
  discrepancy_tolerance_xof integer NOT NULL DEFAULT 0,
  allowed_roles text[] NOT NULL DEFAULT ARRAY['owner','manager','cashier'],
  allow_void boolean NOT NULL DEFAULT true,
  receipt_prefix text NOT NULL DEFAULT 'WG',
  receipt_footer text NOT NULL DEFAULT '',
  receipt_legal_name text NOT NULL DEFAULT '',
  receipt_tax_id text NOT NULL DEFAULT '',
  receipt_logo_url text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid
);

-- Index pour audit_logs par action (déjà existant sur organization_id, created_at)
CREATE INDEX IF NOT EXISTS audit_action_idx ON audit_logs(action);

-- S'assurer que receipt_counters existe (déjà dans 0001)
-- CREATE TABLE IF NOT EXISTS receipt_counters ... déjà fait

-- Fonction update_updated_at_column si pas existante
-- DOIT être créée AVANT le trigger qui la référence (PostgreSQL valide l'existence
-- de la fonction à la création du trigger).
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'update_updated_at_column') THEN
    CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS trigger LANGUAGE plpgsql AS $$
    BEGIN
      NEW.updated_at = now();
      RETURN NEW;
    END;
    $$;
  END IF;
END $$;

-- Trigger pour updated_at sur station_order_rules
DROP TRIGGER IF EXISTS station_order_rules_updated_at ON station_order_rules;
CREATE TRIGGER station_order_rules_updated_at
  BEFORE UPDATE ON station_order_rules
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Amorçage des règles par défaut pour les stations existantes
INSERT INTO station_order_rules (station_id)
SELECT id FROM stations
ON CONFLICT (station_id) DO NOTHING;

-- Permissions pour les nouvelles routes (les permissions sont gérées côté code via rt.proteger)
-- Pas de migration nécessaire pour les permissions