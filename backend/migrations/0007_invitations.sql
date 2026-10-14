-- 0007 — Invitations : rejoindre une entreprise existante.
--
-- Créer un espace (inscription) et le rejoindre (invitation) sont deux chemins
-- distincts. Une invitation ne crée jamais d'entreprise, de station ni
-- d'abonnement : elle rattache une personne à ce qui existe déjà, avec un rôle
-- opérationnel. « owner » est volontairement absent des rôles autorisés : la
-- propriété d'une entreprise ne s'obtient pas par un lien, seulement par
-- l'inscription ou par une décision du support.
--
-- Deux identifiants désignent la même invitation :
--   - `code`, 8 caractères, saisissable au téléphone et dictable à voix haute
--     (alphabet sans 0/O/1/I/L, qui se confondent) ;
--   - `token_hash`, l'empreinte d'un jeton long présent dans le lien partagé.
-- Le jeton en clair n'est jamais stocké : un vol de base ne permet pas de
-- rejoindre une entreprise. Seul le hachage est conservé, donc le lien ne peut
-- pas non plus être reconstitué après coup — l'interface le montre une fois,
-- au moment de la création, puis ne peut que régénérer une invitation.

CREATE TABLE IF NOT EXISTS invitations (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT REFERENCES stations(id) ON DELETE SET NULL,
  email           TEXT NOT NULL DEFAULT '',
  phone           TEXT NOT NULL DEFAULT '',
  role            TEXT NOT NULL CHECK (role IN ('manager', 'cashier', 'washer')),
  token_hash      TEXT NOT NULL UNIQUE,
  code            TEXT NOT NULL UNIQUE,
  expires_at      TIMESTAMPTZ NOT NULL,
  invited_by      TEXT REFERENCES users(id) ON DELETE SET NULL,
  accepted_at     TIMESTAMPTZ,
  accepted_by     TEXT REFERENCES users(id) ON DELETE SET NULL,
  revoked_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Une invitation sans destinataire ne serait remise à personne : il faut au
  -- moins une adresse e-mail ou un numéro de téléphone.
  CHECK (email <> '' OR phone <> '')
);

CREATE INDEX IF NOT EXISTS idx_invitations_org ON invitations(organization_id);
CREATE INDEX IF NOT EXISTS idx_invitations_email ON invitations(lower(email));
CREATE INDEX IF NOT EXISTS idx_invitations_phone ON invitations(phone);
