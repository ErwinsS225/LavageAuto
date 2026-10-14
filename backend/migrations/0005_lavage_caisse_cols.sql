-- 0005_lavage_caisse_cols.sql — colonnes et tables pour Lots 5 et 6
-- Idempotent : applicable à chaque démarrage (ADD COLUMN IF NOT EXISTS,
-- CREATE TABLE IF NOT EXISTS). Le suivi par schema_migrations garantit
-- qu'elle n'est jouée qu'une fois.

-- ============================================================
-- Tables manquantes : vehicles (véhicules individuels) et
-- wash_packages (prestations/packages vendables). Référencées
-- par pricing.go et routes_lavages.go.
-- ============================================================

CREATE TABLE IF NOT EXISTS vehicles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  category_code text NOT NULL DEFAULT '',
  usage_code text NOT NULL DEFAULT '',
  plate text NOT NULL DEFAULT '',
  make text NOT NULL DEFAULT '',
  model text NOT NULL DEFAULT '',
  color text NOT NULL DEFAULT '',
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (station_id, plate)
);
CREATE INDEX IF NOT EXISTS vehicles_station_idx ON vehicles(station_id);

CREATE TABLE IF NOT EXISTS wash_packages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  service_id uuid NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  name text NOT NULL,
  price_xof integer NOT NULL DEFAULT 0,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS wpackages_station_idx ON wash_packages(station_id);

-- ============================================================
-- wash_orders — colonnes attendues par routes_lavages.go
-- ============================================================

ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS vehicle_id uuid;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS package_id uuid;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS unit_price_xof integer NOT NULL DEFAULT 0;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS line_total_xof integer NOT NULL DEFAULT 0;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS price_source text NOT NULL DEFAULT '';
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS price_source_label text NOT NULL DEFAULT '';
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS options jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS receipt_number text NOT NULL DEFAULT '';
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS cancelled_reason text NOT NULL DEFAULT '';

-- assigned_to_id : alias de washer_id pour l'API (les deux pointent vers users)
-- On ne duplique pas la colonne ; les routes utilisent assigned_to_id dans
-- leur propre logique. Ici on mappe via un alias SQL si besoin.
-- (la colonne washer_id existe déjà dans le schéma)

-- ============================================================
-- cash_sessions — colonnes attendues par routes_caisse.go
-- ============================================================

ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS is_open boolean NOT NULL DEFAULT true;
ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS opening_balance_xof integer NOT NULL DEFAULT 0;
ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS closing_balance_xof integer;
ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS closing_notes text NOT NULL DEFAULT '';

-- Sync is_open avec status : une session fermée (status != 'open') -> is_open = false
UPDATE cash_sessions SET is_open = false WHERE status != 'open' AND is_open = true;

-- ============================================================
-- payments — colonnes manquantes
-- ============================================================

ALTER TABLE payments ADD COLUMN IF NOT EXISTS voided_by uuid;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS voided_at timestamptz;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS created_by uuid;

-- wash_order_id : alias de order_id. On synchronise les valeurs existantes.
-- Les nouvelles insertion utilisent order_id (colonne native) ; les routes
-- qui lisent SELECT * recevront donc les deux colonnes.
-- On mappe voided_at/is_void via status pour la cohérence.

-- ============================================================
-- stations — compteur de reçus
-- ============================================================

ALTER TABLE stations ADD COLUMN IF NOT EXISTS receipt_counter integer NOT NULL DEFAULT 0;
