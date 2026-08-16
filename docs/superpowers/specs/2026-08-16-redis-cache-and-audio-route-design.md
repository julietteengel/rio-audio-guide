# Rio Audio Guide — Route audio, cache Redis, classification des erreurs S3

Ce document conçoit la dernière pièce manquante du scénario "je suis près d'un lieu, je peux écouter son
histoire" : une route pour récupérer l'audio déjà généré, le cache Redis devant les lectures chaudes
(déclencheur mesuré, voir `2026-08-04-backend-stack-decision.md`), et un correctif sur la classification
des erreurs S3 trouvé en testant la chaîne réelle ce soir.

## Contexte

`POST /scripts/{id}/review` déclenche la génération mais rien ne permet de récupérer le résultat ensuite.
Testé manuellement ce soir avec succès (requête Postgres directe + `aws s3 cp`), mais ce n'est pas ce
qu'un vrai client ferait. Deux méthodes de repository manquent pour construire cette route : aucune ne
permet aujourd'hui de retrouver un `Script` par lieu+langue, ni un `AudioFile` par `Script`.

Par ailleurs, l'URL stockée par le worker (`internal/adapters/s3/audio_storage.go`) est de la forme
`s3://bucket/clé` — pas une URL HTTP qu'un client peut charger. Il faut la convertir en URL présignée à la
demande.

Enfin, un bug trouvé en testant ce soir : une erreur d'upload S3 (`InvalidAccessKeyId`, credentials
invalides) a bouclé en retry indéfiniment — les erreurs S3 ne sont pas classées transitoire/permanent
comme le sont déjà celles d'ElevenLabs (`2026-08-16-elevenlabs-integration-design.md`).

## Autorship — pair programming pour cette session

Contrairement au chantier ElevenLabs (où une exception d'auteur avait été accordée pour `internal/ports/`
sur demande explicite, faute de temps), pour ce chantier les trois ajouts à `internal/ports/` sont écrits
par la fondatrice elle-même, en pair programming — l'IA explique le pattern, propose la structure attendue
(signatures, où ça s'insère), relit chaque morceau au fur et à mesure. Le reste (adaptateurs, route HTTP,
correctif S3) reste IA-écrit, sans restriction.

| Fichier | Auteur |
|---|---|
| `internal/ports/script_repository.go` (ajout) | Fondatrice, pair programming |
| `internal/ports/audiofile_repository.go` (ajout) | Fondatrice, pair programming |
| `internal/ports/audio_storage.go` (ajout présignation) | Fondatrice, pair programming |
| `internal/ports/cache.go` (nouveau) | Fondatrice, pair programming |
| `internal/adapters/postgres/*.go` | IA |
| `internal/adapters/s3/audio_storage.go` (présignation + classification erreurs) | IA |
| `internal/adapters/redis/cache.go` (nouveau) | IA |
| `internal/adapters/http/server.go` (nouvelle route, cache-aside) | IA |
| `cmd/api/main.go` | IA |
| `deploy/helm/rio-backend/` (Redis ajouté au chart) | IA |

## 1. Nouvelles méthodes de repository

```go
// internal/ports/script_repository.go
FindByPlaceIDAndLanguage(ctx context.Context, placeID, language string) (*domain.Script, error)
```
S'appuie sur la contrainte `UNIQUE (place_id, language)` déjà présente sur la table `scripts` — au plus
une ligne possible, pas d'ambiguïté à gérer.

```go
// internal/ports/audiofile_repository.go
FindByScriptID(ctx context.Context, scriptID string) (*domain.AudioFile, error)
```

## 2. Route audio + présignation S3

**Nouvelle route** `GET /places/:id/audio?language=fr` :
1. `ScriptRepository.FindByPlaceIDAndLanguage(placeID, language)` — 404 si absent.
2. `AudioFileRepository.FindByScriptID(script.ID())` — 404 si absent (aucune génération jamais demandée) ;
   **202 Accepted** avec un corps `{"status": "<queued|generating|failed>"}` si trouvé mais
   `AudioFile.Status() != ready` — ne jamais planter sur un audio en cours de génération, c'est un état
   normal, distinct de "n'existe pas".
3. Si `ready` : convertir `storage_url` (`s3://bucket/clé`) en URL HTTP présignée via le SDK AWS
   (`s3.NewPresignClient`, expiration courte — 15 minutes, assez pour un téléchargement immédiat, pas pour
   un lien partageable durablement).

