-- La station : où elle est, ce qu'elle peut faire, et quand elle ouvre.
--
-- Trois sujets distincts, trois objets :
--   * l'adresse et la position → colonnes sur `stations` (une station a une
--     seule localisation) ;
--   * les caractéristiques physiques → `station_site_profile`, une ligne par
--     station, avec des booléens **nullables** : « pas d'aspiration » et « on ne
--     sait pas encore » ne sont pas la même information ;
--   * les horaires → `station_opening_hours`, une ligne par jour, plus
--     `station_opening_exceptions` pour les journées particulières. Les anciens
--     horaires stockés en JSONB sont repris au passage.

ALTER TABLE stations ADD COLUMN IF NOT EXISTS country TEXT NOT NULL DEFAULT 'Côte d''Ivoire';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS district TEXT NOT NULL DEFAULT '';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS commune TEXT NOT NULL DEFAULT '';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS district_area TEXT NOT NULL DEFAULT '';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS street TEXT NOT NULL DEFAULT '';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS landmark TEXT NOT NULL DEFAULT '';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS latitude NUMERIC(9,6);
ALTER TABLE stations ADD COLUMN IF NOT EXISTS longitude NUMERIC(9,6);
ALTER TABLE stations ADD COLUMN IF NOT EXISTS billing_address TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS station_site_profile (
  station_id            TEXT PRIMARY KEY REFERENCES stations(id) ON DELETE CASCADE,
  -- Types de lavage pratiqués sur le site (lavage manuel, haute pression…).
  wash_types            TEXT[] NOT NULL DEFAULT '{}',
  bays                  INTEGER,           -- pistes de lavage déclarées
  simultaneous_vehicles INTEGER,           -- véhicules traités en même temps
  has_vacuum            BOOLEAN,
  has_drying            BOOLEAN,
  has_detailing         BOOLEAN,
  has_waiting_room      BOOLEAN,
  has_storage           BOOLEAN,
  water_source          TEXT NOT NULL DEFAULT '',
  water_reserve_liters  INTEGER,
  power_supply          TEXT NOT NULL DEFAULT '',
  has_generator         BOOLEAN,
  notes                 TEXT NOT NULL DEFAULT '',
  constraints           TEXT NOT NULL DEFAULT '',
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by            TEXT REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT station_site_profile_capacites_positives CHECK (
    (bays IS NULL OR bays >= 0) AND
    (simultaneous_vehicles IS NULL OR simultaneous_vehicles >= 0) AND
    (water_reserve_liters IS NULL OR water_reserve_liters >= 0)
  )
);

CREATE TABLE IF NOT EXISTS station_opening_hours (
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  -- 1 = lundi … 7 = dimanche (ISO-8601, comme le reste du produit).
  weekday         SMALLINT NOT NULL CHECK (weekday BETWEEN 1 AND 7),
  is_open         BOOLEAN NOT NULL DEFAULT FALSE,
  opens_at        TEXT NOT NULL DEFAULT '',
  closes_at       TEXT NOT NULL DEFAULT '',
  break_start     TEXT NOT NULL DEFAULT '',
  break_end       TEXT NOT NULL DEFAULT '',
  last_checkin_at TEXT NOT NULL DEFAULT '',
  booking_allowed BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (station_id, weekday)
);

CREATE TABLE IF NOT EXISTS station_opening_exceptions (
  id         TEXT PRIMARY KEY,
  station_id TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  day        DATE NOT NULL,
  is_open    BOOLEAN NOT NULL DEFAULT FALSE,
  opens_at   TEXT NOT NULL DEFAULT '',
  closes_at  TEXT NOT NULL DEFAULT '',
  label      TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Une seule exception par jour : deux règles le même jour seraient
  -- contradictoires, et l'interface ne saurait pas laquelle appliquer.
  UNIQUE (station_id, day)
);

CREATE INDEX IF NOT EXISTS idx_station_hours_station ON station_opening_hours(station_id);
CREATE INDEX IF NOT EXISTS idx_station_exceptions_station ON station_opening_exceptions(station_id);

-- Reprise des horaires existants (`stations.opening_hours` en JSONB, alimenté
-- par l'ancien écran Paramètres : { "lundi": {"ouverture":"08:00",
-- "fermeture":"18:00","ferme":false}, … }). Aucune donnée n'est inventée : seuls
-- les jours réellement décrits sont repris, les autres restent non définis.
INSERT INTO station_opening_hours (station_id, weekday, is_open, opens_at, closes_at)
SELECT s.id,
       CASE jour.cle
         WHEN 'lundi' THEN 1 WHEN 'mardi' THEN 2 WHEN 'mercredi' THEN 3
         WHEN 'jeudi' THEN 4 WHEN 'vendredi' THEN 5 WHEN 'samedi' THEN 6
         WHEN 'dimanche' THEN 7
       END,
       CASE WHEN COALESCE((s.opening_hours -> jour.cle ->> 'ferme')::boolean, false) THEN FALSE ELSE TRUE END,
       COALESCE(s.opening_hours -> jour.cle ->> 'ouverture', ''),
       COALESCE(s.opening_hours -> jour.cle ->> 'fermeture', '')
  FROM stations s
  CROSS JOIN (VALUES ('lundi'), ('mardi'), ('mercredi'), ('jeudi'),
                     ('vendredi'), ('samedi'), ('dimanche')) AS jour(cle)
 WHERE s.opening_hours ? jour.cle
   AND jsonb_typeof(s.opening_hours -> jour.cle) = 'object'
ON CONFLICT (station_id, weekday) DO NOTHING;
