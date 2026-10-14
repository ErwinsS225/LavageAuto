-- 0011_equipment_stock — équipe décrite, matériel, stock, et cycle de vie d'une fiche.
--
-- Trois sujets dans une même migration parce qu'ils répondent à la même étape
-- du parcours (celle où l'exploitant décrit *comment* il travaille) :
--
--   1. **L'équipe** : un compte disait « qui peut se connecter ». Il ne disait
--      pas qui fait quoi, depuis quand, ni comment il est rémunéré. Les champs
--      ajoutés ici sont facultatifs : une station qui ne renseigne rien reste
--      utilisable, et un champ vide s'affiche « non renseigné » — jamais « 0 ».
--   2. **Le matériel et le stock** : deux référentiels (modèles d'équipements
--      et catégories de consommables) et trois tables (équipements, articles,
--      mouvements). Le point délicat est `quantity_known` : **« je ne sais pas
--      encore » n'est pas « zéro »**. Une station qui n'a pas compté son stock
--      ne doit pas voir « rupture » partout — elle doit voir « non renseigné ».
--   3. **Le cycle de vie d'une fiche** : le statut ne connaissait que
--      `pending → in_progress → completed` (et `canceled`). Une station qui
--      contrôle la qualité, qui met un véhicule de côté « prêt », ou qui a un
--      litige client n'avait aucun moyen de le dire. On élargit la liste sans
--      toucher aux valeurs existantes.

-- ---------------------------------------------------------------------------
-- 1. L'équipe : ce qu'un compte ne disait pas
-- ---------------------------------------------------------------------------

ALTER TABLE users ADD COLUMN IF NOT EXISTS job_title TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS hired_on DATE;
-- « Horaires » de la personne (texte libre : une équipe ivoirienne écrit aussi
-- bien « lun–sam 8h–18h » que « relève, 2 semaines sur 3 »). Aucun calcul de
-- planning n'est fait à partir de ce champ — il est descriptif, et l'écran le
-- dit.
ALTER TABLE users ADD COLUMN IF NOT EXISTS work_schedule TEXT NOT NULL DEFAULT '';
-- Situation RH, distincte de `is_active` (qui commande l'accès applicatif) :
-- une personne peut être en congé (accès conservé) ou partie (accès coupé).
ALTER TABLE users ADD COLUMN IF NOT EXISTS employment_status TEXT NOT NULL DEFAULT 'active';

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_employment_status_check') THEN
    ALTER TABLE users ADD CONSTRAINT users_employment_status_check
      CHECK (employment_status IN ('active', 'on_leave', 'departed'));
  END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 2. Modèles d'équipements : le référentiel produit
-- ---------------------------------------------------------------------------
-- Un modèle décrit un type de matériel (jet haute pression, aspirateur, pont,
-- compresseur…). La station s'en sert pour **ajouter** un équipement en trois
-- champs au lieu de dix — elle n'est jamais obligée de l'utiliser : un matériel
-- qui n'est pas dans la liste s'ajoute à la main (`template_code` NULL).

CREATE TABLE IF NOT EXISTS equipment_templates (
  code       TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  family     TEXT NOT NULL DEFAULT 'autre',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS equipments (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  template_code   TEXT REFERENCES equipment_templates(code) ON DELETE SET NULL,
  name            TEXT NOT NULL,
  family          TEXT NOT NULL DEFAULT 'autre',
  quantity        INTEGER NOT NULL DEFAULT 1 CHECK (quantity >= 0),
  -- État matériel : « neuf », « bon », « à réparer », « hors service ».
  state           TEXT NOT NULL DEFAULT 'bon',
  acquired_on     DATE,
  last_service_on DATE,
  next_service_on DATE,
  cost_xof        INTEGER NOT NULL DEFAULT 0 CHECK (cost_xof >= 0),
  notes           TEXT NOT NULL DEFAULT '',
  -- `is_active` = présent dans la station. Un équipement retiré est désactivé,
  -- jamais supprimé : son historique (dates, coût) reste consultable.
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_name TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT equipments_state_check CHECK (state IN ('neuf', 'bon', 'a_reparer', 'hors_service'))
);
CREATE INDEX IF NOT EXISTS idx_equipments_station
  ON equipments(station_id, is_active, name);

-- ---------------------------------------------------------------------------
-- 3. Catégories de consommables : amorcées par station
-- ---------------------------------------------------------------------------
-- Même choix que pour les moyens de paiement (0010) : le référentiel est posé
-- à la création de la station, et chaque ligne appartient à la station. Une
-- station peut donc renommer « Shampooing » en « Savon mousse » sans que le
-- voisin change.

CREATE TABLE IF NOT EXISTS stock_categories (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  code            TEXT NOT NULL,
  name            TEXT NOT NULL,
  sort_order      INTEGER NOT NULL DEFAULT 0,
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT stock_categories_code_unique UNIQUE (station_id, code)
);
CREATE INDEX IF NOT EXISTS idx_stock_categories_station
  ON stock_categories(station_id, is_active, sort_order);

-- ---------------------------------------------------------------------------
-- 4. Articles en stock
-- ---------------------------------------------------------------------------
-- `quantity_known` est la colonne qui compte :
--   - TRUE  : la quantité a été saisie ou comptée, les alertes de seuil parlent ;
--   - FALSE : personne n'a encore compté (« je ne sais pas encore »). La
--     quantité reste à 0 mais **n'est pas affichée comme nulle**, et aucune
--     alerte de rupture n'est produite, même si un seuil est renseigné.

CREATE TABLE IF NOT EXISTS stock_items (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  category_code   TEXT NOT NULL DEFAULT '',
  name            TEXT NOT NULL,
  internal_code   TEXT NOT NULL DEFAULT '',
  unit            TEXT NOT NULL DEFAULT 'unité',
  quantity_known  BOOLEAN NOT NULL DEFAULT FALSE,
  quantity        NUMERIC(12,3) NOT NULL DEFAULT 0,
  unit_cost_xof   INTEGER NOT NULL DEFAULT 0 CHECK (unit_cost_xof >= 0),
  -- Seuil d'alerte. NULL = aucun seuil déclaré (et non « seuil à zéro »).
  min_quantity    NUMERIC(12,3),
  supplier        TEXT NOT NULL DEFAULT '',
  location        TEXT NOT NULL DEFAULT '',
  expires_on      DATE,
  notes           TEXT NOT NULL DEFAULT '',
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_name TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_stock_items_station
  ON stock_items(station_id, is_active, name);

-- ---------------------------------------------------------------------------
-- 5. Mouvements de stock
-- ---------------------------------------------------------------------------
-- Un mouvement est un fait daté qui explique la quantité actuelle : sans lui,
-- un écart d'inventaire ne serait pas justifiable. Types :
--   entree     : réception d'une commande        (delta > 0)
--   sortie     : consommation sur une journée    (delta < 0)
--   inventaire : comptage physique               (counted = quantité comptée)
--   ajustement : correction justifiée            (delta ±)
--   perte      : casse, vol, produit périmé      (delta < 0)
CREATE TABLE IF NOT EXISTS stock_movements (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  item_id         TEXT NOT NULL REFERENCES stock_items(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,
  delta           NUMERIC(12,3) NOT NULL DEFAULT 0,
  counted         NUMERIC(12,3),
  -- Quantité telle qu'elle est **après** le mouvement, et si elle est connue.
  -- NULL quand l'article reste « non compté » : on ne fabrique pas un chiffre.
  quantity_after  NUMERIC(12,3),
  known_after     BOOLEAN NOT NULL DEFAULT FALSE,
  unit_cost_xof   INTEGER NOT NULL DEFAULT 0 CHECK (unit_cost_xof >= 0),
  reason          TEXT NOT NULL DEFAULT '',
  created_by      TEXT REFERENCES users(id) ON DELETE SET NULL,
  created_by_name TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT stock_movements_kind_check
    CHECK (kind IN ('entree', 'sortie', 'inventaire', 'ajustement', 'perte'))
);
CREATE INDEX IF NOT EXISTS idx_stock_movements_item
  ON stock_movements(item_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_stock_movements_station
  ON stock_movements(station_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- 6. Cycle de vie d'une fiche de lavage
-- ---------------------------------------------------------------------------
-- Élargissement de la contrainte d'origine, sans réécrire l'existant :
--   pending        reçue, en attente (la réception est l'acte de création)
--   in_progress    en cours de lavage
--   quality_check  contrôle qualité (nouveau)
--   ready          prête à être remise au client (nouveau)
--   completed      livrée et soldée
--   canceled       annulée, motif obligatoire
--   disputed       en litige avec le client (nouveau)
DO $$
BEGIN
  ALTER TABLE wash_orders DROP CONSTRAINT IF EXISTS wash_orders_status_check;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'wash_orders_status_check') THEN
    ALTER TABLE wash_orders ADD CONSTRAINT wash_orders_status_check
      CHECK (status IN ('pending', 'in_progress', 'quality_check', 'ready', 'completed', 'canceled', 'disputed'));
  END IF;
END $$;

-- Horodatages et motif des nouvelles étapes. Ils servent au récapitulatif et au
-- rapport « où en sont les véhicules » — pas à décorer.
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS quality_checked_at TIMESTAMPTZ;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS ready_at TIMESTAMPTZ;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS dispute_reason TEXT NOT NULL DEFAULT '';
