-- 0008 — Catégories de véhicules et usages commerciaux.
--
-- Deux référentiels **distincts**, parce qu'ils ne répondent pas à la même
-- question :
--
--   * **la catégorie** décrit le véhicule (une berline, un gbaka, une moto) ;
--   * **l'usage** décrit ce qu'on en fait (voiture personnelle, taxi compteur,
--     livraison…).
--
-- Une berline « taxi compteur » reste une berline : mélanger les deux dans une
-- seule liste obligerait à créer « Berline taxi » et « Berline personnelle », et
-- la caisse ne saurait plus dire combien de berlines elle a lavées.
--
-- Les deux niveaux :
--   * `vehicle_category_templates` / `vehicle_usage_templates` : le catalogue de
--     référence du produit (amorcé par le serveur, rejouable sans doublon) ;
--   * `station_vehicle_categories` / `station_vehicle_usages` : ce que **cette**
--     station propose, avec son nom local, son ordre et son activation.
--
-- Une station n'écrit jamais dans le référentiel : elle l'active. Désactiver ne
-- supprime rien — les fiches passées gardent le nom qu'elles portaient
-- (`wash_orders.vehicle_category_name`), et l'historique reste lisible même si
-- la catégorie n'existe plus.

CREATE TABLE IF NOT EXISTS vehicle_category_templates (
  code             TEXT PRIMARY KEY,
  name             TEXT NOT NULL,
  -- Regroupement d'affichage (véhicules légers, deux-roues, poids lourds…).
  family           TEXT NOT NULL DEFAULT '',
  description      TEXT NOT NULL DEFAULT '',
  -- Gabarit tarifaire : à quelle famille de prix cette catégorie se rattache.
  pricing_class    TEXT NOT NULL DEFAULT '',
  -- Durée indicative d'un lavage, en minutes (0 = non précisée).
  duration_minutes INTEGER NOT NULL DEFAULT 0,
  aliases          TEXT[] NOT NULL DEFAULT '{}',
  sort_order       INTEGER NOT NULL DEFAULT 0,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT vehicle_category_templates_duree CHECK (duration_minutes >= 0)
);

CREATE TABLE IF NOT EXISTS vehicle_usage_templates (
  code        TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  family      TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  -- Tarifé comme un véhicule particulier, ou comme une prestation commerciale ?
  -- Sert au classement de la grille tarifaire (lot 5), pas à un prix.
  clientele   TEXT NOT NULL DEFAULT 'particulier',
  aliases     TEXT[] NOT NULL DEFAULT '{}',
  sort_order  INTEGER NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ce que la station propose réellement. `kind` distingue ce qui vient du
-- référentiel d'une catégorie créée par l'exploitant : sans cette distinction,
-- une catégorie personnalisée serait effacée à la prochaine mise à jour du
-- catalogue (et réciproquement, on ne saurait pas quoi proposer à l'écran).
CREATE TABLE IF NOT EXISTS station_vehicle_categories (
  station_id  TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  code        TEXT NOT NULL,
  kind        TEXT NOT NULL DEFAULT 'modele',
  -- Nom local : vide = celui du référentiel. Un exploitant peut appeler une
  -- « berline » une « voiture de ville » sans casser le référentiel commun.
  name        TEXT NOT NULL DEFAULT '',
  family      TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  aliases     TEXT[] NOT NULL DEFAULT '{}',
  pricing_class TEXT NOT NULL DEFAULT '',
  sort_order  INTEGER NOT NULL DEFAULT 0,
  is_active   BOOLEAN NOT NULL DEFAULT TRUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (station_id, code),
  CONSTRAINT station_vehicle_categories_kind CHECK (kind IN ('modele', 'personnalisee'))
);

CREATE TABLE IF NOT EXISTS station_vehicle_usages (
  station_id TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  code       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'modele',
  name       TEXT NOT NULL DEFAULT '',
  family     TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  aliases    TEXT[] NOT NULL DEFAULT '{}',
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_active  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (station_id, code),
  CONSTRAINT station_vehicle_usages_kind CHECK (kind IN ('modele', 'personnalisee'))
);

-- Les couples (catégorie, usage) que la station accepte réellement.
--
-- Une station qui lave des motos ne lave pas forcément des « motos de
-- livraison » : le couple dit ce qui est vendable. Volontairement **sans
-- prix** : la grille tarifaire par couple arrive au lot 5 (`price_rules`), et
-- inventer un prix ici serait contraire à la règle « aucun prix inventé ».
CREATE TABLE IF NOT EXISTS station_vehicle_usage_categories (
  station_id  TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  usage_code  TEXT NOT NULL,
  category_code TEXT NOT NULL,
  is_active   BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (station_id, usage_code, category_code)
);

-- La fiche de lavage conserve **les deux** noms : c'est ce qui permet de relire
-- une fiche ancienne après un renommage ou une désactivation.
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS vehicle_usage_id TEXT;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS vehicle_usage_name TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_station_vehicle_categories_station
  ON station_vehicle_categories(station_id);
CREATE INDEX IF NOT EXISTS idx_station_vehicle_usages_station
  ON station_vehicle_usages(station_id);
CREATE INDEX IF NOT EXISTS idx_station_vehicle_usage_categories_station
  ON station_vehicle_usage_categories(station_id);
CREATE INDEX IF NOT EXISTS idx_wash_orders_vehicle_usage ON wash_orders(vehicle_usage_id);

-- Reprise : l'ancienne table `vehicle_categories` était **globale** (sans
-- entreprise) et n'est plus lue par la caisse. Elle n'est pas supprimée — rien
-- n'est retiré sans que les données aient été recopiées. Si une installation y
-- avait saisi des libellés, ils deviennent des catégories personnalisées de
-- chaque station : aucun nom ne disparaît de l'interface. Sur une base où la
-- table est vide (cas de nos installations), cette instruction n'écrit rien.
INSERT INTO station_vehicle_categories (station_id, code, kind, name, sort_order, is_active)
SELECT s.id, 'perso_' || lower(vc.id), 'personnalisee', vc.name,
       COALESCE(vc.sort_order, 0), COALESCE(vc.is_active, true)
FROM vehicle_categories vc
CROSS JOIN stations s
WHERE btrim(vc.name) <> ''
ON CONFLICT (station_id, code) DO NOTHING;
