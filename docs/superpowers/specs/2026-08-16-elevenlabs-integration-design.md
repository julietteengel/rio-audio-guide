# Rio Audio Guide — Intégration ElevenLabs réelle + import Python → Postgres

Ce document couvre la tâche laissée explicitement de côté dans
`2026-08-16-backend-mvp-completion-design.md` : remplacer `generateAudioStub` par un vrai appel
ElevenLabs, et construire le pont entre la pipeline Python (sourcing + curation, narrations
multilingues déjà produites) et Postgres, dont `worker.go` a besoin pour avoir des `Script` réels à
traiter. Ne remplace ni ne modifie le design backend existant — vient s'ajouter dessus.

## Contexte

Le worker RabbitMQ (`internal/adapters/rabbitmq/worker.go`) consomme déjà `tts_jobs`, upload sur S3 et
fait la transition de domaine (`Script` → `published`) — tout fonctionne sauf l'appel TTS lui-même,
volontairement stubé faute de clé API au moment de l'écrire. La clé ElevenLabs est maintenant
disponible. Par ailleurs, aucune donnée n'existe encore en Postgres : la pipeline Python (worktree
`sourcing-pipeline`) produit déjà des lieux enrichis (`pipeline/curation/places_clean_vN.csv`) et des
narrations multilingues (fr/en/es/pt, générées par `pipeline/curation/build_narrations_multi.py`), mais
rien ne les importe dans le backend.

## Périmètre

1. Vrai appel ElevenLabs dans le worker Go (remplace le stub).
2. Commande d'import Go, CSV Python → Postgres (`Place` + `Script`).
3. Script Python ponctuel de clonage de voix ElevenLabs.

