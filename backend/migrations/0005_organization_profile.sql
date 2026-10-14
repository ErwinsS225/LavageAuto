-- Identité commerciale, profil juridique et facturation de l'entreprise.
--
-- Pourquoi ces colonnes plutôt qu'un JSONB fourre-tout : ces valeurs figurent
-- sur les factures et servent au contrôle (RCCM, compte contribuable). Elles
-- doivent être interrogeables, comparables d'une entreprise à l'autre, et
-- vérifiables une par une — ce qu'un bloc JSONB ne permet pas proprement.
--
-- Toutes sont nullables ou à défaut vide : un lavage peut ouvrir son compte et
-- commencer à travailler sans connaître son numéro CNPS. Le parcours les
-- demande, il ne les exige pas.

ALTER TABLE organizations ADD COLUMN IF NOT EXISTS legal_name TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS trade_name TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS activity_type TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS activity_description TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS activity_started_on DATE;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS whatsapp TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS website TEXT NOT NULL DEFAULT '';

ALTER TABLE organizations ADD COLUMN IF NOT EXISTS billing_name TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS billing_address TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS billing_email TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS billing_phone TEXT NOT NULL DEFAULT '';

ALTER TABLE organizations ADD COLUMN IF NOT EXISTS legal_form TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS legal_representative TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS rccm TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS tax_id TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS cnps_employer_number TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS fiscal_regime TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS legal_mentions TEXT NOT NULL DEFAULT '';

-- Une information administrative, son état, et qui l'a vérifiée.
--
-- « Renseigné » n'est pas « vérifié » : c'est l'entreprise qui saisit son RCCM,
-- et seule la plateforme (support WashGuard) peut attester l'avoir contrôlé.
-- L'état est donc stocké à part de la valeur : une valeur peut changer sans
-- que la vérification précédente reste valable — d'où la remise à « provided »
-- à chaque modification (voir backend/org_profile.go).
CREATE TABLE IF NOT EXISTS organization_registry_fields (
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  field_code      TEXT NOT NULL,
  value           TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'empty'
                  CHECK (status IN ('empty', 'provided', 'pending_review', 'verified')),
  verified_by     TEXT REFERENCES users(id) ON DELETE SET NULL,
  verified_at     TIMESTAMPTZ,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, field_code)
);

CREATE INDEX IF NOT EXISTS idx_registry_fields_org ON organization_registry_fields(organization_id);
