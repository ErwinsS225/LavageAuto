-- 0012_onboarding_activation — la trace de l'activation d'une station.
--
-- L'activation ouvre l'exploitation : à partir de cet instant, la station
-- encaisse. Jusqu'ici, seul `onboarding_state` changeait — impossible de dire
-- plus tard sur quoi portait l'ouverture, ni qui l'avait décidée.
--
-- Trois colonnes, aucune donnée inventée :
--   * `onboarding_activated_by(_name)` : l'auteur, lisible sans jointure (le
--     compte peut disparaître, la trace reste) ;
--   * `onboarding_snapshot` : ce qui a été **constaté** au moment de
--     l'activation (prérequis satisfaits, comptages du catalogue, journées
--     d'ouverture, réglages facultatifs restants). Aucun montant, aucune donnée
--     personnelle : des constats et des nombres.
--
-- Les entreprises déjà actives gardent leurs dates et un instantané vide : on
-- ne reconstitue pas après coup ce qui n'a pas été observé.

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS onboarding_activated_by TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS onboarding_activated_by_name TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS onboarding_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN organizations.onboarding_snapshot IS
  'Constats enregistrés au moment de l''activation (prérequis, comptages, horaires, réglages facultatifs restants).';
