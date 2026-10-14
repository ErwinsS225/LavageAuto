-- Onboarding des entreprises : état global, étapes enregistrées.
--
-- Pourquoi une table d'étapes plutôt que des colonnes éparpillées : le parcours
-- s'allonge au fil des lots (identité, station, horaires, véhicules,
-- prestations, tarifs, caisse…). Une ligne par étape permet d'ajouter une étape
-- sans nouvelle migration, de connaître la date de complétion et l'auteur.
--
-- `data` ne contient que du descriptif (choix à réafficher dans l'interface) :
-- ce qui compte pour dire qu'une étape est faite est **calculé** à partir des
-- tables réelles (stations, services…), jamais d'un indicateur déclaré par le
-- front — sinon « Suivant » suffirait à déclarer une station configurée.

ALTER TABLE organizations ADD COLUMN IF NOT EXISTS onboarding_state TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS onboarding_started_at TIMESTAMPTZ;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS onboarding_completed_at TIMESTAMPTZ;

-- Contrainte de domaine, posée une seule fois (DO bloque car ADD CONSTRAINT
-- n'accepte pas IF NOT EXISTS).
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'organizations_onboarding_state_check'
  ) THEN
    ALTER TABLE organizations ADD CONSTRAINT organizations_onboarding_state_check
      CHECK (onboarding_state IN ('pending', 'in_progress', 'completed', 'active'));
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS onboarding_steps (
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  step_code       TEXT NOT NULL,
  state           TEXT NOT NULL DEFAULT 'not_started'
                  CHECK (state IN ('not_started', 'in_progress', 'completed', 'needs_fix', 'blocked')),
  data            JSONB NOT NULL DEFAULT '{}'::jsonb,
  completed_at    TIMESTAMPTZ,
  updated_by      TEXT REFERENCES users(id) ON DELETE SET NULL,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, step_code)
);

CREATE INDEX IF NOT EXISTS idx_onboarding_steps_org ON onboarding_steps(organization_id);

-- Les entreprises déjà en service ne repassent pas par le tunnel : elles
-- exploitaient l'application avant que l'onboarding n'existe, et l'exigence
-- « une station non configurée ne vend pas » ne doit pas interrompre leur
-- journée. Les entreprises créées ensuite partent de l'état par défaut
-- (« pending », posé par la colonne).
UPDATE organizations
   SET onboarding_state = 'active',
       onboarding_completed_at = COALESCE(onboarding_completed_at, created_at)
 WHERE onboarding_state = 'pending';
