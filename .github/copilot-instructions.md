# Copilot Instructions

## Project Overview

**vet-tracker** — Application web fullstack de suivi de recherche de stages en clinique vétérinaire.

Le backend Go récupère quotidiennement les cliniques vétérinaires depuis l'API de l'Ordre des Vétérinaires
et les stocke en base. Le frontend est rendu côté serveur en Go (html/template) avec HTMX pour les
interactions dynamiques, permettant de suivre l'état de la démarche pour chaque clinique
(contactée, à relancer, acceptée, refusée, etc.) et d'y attacher des notes libres.

## Repository Structure

```
/
├── backend/
│   ├── cmd/
│   │   └── server/         # main.go — point d'entrée
│   ├── internal/
│   │   ├── api/            # handlers HTTP (Chi router)
│   │   ├── db/             # sqlc generated + migrations goose
│   │   ├── fetcher/        # cron job quotidien + appel API vétérinaire
│   │   ├── models/         # types métier partagés
│   │   └── templates/      # html/template — pages et partials HTMX
│   └── static/             # go:embed — CSS, htmx.min.js, assets
├── Dockerfile              # single-stage : go → alpine runtime
├── docker-compose.yml
└── .env.example
```

## Build & Run

### Développement local

#### Backend (Go)

```bash
cd backend

# Lancer le serveur (hot reload recommandé avec air)
go run ./cmd/server

# Build du binaire
go build -o bin/vet-tracker ./cmd/server

# Tests
go test ./...
go test ./internal/fetcher -run TestFetchClinics

# Lint
golangci-lint run

# Générer le code sqlc après modification des queries SQL
sqlc generate

# Appliquer les migrations goose
goose -dir internal/db/migrations sqlite3 ./data.db up
```

### Docker (production homelab)

```bash
# Premier lancement
docker compose up -d --build

# Rebuild après modification du code
docker compose up -d --build

# Logs
docker compose logs -f app

# Arrêt sans supprimer le volume SQLite
docker compose stop
```

## Architecture

- Le backend Go expose des **routes HTML** (SSR) et des **partials HTMX** sur `/`, servis directement par Chi.
- Les templates HTML sont dans `internal/templates/` et embedés dans le binaire via `go:embed`.
- HTMX est servi depuis `static/htmx.min.js` (embedé) — pas de CDN externe, fonctionne offline.
- Les routes HTMX retournent des **fragments HTML** (partials) via `HX-Request` header detection.
- Un **cron job interne** (robfig/cron) déclenche le fetch quotidien de l'endpoint :
  `https://extranet.veterinaire.fr/api/directories/search-veterinaries-for-care`
  Les cliniques sont upsertées en base sans écraser les données de tracking existantes.
- La base de données est **SQLite** (fichier `data.db`), montée sur un volume Docker pour la persistance.
- Pas d'authentification — l'app est exposée uniquement sur le réseau local du homelab.

### Réponse JSON de l'API vétérinaire

Endpoint : `POST https://extranet.veterinaire.fr/api/directories/search-veterinaries-for-care`

#### Paramètres du corps de la requête (JSON)

| Paramètre       | Type          | Description                                                       | Exemple                          |
|-----------------|---------------|-------------------------------------------------------------------|----------------------------------|
| `page`          | `int`         | Numéro de page (0-indexé)                                        | `0`                              |
| `limit`         | `int`         | Nombre de résultats par page                                     | `15`                             |
| `ordinalNumber` | `int`         | Numéro ordinal du vétérinaire — filtre sur un vétérinaire précis | `13022`                          |
| `regionId`      | `int`         | Identifiant de région                                             | `5`                              |
| `departmentId`  | `int`         | Identifiant de département                                        | `30`                             |
| `municipalities`| `string`      | Commune(s), séparées par virgule                                  | `"VILLEPARISIS,IDS-SAINT-ROCH"`  |

Exemple de corps de requête complet :

```json
{
  "page": 0,
  "limit": 15,
  "regionId": 5,
  "departmentId": 30,
  "municipalities": "VILLEPARISIS"
}
```

> Le fetcher de production utilise uniquement `page` et `limit` pour paginer sur l'ensemble des cliniques sans filtre géographique.

#### Exemple de réponse

