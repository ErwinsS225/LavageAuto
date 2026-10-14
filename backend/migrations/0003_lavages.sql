-- 0003_lavages.sql — colonnes attendues par les fiches de lavage, la caisse
-- et les reçus (Lot 5). Idempotent : applicable à chaque démarrage.

-- Motif d'une remise : une remise se justifie, elle reste lisible sur la fiche.
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS discount_reason text NOT NULL DEFAULT '';

-- Session de caisse : qui l'a close, ses notes, et le nombre de réouvertures
-- (une caisse se rouvre, ça se compte).
ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS closed_by uuid;
ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS close_notes text NOT NULL DEFAULT '';
ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS reopen_count integer NOT NULL DEFAULT 0;

-- Index des encaissements d'une fiche (lecture du détail, totaux de période).
CREATE INDEX IF NOT EXISTS payments_order_idx ON payments(order_id) WHERE order_id IS NOT NULL;

-- Une seule règle active par (station, prestation, catégorie, usage) : c'est
-- la contrainte qui rend l'enregistrement de la grille idempotent (ON
-- CONFLICT). Les lignes désactivées (is_active = false) sont exclues, pour
-- pouvoir réactiver une combinaison sans doublon.
CREATE UNIQUE INDEX IF NOT EXISTS price_rules_cle_unique
  ON price_rules (station_id, service_id, category_code, usage_code) WHERE is_active;

-- Réglages par défaut de la station : sept réglages de fonctionnement des
-- commandes + onze réglages de caisse/reçus, tous lus là où le serveur décide.
-- Un réglage explicite déjà enregistré n'est jamais écrasé (jsonb || : les
-- clés existantes de `settings` gagnent).
UPDATE stations
SET settings = settings || '{
  "allow_walk_in": true,
  "require_washer": false,
  "max_open_orders_per_washer": 0,
  "quality_check_required": false,
  "release_requires_payment": true,
  "allow_partial_payments": true,
  "require_cancel_reason": true,
  "auto_close_on_payment": true,
  "require_open_cash_session": true,
  "max_cashier_discount_xof": 0,
  "default_float_xof": 0,
  "discrepancy_tolerance_xof": 0,
  "allowed_roles": ["owner","manager","cashier"],
  "allow_void": true,
  "receipt_prefix": "WG",
  "receipt_footer": "",
  "receipt_legal_name": "",
  "receipt_tax_id": "",
  "receipt_logo_url": ""
}'::jsonb
WHERE NOT (settings ? 'allow_walk_in');
