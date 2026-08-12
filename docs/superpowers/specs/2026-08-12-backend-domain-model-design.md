# Rio Audio Guide — Décision : modèle de domaine et schéma Postgres du backend

Ce document précise, pour la première fois, le contenu concret du backend décidé en principe dans
`2026-08-04-backend-stack-decision.md` (Go, hexagonal, DDD, PostgreSQL/PostGIS, RabbitMQ). Il ne
remplace pas ce document — il le rend exécutable : quels agrégats, quels invariants, quel schéma.

Périmètre : le workflow de publication de contenu uniquement (Place, Script, AudioFile). Partners et
users, présents dans l'esquisse de schéma du design doc v1, restent hors de ce document — secondaires,
à modéliser séparément le moment venu.

## Agrégats

### Place

Import quasi en lecture seule depuis le pipeline Python (dédoublonnage, vérification de périmètre
municipal, triage de catégorie déjà faits en amont) — mais éditable côté admin sur un périmètre
restreint : nom, catégorie, coordonnées, `wikidata_qid`. Raison : la relecture éditoriale des scripts
fait remonter des erreurs ponctuelles sur les lieux eux-mêmes (cas réel déjà rencontré : Casa de Cultura
de Nova Iguaçu, hors périmètre municipal, trouvé après coup) ; forcer un aller-retour par le pipeline
Python pour une correction mineure serait un frein opérationnel réel.

- Une commande d'édition plate (pas d'historique de versions, pas de workflow d'approbation séparé pour
  ces corrections).
- Un retrait en soft-delete (`status = removed`, avec raison) plutôt qu'une suppression physique.
- **Explicitement hors scope** : réconciliation avec un futur réimport du pipeline (que faire si Python
  renvoie une donnée qui écrase une correction manuelle). Pas encore rencontré, pas à résoudre par
  anticipation.

### Script

Une narration = un lieu + une langue. Cycle de vie : `draft` (généré par le pipeline de contenu, importé
tel quel) → `reviewed` (relecture humaine passée) → `published`.

**`published` est un champ stocké**, pas une condition calculée à la volée (relu ET audio présent).
Transition déclenchée par un gestionnaire d'événement quand l'AudioFile associé passe à `ready` — voir
plus bas. Choisi pour deux raisons : ça colle naturellement au flux asynchrone déjà décidé (RabbitMQ),
et ça donne une vraie date de publication (`published_at`) plutôt qu'un état recalculé sans historique.

**Publication par variante linguistique, pas par lieu** : un Script FR peut être publié alors que EN/ES/
PT ne le sont pas encore. Cohérent avec l'état réel du pipeline (les langues n'avancent pas au même
rythme — voir `mission.md`, Current status).

### AudioFile

Résultat du job TTS, agrégat séparé avec son propre cycle de vie : `queued → generating → ready` ou
`failed`. Raison de la séparation : le job est asynchrone et peut échouer/être retenté (retries + DLQ
déjà prévus pour RabbitMQ dans `2026-08-04-backend-stack-decision.md`) — il faut un endroit pour stocker
cet état entre l'envoi du job et son résultat, et c'est précisément l'événement "AudioFile devient ready"
qui fait transiter le Script associé vers `published`.

## Schéma Postgres (v1)

```sql
places       (id, name, category, geom geography(Point,4326), wikidata_qid, source,
              source_richness, status[active|removed], removed_reason, created_at, updated_at)
scripts      (id, place_id fk, language, text, source_text, status[draft|reviewed|published],
              reviewer, reviewed_at, published_at, created_at, updated_at)
audio_files  (id, script_id fk, voice_id, status[queued|generating|ready|failed],
              storage_url, timestamps_url, duration, failure_reason, created_at, updated_at)
```

`geom` en PostGIS (`geography(Point,4326)`) plutôt que des colonnes `lat`/`lon` nues, avec un index GIST.
Usage attendu : requêtes par zone ("lieux dans telle région à télécharger" pour le bundle hors-ligne de
l'app) et la carte du dashboard admin — pas de recherche de plus-proche-voisin en temps réel, puisque la
détection de proximité tourne côté app mobile (`2026-08-12-guide-runtime-v1-scope-design.md`).

## Ports

Un repository par agrégat, plus la sortie vers la queue :

- `PlaceRepository` — persistance et lecture de Place (y compris requêtes par zone géographique)
- `ScriptRepository` — persistance et lecture de Script
- `AudioFileRepository` — persistance et lecture d'AudioFile
- `AudioJobPublisher` — port sortant vers RabbitMQ, appelé quand un Script passe à `reviewed`

## Structure de dossiers

Hexagone classique, découpé par couche technique (choisi plutôt qu'un mini-hexagone par contexte
métier : plus simple à tenir en tête avec seulement 3 agrégats au démarrage, structure la plus
directement reconnaissable comme "architecture hexagonale" dans les références Go du domaine) :

```
internal/
  domain/         Place, Script, AudioFile — entités, invariants, purs, zéro dépendance
  application/    use cases (PublishScript, RequestAudioGeneration...) — orchestrent le domaine
  ports/          interfaces (PlaceRepository, ScriptRepository, AudioJobPublisher...)
  adapters/
    postgres/     implémentations des repositories
    rabbitmq/     implémentation du publisher de jobs TTS
cmd/api/          point d'entrée
```

## Frontière de collaboration (spécifique à ce sous-système)

Rappel explicite déjà présent dans `mission.md` : le backend est "written by hand, not AI-generated".
En pratique pour ce projet : le squelette ci-dessus (dossiers, fichiers vides, `go.mod`) est posé une
seule fois par Claude pour éviter la friction de setup pure — zéro logique, zéro signature de fonction,
zéro commentaire métier à l'intérieur. Tout contenu réel (structs, invariants, implémentations) est écrit
à la main. Le rôle de Claude sur ce sous-système se limite à expliquer des concepts (avec des exemples
jetables, génériques, hors du repo), répondre aux questions de compilation/debug, et relire le code une
fois écrit — jamais l'écrire à la place de l'autrice. Le futur plan d'implémentation (prochaine étape)
est donc un guide à suivre, pas une liste de tâches à exécuter automatiquement.

## Ce qui reste ouvert (hors scope de ce document)

- Partners et users — schéma et invariants à définir séparément.
- Réconciliation Place entre édition manuelle et réimport pipeline — différé jusqu'à occurrence réelle.
- Détail des use cases applicatifs (validation, erreurs) — à préciser dans le plan d'implémentation.
