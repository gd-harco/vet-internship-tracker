# Vet-Tracker

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go) ![SQLite](https://img.shields.io/badge/SQLite-003B57?logo=sqlite) ![HTMX](https://img.shields.io/badge/HTMX-3366CC) ![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)

> Web app (Go + HTMX + SQLite) to track internship applications to veterinary clinics: periodic sync of the official directory, status tracking, notes and map view.

Application web de suivi des candidatures auprès de cliniques vétérinaires. Elle permet de consulter les cliniques, suivre les démarches, ajouter des notes et visualiser les adresses géocodées sur une carte.

## Stack et structure

- Backend : Go, routeur Chi, SQLite (`modernc.org/sqlite`) et migrations Goose.
- Interface : templates `html/template` en `.gohtml`, HTMX et CSS servi localement. La carte est rendue par les templates dédiés.
- Les requêtes SQL sont dans `backend/sql/query/`; `backend/internal/database/` contient le code généré par sqlc.

```text
Vet-Tracker/
├── backend/
│   ├── cmd/server/           # Serveur web et synchronisation périodique
│   ├── cmd/geocoding/        # Géocodage des cliniques sans coordonnées
│   ├── internal/api/         # Routes, handlers et accès aux données de l'interface
│   ├── internal/database/    # Code sqlc généré
│   ├── internal/fetcher/     # Client de l'annuaire vétérinaire
│   ├── internal/templates/   # Layouts, pages et fragments HTML
│   ├── sql/migrations/       # Migrations Goose embarquées dans le binaire
│   ├── sql/query/            # Requêtes source sqlc
│   └── static/               # HTMX et styles
├── Dockerfile
├── docker-compose.yml        # Déploiement Traefik
├── docker-compose.dev.yml    # Build local
└── doc.MD
```

## Prérequis et démarrage local

Le module Go déclare Go **1.26.0** (`backend/go.mod`). Depuis la racine du dépôt :

```powershell
cd backend
$env:DEV="true"
go run ./cmd/server
```

Ouvrir <http://localhost:8080>. `DEV=true` recharge les templates à chaque requête et active le journal HTTP de Chi. Sans cette variable, les templates sont chargés au démarrage.

Le serveur crée ou met à niveau le schéma SQLite au démarrage. Il ne crée pas de cliniques de démonstration : si la base ne contient pas encore de données synchronisées, il lance une synchronisation de l'annuaire en arrière-plan.

Commandes utiles, depuis `backend/` :

```powershell
go test ./...
go build -o vet-tracker.exe ./cmd/server
go run ./cmd/geocoding
```

Le serveur et le géocodeur utilisent le même `DB_PATH`. Lancez-les depuis le dossier `backend` si vous souhaitez utiliser les chemins par défaut.

## Fonctionnalités et routes

| Méthode  | Route                                | Comportement                                                    |
| -------- | ------------------------------------ | --------------------------------------------------------------- |
| `GET`    | `/`                                  | Liste paginée des cliniques et formulaires de suivi             |
| `GET`    | `/partials/clinics?page=N`           | Fragment HTML d'une page de résultats pour le défilement infini |
| `GET`    | `/card`                              | Vue en liste des cliniques                                      |
| `GET`    | `/map`                               | Vue carte et marqueurs géocodés                                 |
| `GET`    | `/map/popup/{clinicID}`              | Informations JSON pour une clinique de la carte                 |
| `GET`    | `/clinics/{clinicID}/edit`           | Formulaire de modification                                      |
| `PUT`    | `/clinics/{clinicID}`                | Enregistre les modifications d'une clinique                     |
| `GET`    | `/clinics/{clinicID}/view`           | Fragment d'informations d'une clinique                          |
| `GET`    | `/clinics/{clinicID}/card`           | Carte HTML d'une clinique et de ses notes                       |
| `POST`   | `/clinics/manual`                    | Ajoute une clinique manuellement                                |
| `POST`   | `/clinics/{clinicID}/hide`           | Masque une clinique sans la supprimer                           |
| `POST`   | `/clinics/{clinicID}/status`         | Met à jour le statut de suivi                                   |
| `POST`   | `/clinics/{clinicID}/notes`          | Ajoute une note                                                 |
| `DELETE` | `/clinics/{clinicID}/notes/{noteID}` | Supprime une note                                               |
| `GET`    | `/static/*`                          | Sert les fichiers statiques                                     |

Les formulaires et fragments HTML utilisent HTMX pour mettre à jour la page sans rechargement complet. Les routes ne mettent pas en œuvre d'authentification; ne les exposez pas directement à Internet sans ajouter une protection adaptée.

## Données et synchronisation

Les migrations Goose sont embarquées dans `backend/sql/migrations/` et appliquées automatiquement au démarrage du serveur.

- `clinics` contient les coordonnées de contact et d'adresse, l'identifiant externe, la source (`fetched` ou `manual`), la visibilité, le statut de suivi et les coordonnées géographiques.
- `notes` contient les notes liées aux cliniques.
- Le statut est stocké directement dans `clinics.track_status`; la table historique `tracking_entries` est supprimée par la migration `0005_tracking_in_clinic.sql`.
- Les cliniques masquées restent en base mais ne figurent plus dans les vues visibles.

Le serveur interroge l'annuaire de l'Ordre pour le département **70**, avec une limite de 15 résultats par page et pagination selon la réponse de l'API. Il synchronise au démarrage si aucune clinique récupérée n'existe ou si la dernière récupération date de plus de 12 heures, puis toutes les 24 heures. Les enregistrements sont insérés ou mis à jour par identifiant externe; les notes et statuts existants sont conservés.

Le programme `cmd/geocoding` recherche les cliniques sans coordonnées avec le service de géocodage adresse.data.gouv.fr, puis enregistre latitude et longitude. Il peut être lancé ponctuellement avec `go run ./cmd/geocoding` depuis `backend/`.

## Configuration

| Variable                  | Valeur par défaut                 | Utilisation                                                                                  |
| ------------------------- | --------------------------------- | -------------------------------------------------------------------------------------------- |
| `PORT`                    | `8080`                            | Port HTTP du serveur                                                                         |
| `DB_PATH`                 | `./data.db`                       | Fichier de base SQLite                                                                       |
| `LOG_PATH`                | `./vet-tracker.log`               | Fichier de journal du serveur                                                                |
| `VET_API_BASE_URL`        | `https://extranet.veterinaire.fr` | URL de base de l'API de l'annuaire                                                           |
| `VET_API_TIMEOUT_SECONDS` | `20`                              | Timeout des requêtes à l'annuaire; une valeur invalide ou non positive revient à 20 secondes |
| `DEV`                     | non définie                       | `true` active le rechargement des templates et le middleware de journal HTTP                 |

Le géocodeur respecte `DB_PATH` et `LOG_PATH`; son fichier de journal par défaut est `./geocoding.log`.

Exemple PowerShell avec une base persistante distincte :

```powershell
cd backend
$env:DB_PATH=".\data.db"
$env:LOG_PATH=".\vet-tracker.log"
go run ./cmd/server
```

## Docker

Développement (port `8080`, données dans un volume Docker nommé) :

```bash
docker compose -f docker-compose.dev.yml up --build
```

Production (homelab) : `docker-compose.yml` construit l'image, la publie derrière Traefik et protège l'accès hors réseau local par basic-auth. Copiez `.env.example` vers `.env`, renseignez `VET_HOST` et `VET_BASICAUTH_USERS`, puis :

```bash
docker compose up -d --build
```

Le réseau Docker externe `frontend` (Traefik) doit exister sur l'hôte. Le conteneur tourne en utilisateur non-root.

## Licence

[MIT](LICENSE)