```json
{
  "pagination": { "page": 0, "total": 162, "limit": 1 },
  "veterinaries": [
    {
      "ordinalNumber": 19315,
      "lastname": "ACCICA",
      "firstname": "Paolo",
      "dpes": [
        {
          "name": "CABINET VETERINAIRE CHARLEMAGNE",
          "phone": "04 27 01 75 08",
          "address": {
            "way": "17 Cours Charlemagne",
            "additionalAddress1": null,
            "additionalAddress2": null,
            "zipCode": "69002",
            "city": "LYON",
            "countryId": null
          },
          "ordinalNumber": "503737-1"
        }
      ]
    }
  ]
}
```

> Un vétérinaire (`veterinary`) peut avoir plusieurs DPEs (domiciles professionnels d'exercice).
> L'entité principale pour le suivi de stage est le **DPE** (= une clinique).
> Le fetcher itère sur toutes les pages via le champ `pagination.total`.

### Modèle de données SQLite

```sql
-- Cliniques / DPEs — données brutes issues de l'API
-- external_id = dpe.ordinalNumber ("503737-1")
clinics (
  id                  INTEGER PRIMARY KEY,
  external_id         TEXT UNIQUE NOT NULL,  -- dpe.ordinalNumber
  name                TEXT NOT NULL,          -- dpe.name
  phone               TEXT,                   -- dpe.phone
  address_way         TEXT,                   -- dpe.address.way
  address_additional1 TEXT,                   -- dpe.address.additionalAddress1
  address_additional2 TEXT,                   -- dpe.address.additionalAddress2
  zip_code            TEXT,                   -- dpe.address.zipCode
  city                TEXT,                   -- dpe.address.city
  vet_lastname        TEXT,                   -- veterinary.lastname
  vet_firstname       TEXT,                   -- veterinary.firstname
  vet_ordinal_number  INTEGER,                -- veterinary.ordinalNumber
  fetched_at          DATETIME NOT NULL
)

-- Statut de suivi par clinique (une entrée par clinique)
tracking_entries (
  id         INTEGER PRIMARY KEY,
  clinic_id  INTEGER UNIQUE NOT NULL REFERENCES clinics(id),
  status     TEXT NOT NULL DEFAULT 'not_contacted',
  updated_at DATETIME NOT NULL
)
-- status : "not_contacted" | "contacted" | "to_follow_up" | "accepted" | "refused" | "no_reply"

-- Notes libres horodatées (plusieurs notes possibles par clinique)
notes (
  id         INTEGER PRIMARY KEY,
  clinic_id  INTEGER NOT NULL REFERENCES clinics(id),
  content    TEXT NOT NULL,
  created_at DATETIME NOT NULL
)
```

### Variables d'environnement (`.env`)

```env
PORT=8080
DB_PATH=./data.db
FETCH_CRON="0 8 * * *"   # tous les jours à 8h (format cron standard)
```

## Key Conventions

### Go (Backend)

- Package layout : `cmd/` pour les points d'entrée, `internal/` pour tout le code applicatif.
- Les erreurs sont retournées, jamais `panic` ; wrapping avec `fmt.Errorf("context: %w", err)`.
- Les handlers HTTP sont dans `internal/api/` ; chaque ressource a son propre fichier (`clinics.go`, `notes.go`).
- Les handlers détectent `HX-Request` pour retourner soit une page complète, soit un partial HTMX.
- Les queries SQL sont dans `internal/db/query/` et gérées par **sqlc** — ne pas écrire de SQL inline dans les handlers.
- Les migrations sont dans `internal/db/migrations/` et gérées par **goose** (format SQL).
- Le fetcher dans `internal/fetcher/` est découplé du reste via une interface — facilite les tests unitaires.

### Templates (Frontend)

- Les templates Go (`html/template`) sont dans `internal/templates/`.
- Organisation : `layouts/base.html` pour le layout commun, `pages/*.html` pour les pages complètes, `partials/*.html` pour les fragments HTMX.
- HTMX est chargé depuis `/static/htmx.min.js` (embedé dans le binaire).
- Le style est géré par **Tailwind CSS** via le CDN play.tailwindcss.com en dev, ou un fichier CSS compilé embedé en prod.
- Pas de JavaScript custom — toute interactivité passe par les attributs HTMX (`hx-get`, `hx-post`, `hx-swap`, etc.).

### Docker

- Le `Dockerfile` est **single-stage** : golang builder → alpine runtime (plus de stage node).
- Le container final tourne en **utilisateur non-root**.
- Le fichier `data.db` est dans un **volume nommé** Docker — ne jamais le binder sur un chemin relatif en prod.

---

_Mettre à jour ce fichier au fur et à mesure que l'architecture évolue._