**Nouvelle méthode sur le port `AudioStorage`** :
```go
// internal/ports/audio_storage.go
PresignURL(ctx context.Context, key string, expiry time.Duration) (string, error)
```

## 3. Cache Redis (cache-aside, TTL seul, fail-open)

Déclencheur mesuré : voir `2026-08-04-backend-stack-decision.md`, section "Redis — déclencheur mesuré
atteint (2026-08-16)".

**Nouveau port** :
```go
// internal/ports/cache.go
type Cache interface {
	Get(ctx context.Context, key string) (value string, found bool, err error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}
```

**Comportement** :
- TTL fixe 5 minutes sur les deux routes (`GET /places`, `GET /places/:id/audio`) — pas d'invalidation
  active (voir discussion : les imports/publications sont rares, le produit tolère la fraîcheur à 5 min).
- Clé `GET /places` : les paramètres de bounding box tels quels, sans arrondi en v1 (YAGNI — à ajouter si
  le taux de hit s'avère faible en pratique).
- Clé audio : `audio:{placeID}:{language}`.
- **Fail-open** : toute erreur Redis (indisponible, timeout) est traitée comme un cache miss, jamais comme
  une erreur de la requête — Redis n'est un point de défaillance unique que pour la performance, jamais
  pour la correction fonctionnelle.
- Ne PAS cacher les URLs présignées elles-mêmes au-delà de leur propre expiration (15 min) — si le TTL du
  cache (5 min) est plus court que l'expiration de l'URL présignée, c'est cohérent ; documenté ici pour que
  personne n'allonge le TTL du cache sans y repenser.

**Adaptateur** : `internal/adapters/redis/cache.go`, implémente `ports.Cache` via `redis/go-redis/v9` (pas
de nouvelle dépendance majeure, client Redis standard de l'écosystème Go).

**Déploiement** : Redis ajouté au chart Helm (`deploy/helm/rio-backend`), même pattern que Postgres (chart
Bitnami en dépendance). `REDIS_URL` uniquement sur `rio-api` (le worker ne sert pas de lectures).

## 4. Classification des erreurs S3 (transitoire vs permanent)

Réutilise le type `ports.PermanentError` déjà créé pour ElevenLabs (`2026-08-16-elevenlabs-integration-
design.md`) — pas de nouveau mécanisme. Dans `internal/adapters/s3/audio_storage.go`, inspecter le type
d'erreur retourné par le SDK AWS (`smithy.APIError`, champ `ErrorCode()`) :

- **Permanent** (jamais résolu par un retry) : `InvalidAccessKeyId`, `AccessDenied`,
  `SignatureDoesNotMatch`, `NoSuchBucket`.
- **Transitoire** : tout le reste (`SlowDown`, `InternalError`, `ServiceUnavailable`, timeout réseau).

`internal/adapters/rabbitmq/worker.go` gagne la même vérification `errors.As(err, &permErr)` sur la
branche upload que celle déjà présente sur la branche TTS — sur permanent, `FailAudioGeneration` + `Ack` ;
sinon, `Nack(requeue=true)` avec le délai de 2s déjà en place.

## Tests

- Repository methods (pair programming) : tests d'intégration Postgres, même pattern que l'existant
  (`FindByName` de la Tâche 6 du chantier ElevenLabs).
- Présignation S3 : test unitaire vérifiant la forme de l'URL générée (pas d'appel réseau réel nécessaire,
  le SDK de présignation est déterministe côté client).
- Cache Redis : adaptateur testé en intégration (Redis réel via Docker, `integration` build tag, même
  pattern que Postgres/RabbitMQ) ; la logique cache-aside elle-même testée avec un fake `ports.Cache` dans
  les tests HTTP existants.
- Classification erreurs S3 : table-driven test sur les codes d'erreur SDK, même pattern que le test
  ElevenLabs déjà en place.
- Test manuel de bout en bout déjà effectué ce soir (sans la route/le cache) — sert de preuve que la
  génération fonctionne ; la route/le cache doivent reproduire le même résultat via HTTP plutôt que via
  psql+aws direct.

## Hors scope

- Normalisation/arrondi des clés de cache bounding-box (YAGNI tant que le taux de hit n'est pas mesuré
  faible).
- Invalidation active du cache (TTL seul suffit pour l'instant, voir discussion mesurée).
- Déploiement AWS/EKS réel — voir `2026-08-16-aws-eks-real-deployment-design.md`, chantier séparé.
