-- 0002_catalogue.sql — colonnes attendues par le front pour le catalogue,
-- les options et la grille tarifaire. Idempotent : applicable à chaque boot.

ALTER TABLE services            ADD COLUMN IF NOT EXISTS code text NOT NULL DEFAULT '';
ALTER TABLE services            ADD COLUMN IF NOT EXISTS template_code text NOT NULL DEFAULT '';
ALTER TABLE services            ADD COLUMN IF NOT EXISTS sort_order integer NOT NULL DEFAULT 0;
ALTER TABLE services            ADD COLUMN IF NOT EXISTS includes jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE services            ADD COLUMN IF NOT EXISTS equipment_notes text NOT NULL DEFAULT '';
ALTER TABLE services            ADD COLUMN IF NOT EXISTS safety_notes text NOT NULL DEFAULT '';
ALTER TABLE services            ADD COLUMN IF NOT EXISTS preconditions text NOT NULL DEFAULT '';
ALTER TABLE service_templates   ADD COLUMN IF NOT EXISTS includes jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE service_templates   ADD COLUMN IF NOT EXISTS equipment_notes text NOT NULL DEFAULT '';
ALTER TABLE service_templates   ADD COLUMN IF NOT EXISTS safety_notes text NOT NULL DEFAULT '';
ALTER TABLE service_templates   ADD COLUMN IF NOT EXISTS preconditions text NOT NULL DEFAULT '';
ALTER TABLE service_templates   ADD COLUMN IF NOT EXISTS is_quote_only boolean NOT NULL DEFAULT false;
ALTER TABLE service_options     ADD COLUMN IF NOT EXISTS duration_minutes integer NOT NULL DEFAULT 0;
ALTER TABLE service_options     ADD COLUMN IF NOT EXISTS sort_order integer NOT NULL DEFAULT 0;
ALTER TABLE price_rules         ADD COLUMN IF NOT EXISTS created_by uuid;

-- Table pour activer/désactiver des familles de services par station (Lot 2)
CREATE TABLE IF NOT EXISTS station_service_families (
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  family_code text NOT NULL,
  is_active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (station_id, family_code)
);

CREATE UNIQUE INDEX IF NOT EXISTS services_station_code_idx ON services (station_id, code) WHERE code <> '';