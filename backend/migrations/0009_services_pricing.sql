-- 0009 — Prestations, options et grille tarifaire.
--
-- Jusqu'ici une prestation portait **un** prix (`services.price_xof`). C'est
-- suffisant pour un lavage unique, ça ne l'est pas pour la réalité d'une
-- station : un lavage complet ne se vend pas au même prix pour une citadine et
-- pour un camion, ni pour un taxi qui revient tous les jours. Multiplier les
-- prestations (« Lavage complet citadine », « Lavage complet camion »…) pour
-- contourner le problème rend le catalogue illisible et fausse les rapports.
--
-- D'où quatre niveaux de prix, résolus par le serveur (`backend/pricing.go`),
-- du plus particulier au plus général :
--
--   1. **contrat client**   — un tarif négocié pour un client précis ;
--   2. **prestation + catégorie + usage** — « Lavage complet, berline, taxi » ;
--   3. **prestation + catégorie** — « Lavage complet, berline » ;
--   4. **tarif générique**  — `services.price_xof`, si la station l'autorise.
--
-- Le niveau retenu est **tracé** (`price_source`) et recopié sur la ligne de la
-- fiche : un écart de prix doit toujours pouvoir s'expliquer après coup.
--
-- Deux principes repris du reste du projet :
--
--   * **Prix absent ≠ prix zéro.** Une prestation sans tarif ne se vend pas
--     (`409 price_required`). Une prestation réellement gratuite le dit
--     explicitement (`is_free`).
--   * **Un modèle n'est pas un prix.** Le catalogue de référence
--     (`service_templates`) ne contient **aucun montant** : il propose des
--     prestations à tarifer, jamais des tarifs.

-- ---------------------------------------------------------------------------
-- Familles de prestations
-- ---------------------------------------------------------------------------

