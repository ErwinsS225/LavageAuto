-- 0010_payment_methods — moyens de paiement de la station et identité des reçus.
--
-- Pourquoi une table plutôt qu'une liste figée dans le code : une station
-- ivoirienne n'accepte pas « mobile money » en général, mais Orange Money, MTN
-- MoMo, Moov Money, Wave… Chaque moyen a sa **famille**, et c'est la famille —
-- pas le libellé — qui décide du compteur de caisse alimenté. Renommer « Orange
-- Money » en « OM » ne change donc aucun total.
--
-- La colonne `payments.method` reste en place : elle porte la famille canonique
-- (`especes`, `mobile_money`, `carte`, `virement`, `differe`, `autre`), ce qui
-- évite de réécrire l'historique. Le moyen précis est cité en plus, par
-- identifiant **et** par libellé recopié — une contrepassation reste lisible
-- même si le moyen est renommé ou désactivé plus tard.
--
-- `payments.payment_method_id` est nullable : les encaissements déjà
-- enregistrés ne référencent aucun moyen déclaré, et le rester est honnête
-- (on ne leur invente pas une origine après coup).

CREATE TABLE IF NOT EXISTS payment_methods (
  id                 TEXT PRIMARY KEY,
  organization_id    TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id         TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  code               TEXT NOT NULL,
  name               TEXT NOT NULL,
  family             TEXT NOT NULL DEFAULT 'other',
  requires_reference BOOLEAN NOT NULL DEFAULT false,
  is_active          BOOLEAN NOT NULL DEFAULT false,
  sort_order         INTEGER NOT NULL DEFAULT 0,
  kind               TEXT NOT NULL DEFAULT 'modele',
  created_by_name    TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT payment_methods_family CHECK
    (family IN ('cash', 'mobile_money', 'card', 'transfer', 'deferred', 'other')),
  CONSTRAINT payment_methods_kind CHECK (kind IN ('modele', 'personnalisee')),
  CONSTRAINT payment_methods_code_unique UNIQUE (station_id, code)
);
CREATE INDEX IF NOT EXISTS idx_payment_methods_station
  ON payment_methods(station_id, is_active, sort_order);

ALTER TABLE payments ADD COLUMN IF NOT EXISTS payment_method_id TEXT
  REFERENCES payment_methods(id) ON DELETE SET NULL;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS payment_method_name TEXT NOT NULL DEFAULT '';

-- La contrainte d'origine ne connaissait pas le paiement différé : un client
-- qui part en promettant de payer n'était pas représentable. On l'élargit sans
-- toucher aux valeurs existantes.
--   `differe` : engagement de paiement, **jamais** un encaissement — le solde
--   de la fiche reste dû et aucun compteur de caisse ne bouge.
DO $$
BEGIN
  ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_method_check;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'payments_method_check') THEN
    ALTER TABLE payments ADD CONSTRAINT payments_method_check
      CHECK (method IN ('especes', 'mobile_money', 'carte', 'virement', 'differe', 'autre'));
  END IF;
END $$;

-- Identité du reçu : numéro attribué une seule fois, quand la fiche est soldée,
-- et figé ensuite. Un reçu qui change de numéro n'est pas un reçu.
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS receipt_number TEXT NOT NULL DEFAULT '';
ALTER TABLE wash_orders ADD COLUMN IF NOT EXISTS receipt_issued_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS idx_wash_orders_receipt_number
  ON wash_orders(station_id, receipt_number) WHERE receipt_number <> '';