**Hors scope** (noté explicitement pour ne pas dériver) : DLQ/backoff avancé au-delà de la distinction
transitoire/permanent déjà posée ci-dessous, décodage réel du MP3 pour obtenir la durée exacte (on garde
l'estimation par nombre de mots, déjà utilisée par le stub), automatisation continue de l'import (reste
une commande manuelle, réexécutable).

## 1. Vrai appel ElevenLabs

**Nouveau port** `internal/ports/tts_generator.go` :

```go
type TTSGenerator interface {
	Generate(ctx context.Context, text, language, voiceID string) (audioBytes []byte, duration time.Duration, err error)
}
```

**Nouvel adaptateur** `internal/adapters/elevenlabs/generator.go` : appelle
`POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}` (header `xi-api-key`, lu depuis
`ELEVENLABS_API_KEY`), body `{"text": ..., "model_id": "eleven_multilingual_v2"}` — le modèle
multilingue est nécessaire puisque les scripts sont en fr/en/es/pt. La réponse est le MP3 en bytes bruts.
La durée est estimée par nombre de mots, comme le faisait `generateAudioStub` — pas de décodage MP3 pour
éviter une dépendance supplémentaire pour une donnée qui n'a pas besoin d'être exacte à la milliseconde.

**`worker.go`** : `w.ttsGenerator.Generate(ctx, job.Text, job.Language, job.VoiceID)` remplace l'appel au
stub. Deux catégories d'erreur, distinguées par code HTTP :

- **Transitoire** (429 rate limit, 5xx, timeout réseau) → `Nack(multiple=false, requeue=true)`, comme le
  reste du worker aujourd'hui pour les erreurs de repository/storage.
- **Permanente** (401 clé invalide, 400 texte/voice_id rejeté) → nouveau use case
  `application.FailAudioGeneration(ctx, audioFileRepo, audioFileID, reason string) error`, qui appelle
  `AudioFile.MarkFailed(reason)` (déjà présent dans le domaine, jamais appelé jusqu'ici) puis `Ack` pour
  arrêter la re-livraison. Sans cette distinction, une clé API invalide ferait boucler le worker à
  l'infini sur le même message.

**Config** : `ELEVENLABS_API_KEY` en variable d'env / secret K8s, même pattern que `RABBITMQ_URL` —
ajouté au secret Helm existant (`deploy/helm/rio-backend`), jamais committé.

## 2. Import CSV → Postgres

**Nouvelle commande** `cmd/import/main.go`, exécutée manuellement (pas de service, pas de queue) :

- Lit `places_clean_vN.csv` (`name, category, source, lat, lon, wikidata_qid`) et le CSV de narrations
  produit par `build_narrations_multi.py` (`name, narration_fr, narration_en, narration_es,
  narration_pt`), jointure sur `name`.
- Pour chaque ligne : `PlaceRepository.FindByName` (nouvelle méthode sur le port, absente aujourd'hui —
  nécessaire pour que réexécuter l'import soit idempotent au lieu de dupliquer des `Place`) ; si absent,
  crée via `domain.NewPlace(...)` et `Save`.
- Pour chaque colonne de langue non vide : `domain.NewScript(placeID, language, text, sourceText)` en
  statut `draft`, `Save` via `ScriptRepository`. Contrairement à `Place`, pas de garde d'idempotence côté
  `Script` (pas de `FindByPlaceIDAndLanguage` sur le port) — limitation assumée : l'import n'est sûr à
  réexécuter que pour des lieux déjà importés dont on n'a *pas* déjà généré de script (sinon doublons).
  En pratique, l'import tourne sur un CSV figé une fois par génération de contenu ; si un besoin réel de
  ré-import partiel apparaît, ajouter `FindByPlaceIDAndLanguage` à ce moment-là plutôt que par
  anticipation.
- Flag `--dry-run` : parse et affiche les comptes (lieux/scripts à créer) sans écrire.

La logique de jointure/validation des lignes est écrite en pur (given deux jeux de lignes en mémoire →
structures `Place`/`Script` à créer), testée sans toucher Postgres — même séparation pure/IO que le reste
du projet (`pipeline/sourcing/*.py` côté Python, cf. `CLAUDE.md`).

## 3. Voix ElevenLabs — clonage et voix de bibliothèque

ElevenLabs propose deux façons d'obtenir un `voice_id`, déjà distinguées dans
`2026-07-21-rio-audio-guide-design.md` :

- **Voix de bibliothèque (stock)** : des dizaines de voix prêtes à l'emploi, plusieurs langues/accents,
  utilisables immédiatement sans rien enregistrer — pratique pour tester tout le pipeline (import →
  review → `tts_jobs` → ElevenLabs → S3) avant de committer à un vrai clonage.
- **Clonage instantané (Instant Voice Cloning)** : à partir d'1-2 minutes d'audio propre (pièce calme,
  micro constant), sans entraînement de modèle. Un seul enregistrement suffit — pas besoin d'un fichier
  par langue : le modèle multilingue (`eleven_multilingual_v2`) fait parler la voix clonée dans les 32+
  langues supportées à partir du *texte* dans la langue cible (le texte, lui, reste à traduire/produire
  en amont par la pipeline Python — le clonage ne traduit rien, il ne reproduit qu'un timbre de voix).

**Script ponctuel** `pipeline/curation/clone_voice.py` (Python, `sourcing-pipeline`, non testé — même
convention que le reste de `pipeline/curation/`) : `POST /v1/voices/add` (multipart : nom, fichier audio
échantillon), imprime le `voice_id` obtenu. Ce `voice_id` est stocké en config/secret et fourni plus tard
manuellement au moment du `POST /scripts/{id}/review` — il n'intervient ni dans l'import ni dans l'appel
TTS lui-même au-delà d'être un paramètre transmis tel quel.

Rappel déjà noté dans le design v1 : usage commercial (vendu aux hôtels) → nécessite un accord écrit de
consentement simple de la personne enregistrée avant tout clonage réel.

## Tests

- Adaptateur `elevenlabs.Generator` : serveur HTTP fake (`httptest`) simulant succès/429/401, vérifie le
  bon découpage transitoire/permanent.
- `worker_test.go` étendu avec un `TTSGenerator` fake (même pattern que les fakes `storage`/`scriptRepo`
  déjà utilisés dans les tests du worker).
- `cmd/import` : la logique de jointure CSV testée en pur, séparée du `main()` qui touche Postgres.
- `clone_voice.py` : pas de couverture — script d'admin ponctuel, cohérent avec `pipeline/curation/`.
