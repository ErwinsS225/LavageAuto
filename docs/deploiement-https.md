# Mettre WashGuard en production derrière HTTPS

Ce document décrit la mise en ligne sur un serveur (VPS, machine au bureau avec un
nom de domaine, ou poste exposé via un tunnel). L'application est un **seul
processus** qui sert l'API et l'interface ; un proxy inverse s'occupe du
certificat et du chiffrement.

Pour une simple station sur le réseau du bureau, sans domaine,
[`installation-station.md`](installation-station.md) suffit.

## 1. Compiler

```bash
git clone <dépôt privé> && cd LavageAuto
make install          # dépendances du front
make build            # binaire washguard-api + interface dist/
make check            # formatage, tests, routes, écrans, configuration
```

`make build` produit `washguard-api` à la racine et `dist/` (interface compilée).
Le binaire sert `dist/` quand `STATIC_DIR` est défini : pas de serveur Node en
production.

## 2. Configuration de production

Dans `.env` (jamais committé) :

| Réglage | Valeur attendue | Pourquoi |
| --- | --- | --- |
| `NEON_DATABASE_URL` (ou `DATABASE_URL`) | chaîne de la base de production | Base dédiée ; jamais celle des essais |
| `SEED_DEMO` | `false` | Aucune donnée de démonstration ne doit apparaître chez un client |
| `SESSION_COOKIE_SECURE` | `true` | Le cookie de session ne circule qu'en HTTPS |
| `APP_URL` | `https://washguard.exemple.ci` | Origine publique de l'application |
| `CLERK_AUTHORIZED_PARTIES` | la même origine | Un jeton Clerk émis pour un autre site est refusé |
| `CLERK_SECRET_KEY`, `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY`, `JWKS_URL_CLERK`, `CLERK_WEBHOOK_SIGNING_SECRET` | clés **`live`** de production | Les clés `test` n'authentifient que des comptes de test |
| `API_HOST` | `127.0.0.1` (derrière un proxy) | L'API n'est joignable que par le proxy |
| `API_PORT` | `3000` (valeur du gabarit) ou tout port libre | Port interne ; seul le proxy est exposé |
| `STATIC_DIR` | `dist` | Le binaire expose l'interface compilée |
| `CORS_ALLOWED_ORIGINS` | l'origine publique, si l'interface est servie ailleurs | Sinon laisser vide |

Contrôle avant démarrage :

```bash
bash scripts/verifications/verifier-env.sh prod    # ne montre jamais une valeur
make prod-check                                    # vérifie l'installation sur une base dédiée
```

`verifier-env.sh prod` refuse `SEED_DEMO` autre chose que `false`, exige
`SESSION_COOKIE_SECURE=true` et signale les valeurs restées au gabarit.

## 3. Proxy inverse

### Caddy (le plus court : certificat automatique)

```caddyfile
washguard.exemple.ci {
    encode zstd gzip
    reverse_proxy 127.0.0.1:3000
}
```

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name washguard.exemple.ci;

    ssl_certificate     /etc/letsencrypt/live/washguard.exemple.ci/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/washguard.exemple.ci/privkey.pem;

    client_max_body_size 8m;      # exports et pièces jointes de dépense

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 120s;
    }
}

server {
    listen 80;
    server_name washguard.exemple.ci;
    return 301 https://$host$request_uri;
}
```

Deux points d'attention :

- **`X-Forwarded-For`** : le limiteur de tentatives de connexion lit l'adresse
  réelle derrière le proxy. L'API ne doit donc **pas** être joignable directement
  depuis Internet (`API_HOST=127.0.0.1`), sinon l'en-tête peut être falsifié.
- **Websocket/HTTP2** : l'application n'en a pas besoin ; une terminaison HTTPS
  classique suffit.

## 4. Démarrage permanent

### Linux (systemd)

```ini
# /etc/systemd/system/washguard.service
[Unit]
Description=WashGuard
After=network.target

[Service]
User=washguard
WorkingDirectory=/home/washguard/LavageAuto/backend
EnvironmentFile=/home/washguard/LavageAuto/.env
Environment=STATIC_DIR=../dist
Environment=API_HOST=127.0.0.1
ExecStart=/home/washguard/LavageAuto/washguard-api
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload && sudo systemctl enable --now washguard
```

### macOS

`make station` installe déjà les tâches launchd équivalentes (API + sauvegarde
quotidienne).

Les migrations de schéma sont appliquées **au démarrage** du binaire : un
`git pull` suivi d'un redémarrage du service suffit à mettre la base à niveau
(`./washguard-api -migrate-only` les joue sans démarrer le serveur).

## 5. Vérifier la mise en service

```bash
curl -s https://washguard.exemple.ci/health          # {"status":"ok",...}
curl -sI https://washguard.exemple.ci/ | head -3     # 200, content-type text/html
make smoke                                           # routes et rendu des 11 écrans
make verifier-acces                                  # sondes d'acceptation (API qui tourne)
```

Puis, dans un navigateur : créer l'espace de l'entreprise (ou se connecter avec le
compte propriétaire), vérifier que le passage en HTTPS ne provoque pas de
« cookie refusé » (donc que `SESSION_COOKIE_SECURE` et `APP_URL` sont cohérents),
et que le webhook Clerk reçoit bien les événements (console Clerk → Webhooks →
tentatives réussies).

## 6. Mises à jour et retour arrière

```bash
git pull
make install && make build      # recompile API et interface
sudo systemctl restart washguard
```

Retour arrière : revenir au commit précédent (`git checkout <commit>`), recompiler,
redémarrer. Si la mise à jour contenait une migration de schéma, restaurer la
sauvegarde prise **avant** la mise à jour
(voir [`sauvegarde-restauration.md`](sauvegarde-restauration.md)) : les migrations
sont appliquées dans l'ordre, jamais annulées automatiquement.
