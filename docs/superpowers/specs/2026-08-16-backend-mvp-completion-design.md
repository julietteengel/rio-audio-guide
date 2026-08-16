# Rio Audio Guide — Décision : compléter le backend pour un MVP montrable

Ce document précise les cinq morceaux du backend qui restaient hors de tout plan — discutés en
conversation mais jamais passés par un cycle spec/plan, à corriger ici avant implémentation, plus un
sixième ajouté en cours de discussion. Il complète `2026-08-12-backend-domain-model-design.md` (domaine,
ports, adaptateurs Postgres — déjà faits) et la Tâche 10/11 du plan associé (adaptateur RabbitMQ
publisher, câblage `main.go` — déjà planifiées, pas encore faites).

Contexte : préparation d'un entretien technique Powens (2026-08-18). Objectif explicite de l'autrice :
un backend qui tourne réellement, pas une démo figée — Postgres avec de vraies données du pipeline,
RabbitMQ réel, une API HTTP réelle, du CI/CD réel. Le frontend et le déploiement K8s réel restent hors
scope pour l'instant (le second est esquissé comme artefact, pas déployé — voir sous-système 6).

## Sous-système 1 — Worker RabbitMQ (rôle entrant, driving adapter)

Complète le rôle sortant déjà planifié (Tâche 10, publisher). Consomme `tts_jobs`, appelle
`application.StartAudioGeneration`, exécute un **stub explicite de génération TTS** (pas de vraie
intégration ElevenLabs — nécessiterait sa propre conception et une clé API, hors scope ici), upload le
résultat via `ports.AudioStorage` (sous-système 2), appelle `application.CompleteAudioGeneration`, puis
`Ack`/`Nack` le message AMQP selon le résultat — `Nack` avec requeue sur erreur transitoire (échec
repository), `Nack` sans requeue sur message malformé (boucle infinie sinon).

`internal/adapters/rabbitmq/worker.go` — `Worker` avec `Run(ctx) error`, bloquant, à lancer dans sa
propre goroutine (ou son propre binaire, voir sous-système 6). Réutilise le message JSON déjà défini
côté publisher (`ttsJobMessage`), même package.

## Sous-système 2 — Adaptateur de stockage (AWS S3)

Nouveau port, absent du plan initial : `ports.AudioStorage` — `Upload(ctx, key string, data []byte)
(url string, err error)`. Implémentation `internal/adapters/s3/audio_storage.go` avec le SDK AWS v2.
**Révisé le 2026-08-16 : testé contre un vrai bucket AWS S3** (`rio-audioguide-bucket`, compte AWS déjà
disponible) plutôt que LocalStack (mur de licence rencontré en pratique) ou MinIO — pratique AWS réelle,
cohérente avec l'objectif de maîtrise. Identifiants uniquement dans l'environnement, jamais dans le code.
La
même implémentation pointerait vers un vrai bucket en changeant seulement l'endpoint/la config, pas le
code. Le worker (sous-système 1) l'appelle avant `CompleteAudioGeneration`.

## Sous-système 3 — API HTTP (Echo)

Framework Echo, déjà maîtrisé par l'autrice (expérience Stockhelp) — choix pragmatique, pas de nouvelle
techno à apprendre sous contrainte de temps. Volontairement minimale, pas un CRUD complet :

- `GET /places` — liste depuis Postgres (`PlaceRepository`), preuve de lecture réelle.
- `POST /scripts/{id}/review` — déclenche `application.ReviewAndRequestAudio`, preuve d'écriture réelle
  et du flux métier complet (relecture → job TTS publié).

Le controller traduit directement la requête HTTP en appel aux fonctions de `internal/application/` —
pas de port d'entrée séparé/explicite (choix assumé, cohérent avec la taille actuelle du projet : un
seul point d'entrée prévu, pas de bénéfice à une interface supplémentaire pour l'instant).

## Sous-système 4 — Script d'import (pipeline Python → Postgres)