-- Modèles de familles, amorcés par le serveur (comme les véhicules).
CREATE TABLE IF NOT EXISTS service_family_templates (
  code        TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  sort_order  INTEGER NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ce qu'une station a activé. Elle peut renommer et réordonner : le référentiel
-- du produit n'est jamais modifié par une station.
CREATE TABLE IF NOT EXISTS service_families (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  code            TEXT NOT NULL,
  kind            TEXT NOT NULL DEFAULT 'modele',
  name            TEXT NOT NULL DEFAULT '',
  description     TEXT NOT NULL DEFAULT '',
  sort_order      INTEGER NOT NULL DEFAULT 0,
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (station_id, code),
  CONSTRAINT service_families_kind CHECK (kind IN ('modele', 'personnalisee'))
);

-- ---------------------------------------------------------------------------
-- Catalogue de référence des prestations
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS service_templates (
  code             TEXT PRIMARY KEY,
  name             TEXT NOT NULL,
  family_code      TEXT NOT NULL REFERENCES service_family_templates(code),
  description      TEXT NOT NULL DEFAULT '',
  -- Ce que la prestation comprend : la liste est affichée telle quelle.
  includes         TEXT[] NOT NULL DEFAULT '{}',
  duration_minutes INTEGER NOT NULL DEFAULT 30,
  equipment_notes  TEXT NOT NULL DEFAULT '',
  safety_notes     TEXT NOT NULL DEFAULT '',
  preconditions    TEXT NOT NULL DEFAULT '',
  -- Une prestation sur devis ne se vend pas au comptoir : elle demande un
  -- chiffrage (céramique, rénovation complète…).
  is_quote_only    BOOLEAN NOT NULL DEFAULT FALSE,
  -- Catégories de véhicules auxquelles ce modèle s'applique par défaut.
  category_codes   TEXT[] NOT NULL DEFAULT '{}',
  sort_order       INTEGER NOT NULL DEFAULT 0,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT service_templates_duree CHECK (duration_minutes >= 0)
);

-- ---------------------------------------------------------------------------
-- Colonnes ajoutées aux prestations d'une station
-- ---------------------------------------------------------------------------

ALTER TABLE services ADD COLUMN IF NOT EXISTS code TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS family_code TEXT NOT NULL DEFAULT '';
-- Provenance : quand la prestation vient d'un modèle, le code du modèle est
-- conservé pour savoir d'où elle sort (et ne pas recréer un doublon à
-- l'adoption suivante).
ALTER TABLE services ADD COLUMN IF NOT EXISTS template_code TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS includes TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE services ADD COLUMN IF NOT EXISTS equipment_notes TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS safety_notes TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS preconditions TEXT NOT NULL DEFAULT '';
-- « Gratuit » est une décision, pas un prix manquant : une prestation non
-- tarifée reste à `price_xof = 0` **et** `is_free = false`, et le serveur refuse
-- de la vendre (409 price_required).
ALTER TABLE services ADD COLUMN IF NOT EXISTS is_free BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE services ADD COLUMN IF NOT EXISTS is_quote_only BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_services_family ON services(station_id, family_code);

-- ---------------------------------------------------------------------------
-- Compatibilités : à quels véhicules une prestation s'applique
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS service_compatibilities (
  service_id      TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  category_code   TEXT NOT NULL,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (service_id, category_code)
);

CREATE INDEX IF NOT EXISTS idx_service_compatibilities_station
  ON service_compatibilities(station_id, category_code);

-- ---------------------------------------------------------------------------
-- Options et suppléments
-- ---------------------------------------------------------------------------

-- Une option se rattache à une prestation précise, ou à la station entière
-- (`service_id` nul : « lavage des tapis », valable sur n'importe quelle
-- prestation). Son prix est un **supplément unitaire**, ajouté à la ligne.
CREATE TABLE IF NOT EXISTS service_options (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  service_id      TEXT REFERENCES services(id) ON DELETE CASCADE,
  category_code   TEXT NOT NULL DEFAULT '',
  name            TEXT NOT NULL,
  description     TEXT NOT NULL DEFAULT '',
  price_xof       INTEGER NOT NULL DEFAULT 0,
  duration_minutes INTEGER NOT NULL DEFAULT 0,
  sort_order      INTEGER NOT NULL DEFAULT 0,
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT service_options_prix CHECK (price_xof >= 0)
);

CREATE INDEX IF NOT EXISTS idx_service_options_station ON service_options(station_id, service_id);

-- ---------------------------------------------------------------------------
-- Règles de prix
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS price_rules (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  service_id      TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  -- Discriminants facultatifs : vide = « ne se prononce pas sur ce critère ».
  category_code   TEXT NOT NULL DEFAULT '',
  usage_code      TEXT NOT NULL DEFAULT '',
  customer_id     TEXT REFERENCES customers(id) ON DELETE CASCADE,
  price_xof       INTEGER NOT NULL,
  -- Période de validité : un tarif de saison, une promotion encadrée.
  valid_from      DATE,
  valid_to        DATE,
  -- Motif obligatoire : un changement de prix doit pouvoir s'expliquer.
  reason          TEXT NOT NULL DEFAULT '',
  created_by      TEXT NOT NULL DEFAULT '',
  created_by_name TEXT NOT NULL DEFAULT '',
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT price_rules_montant CHECK (price_xof >= 0),
  CONSTRAINT price_rules_periode CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from)
);

-- **Unicité partielle** : deux règles actives strictement identiques sont un
-- conflit, pas un choix silencieux. Le serveur répond `409 price_rule_conflict`
-- et demande de désactiver l'ancienne.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_price_rules_active ON price_rules (
  station_id, service_id, COALESCE(category_code, ''), COALESCE(usage_code, ''),
  COALESCE(customer_id, '')
) WHERE is_active;

CREATE INDEX IF NOT EXISTS idx_price_rules_service ON price_rules(station_id, service_id);

-- ---------------------------------------------------------------------------
-- Traçabilité du prix sur les lignes d'une fiche
-- ---------------------------------------------------------------------------

ALTER TABLE wash_order_items ADD COLUMN IF NOT EXISTS price_source TEXT NOT NULL DEFAULT '';
-- Instantané des options retenues (nom et prix au moment de la vente) : si le
-- tarif d'une option change ensuite, la fiche reste explicable.
ALTER TABLE wash_order_items ADD COLUMN IF NOT EXISTS options JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE wash_order_items ADD COLUMN IF NOT EXISTS options_total_xof INTEGER NOT NULL DEFAULT 0;
