-- ---------------------------------------------------------------------------
-- 0003 — toute station exige une session de caisse avant d'encaisser les espèces
--
-- Le réglage `require_open_cash_session` était bien écrit par la création d'une
-- station depuis l'écran Réglages et par le jeu de démonstration, mais pas par
-- l'inscription : une entreprise créée en s'inscrivant acceptait donc des
-- espèces alors qu'aucune session de caisse n'était ouverte, contrairement à
-- ce qu'affiche l'écran Réglages (interrupteur activé par défaut).
--
-- Cette migration remet les stations concernées d'aplomb : le réglage est
-- ajouté uniquement là où il est absent. Un choix explicite de l'exploitant
-- (`false`, pour encaisser sans caisse ouverte) n'est jamais écrasé.
-- ---------------------------------------------------------------------------

UPDATE stations
   SET settings = COALESCE(settings, '{}'::jsonb) || '{"require_open_cash_session": true}'::jsonb
 WHERE settings IS NULL
    OR NOT (settings ? 'require_open_cash_session');
