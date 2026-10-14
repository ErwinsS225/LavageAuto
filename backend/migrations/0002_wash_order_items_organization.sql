-- ---------------------------------------------------------------------------
-- 0002 — wash_order_items porte son entreprise
--
-- Les lignes de fiche n'avaient pas d'organization_id : leur cloisonnement ne
-- tenait qu'à la jointure avec wash_orders. Une requête écrite sans jointure,
-- ou une jointure oubliée dans un futur rapport, laissait donc fuiter les
-- lignes d'une autre entreprise. La colonne rend le filtre possible partout.
--
-- Le remplissage se fait depuis la fiche parente. La colonne reste NULLable
-- le temps de la migration, puis est rendue obligatoire : les lignes créées
-- avant la mise à jour (quelques millisecondes) seraient de toute façon
-- rattrapées par le remplissage ci-dessous.
-- ---------------------------------------------------------------------------

ALTER TABLE wash_order_items ADD COLUMN IF NOT EXISTS organization_id TEXT REFERENCES organizations(id) ON DELETE CASCADE;

UPDATE wash_order_items i
   SET organization_id = o.organization_id
  FROM wash_orders o
 WHERE o.id = i.wash_order_id
   AND i.organization_id IS NULL;

-- Un index pour les rapports par entreprise et par période.
CREATE INDEX IF NOT EXISTS idx_wash_order_items_org ON wash_order_items(organization_id);

-- Contrainte douce : une ligne doit désormais toujours porter son entreprise.
-- (NOT NULL seulement si toutes les lignes ont pu être rattachées : une ligne
-- orpheline — fiche supprimée sans cascade — bloquerait la migration.)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM wash_order_items WHERE organization_id IS NULL LIMIT 1
  ) THEN
    ALTER TABLE wash_order_items ALTER COLUMN organization_id SET NOT NULL;
  END IF;
END $$;