`cmd/import/main.go` — programme Go, pas un script séparé, pour garantir que les données importées
respectent les mêmes invariants que le reste du backend (passe par `domain.NewPlace`/`NewScript` et les
repositories déjà écrits, pas d'insertion SQL directe qui contournerait la validation).

**Contrainte découverte en préparant ce document** : les lieux et les narrations ne vivent pas dans le
même format. `pipeline/curation/places_cultural_v3.csv` (colonnes : `id, name, category, source, lat,
lon, wikidata_qid, reason`) donne les lieux. Les narrations vivent dans `narrations_data_part1-4.py` —
des listes Python littérales (`id, name, fr, en, es, pt`), illisibles directement depuis Go.

**Étape préalable, hors scope Go** : un export Python (quelques lignes, à faire une fois par l'autrice)
qui fusionne `DATA_PART1` à `DATA_PART4` en un unique JSON plat (`id`, `language`, `text` par entrée) —
`narrations_export.json`. Le programme Go lit ce fichier une fois généré, pas les `.py` directement.

Chaque `Script` importé démarre en statut `draft` (comportement par défaut de `NewScript`) — cohérent
avec l'invariant déjà posé : pas de publication sans relecture humaine, y compris pour les données
importées en masse.

## Sous-système 5 — CI/CD (GitHub Actions)

**Révisé le 2026-08-16, inspiré des cours FIAP de l'autrice** (`fiap-dclt-aula01/02/03`, module CI/CD —
patterns déjà éprouvés dans ses propres travaux, adaptés de Node.js à Go). Trois workflows séparés, pas
un seul monolithique, chacun avec un déclencheur distinct :

### `backend-ci.yml` — sur chaque push/PR vers `backend`

Structure en deux étages, comme `fiap-dclt-aula02/ci-multistage.yml` : un premier étage de jobs **en
parallèle** (aucun `needs` entre eux, donc ils tournent simultanément), un second étage qui dépend du
premier.

- **Étage 1 (parallèle)** : `lint` (`go vet ./...`), `test` (`go test ./...`, tests unitaires seulement,
  sans le tag `integration`), `security` (`govulncheck ./...` — l'équivalent Go de `npm audit`, scan de
  vulnérabilités officiel de l'équipe Go).
- **Étage 2** : `build` (`needs: [lint, test, security]`) — `go build ./...`, la preuve que tout
  s'assemble une fois les trois vérifications passées.
- **Étage 3** : `integration-test` (`needs: build`) — `go test -tags=integration ./...` contre des
  `services:` Postgres (`postgis/postgis`) et RabbitMQ démarrés par GitHub Actions lui-même. Le bucket
  S3 réel n'a pas d'équivalent `services:` — ces tests-là liraient des identifiants AWS depuis GitHub
  Secrets plutôt qu'un conteneur (à instrumenter si repris ; sinon `S3_TEST_BUCKET` reste absent en CI
  et le test S3 se `Skip` proprement, comme il le fait déjà en local sans la variable).

### `docker-build.yml` — sur push vers `backend`, seulement si `cmd/**`, `internal/**` ou un Dockerfile changent

Calqué sur `fiap-dclt-aula03/docker-build.yml` : `docker/setup-buildx-action`, `aws-actions/configure-
aws-credentials` (identifiants en GitHub Secrets, jamais en clair), `aws-actions/amazon-ecr-login`, puis
`docker/build-push-action` pour les deux images (`Dockerfile.api`, `Dockerfile.worker`) vers un vrai
ECR — tags `latest` et SHA du commit. Nécessite d'avoir fait le sous-système 6 (Dockerfiles) d'abord.

### `k8s-deploy.yml` — gardé en réserve, pas écrit dans ce cycle

Le pattern de `fiap-dclt-aula03/k8s-deploy.yml` (déclenché par `workflow_run` une fois `docker-
build.yml` réussi, `aws eks update-kubeconfig`, Kustomize pour le tag d'image, `kubectl apply -k`,
smoke test contre le LoadBalancer) est directement réutilisable, mais seulement au moment du vrai
déploiement EKS — pas avant, cohérent avec "manifests écrits, pas déployés" du sous-système 6 tant que
ce moment n'est pas venu.

## Sous-système 6 — Manifests Kubernetes (écrits, pas déployés)

Ajouté en cours de discussion : le worker (sous-système 1) et l'API (sous-système 3) sont deux
processus déployables distincts, avec des besoins de scaling différents — exactement la situation où
Kubernetes devient pertinent plutôt qu'artificiel, cohérent avec `2026-08-04-backend-stack-decision.md`
qui anticipait déjà KEDA pour ce worker précis.

**Écrit maintenant, déployé plus tard** — pas de cluster réel monté avant l'entretien (même raisonnement
que pour la démo : une infra live pendant l'entretien est un risque, pas une preuve) :
- `Dockerfile` (un par binaire : `cmd/api`, futur `cmd/worker` si le worker est séparé en binaire propre).
- `Deployment` + `Service` + `HorizontalPodAutoscaler` (CPU/requêtes) pour l'API.
- `Deployment` + `ScaledObject` KEDA (profondeur de la queue RabbitMQ, pas le CPU) pour le worker.

Objectif explicite de l'autrice : réviser HPA et KEDA concrètement en les écrivant, pas juste en
sachant les définir à l'oral. Le déploiement réel sur un cluster viendra dans un second temps, hors de
la contrainte de temps actuelle.

## Ce qui reste hors scope de ce document

- Vraie intégration TTS (ElevenLabs) — le stub reste un stub, une vraie intégration est sa propre
  conception (clé API, gestion des quotas, format de réponse).
- Déploiement K8s réel (cluster monté, manifests appliqués) — reporté après l'entretien.
- Authentification/autorisation sur l'API HTTP — les deux routes restent ouvertes, cohérent avec
  l'absence de modèle `users`/`partners` dans le backend à ce stade.
