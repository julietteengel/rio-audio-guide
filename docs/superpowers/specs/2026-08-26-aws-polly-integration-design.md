# Rio Audio Guide — Intégration Amazon Polly (remplace ElevenLabs), timestamps mot-par-mot réels

## Contexte

L'intégration ElevenLabs réelle (19/08, voir `2026-08-19-elevenlabs-real-generation-findings.md`) a
généré 3/4 langues d'un seul lieu (Cristo Redentor) avant d'épuiser un quota de 40 000 crédits. Le
corpus réel à générer est bien plus gros que ce test : **254 lieux uniques, 4 langues chacun**
(`pipeline/curation/narrations_data_part1-6.py`, dédupliqué comme le fait
`pipeline/curation/build_narrations_multi.py`) — 4 475 674 caractères, ~736 000 mots, environ
**82 heures d'audio** au total. Au tarif ElevenLabs multilingual v2 (overage Creator, $0,10/1000
caractères), ça représente **~$448** rien que pour la synthèse audio du corpus complet — non
tenable.

Par ailleurs, les timestamps mot-par-mot (surlignage karaoké du texte pendant la lecture, prévu dès
le design initial du 21/07) n'ont **jamais été branchés** dans le code réel, malgré le domaine déjà
prêt à les recevoir : `domain.GeneratedAudio.timestampsURL` existe, et
`application.CompleteAudioGeneration` accepte déjà un paramètre `timestampsURL string` — c'est
`ports.TTSGenerator.Generate` qui ne renvoyait que `(audioBytes []byte, duration time.Duration,
err error)`, sans aucune donnée de timing.

Recherche comparative menée (ElevenLabs / Google Cloud TTS / Azure / Unreal Speech / self-hosted
type `voice-factory`/Qwen3-TTS+faster-whisper / Amazon Polly) : **Amazon Polly retenu**. Speech
Marks natifs confirmés pour le moteur Neural (pas seulement Standard) dans les 4 langues cibles
(voix `Lea`/`Remi` pour fr-FR, `Sergio`/`Lupe` pour es-ES/es-US, `Vitoria`/`Camila`/`Thiago` pour
pt-BR, plusieurs voix en-US), au tarif $16/1M caractères (Neural) + $4/1M pour les Speech Marks —
soit **~$90 pour le corpus complet**, contre ~$448 chez ElevenLabs. Le self-host (`voice-factory`,
Qwen3-TTS + alignement faster-whisper) donnait un ordre de grandeur comparable (~$70-225 en calcul
GPU brut) mais avec une vraie charge d'exploitation (pod à gérer, découpage de texte à développer —
les benchmarks du repo sont calibrés pour des clips de 12-45s, pas des narrations de 4-5 minutes) et
des timestamps de moins bonne qualité (Whisper transcrit, il n'aligne pas parfaitement — mots parfois
substitués). Écarté pour ce projet à cette échelle.

## Décisions actées

- **Feu vert ponctuel pour toucher `internal/ports/`** — même exception que celle accordée le
  19/08 pour ElevenLabs (voir `2026-08-16-elevenlabs-integration-design.md`) : valable pour ce plan
  précis, pas une autorisation permanente. `internal/domain/` reste hors limites — et il se trouve
  qu'aucun changement n'y est nécessaire (voir plus bas).
- **Bucket S3 réutilisé** : `rio-audio-guide`, déjà utilisé par le storage actuel (`S3_BUCKET` dans
  `cmd/worker/main.go`, déjà vu dans `rio-cicd-policy.json`). Pas de bucket dédié Polly.
- **ElevenLabs débranché, pas supprimé** : retiré de `cmd/worker/main.go` (plus instancié, plus
  importé), code de `internal/adapters/elevenlabs/` intact dans le repo. Il n'est pas retouché pour
  coller à la nouvelle forme du port — il devient orphelin (n'implémente plus `ports.TTSGenerator`
  après le redesign ci-dessous), ce qui n'a aucune conséquence puisque rien ne l'appelle plus via
  cette interface.
- **Voix par langue : pas décidées ici.** Comme pour ElevenLabs (hand-picked par la fondatrice, pas
  auto-sélectionnées), le `voiceID` reste un paramètre passé à l'appel — jamais codé en dur dans
  l'adaptateur. Seul le mapping *langue → code langue Polly* est verrouillé dans ce design (voir
  plus bas), pas le choix de la voix précise dans chaque langue.

## Contrainte technique clé : deux APIs Polly, pas une

- **`SynthesizeSpeech`** (synchrone, le réflexe naïf) : plafonnée à **3000 caractères facturés**.
  Sur les vraies données du corpus, **705 des 1016 combinaisons lieu×langue dépassent cette
  limite** (narration la plus longue observée : 11 378 caractères, Museu Penitenciário en espagnol).
  Inutilisable telle quelle pour la majorité du corpus.
- **`StartSpeechSynthesisTask`** (asynchrone) : jusqu'à **100 000 caractères facturés** — 9× le pire
  cas réel du corpus. C'est la seule API utilisée par cet adaptateur ; aucun découpage de texte n'est
  nécessaire, contrairement à ElevenLabs (timeout serveur sur les textes longs) ou à la piste
  self-host (timeout de 100s du proxy RunPod).
- `StartSpeechSynthesisTask` **écrit le résultat directement dans un bucket S3** fourni en paramètre
  (`OutputS3BucketName`/`OutputS3KeyPrefix`) — **c'est Polly qui fait l'upload**, contrairement à
  ElevenLabs qui renvoyait des bytes que `worker.go` uploadait ensuite lui-même via
  `ports.AudioStorage`.
- **Un seul appel ne renvoie pas audio + timestamps ensemble.** Il faut lancer **deux tâches**
  distinctes par narration : une avec `OutputFormat=mp3` (audio), une avec `OutputFormat=json` +
  `SpeechMarkTypes=[word]` (timestamps), chacune pollée séparément via `GetSpeechSynthesisTask`
  jusqu'à `TaskStatus=Completed` (ou `Failed`).
- Le fichier de marks renvoyé est du **NDJSON** (une ligne JSON par mot) :
  `{"time":360,"type":"word","start":0,"end":5,"value":"Bonjour"}` — `time` en millisecondes depuis
  le début de l'audio.

## Redesign du port `ports.TTSGenerator`

Nouvelle forme (remplace l'actuelle dans `internal/ports/tts_generator.go`) :

```go
type TTSGenerator interface {
	// Generate lance la synthèse ET l'obtention des timestamps, attend leur
	// complétion (polling interne à l'implémentation), et renvoie les URLs S3
	// finales -- plus aucun upload à faire côté worker. Contrairement à
	// l'ancien contrat (bytes en retour, upload délégué à ports.AudioStorage
	// côté worker), Polly écrit directement sur S3 : c'est l'implémentation
	// qui porte cette responsabilité désormais, pas worker.go.
	Generate(ctx context.Context, text, language, voiceID string) (storageURL, timestampsURL string, duration time.Duration, err error)
}
```

`ports.PermanentError` est inchangé (déjà générique, testé indépendamment de la forme de
`Generate` — voir `internal/ports/tts_generator_test.go`).

**Conséquence sur `worker.go`** : le bloc `uploadWithRetry(...)` (le retry local d'upload S3 gardant
`audioBytes` en mémoire pour ne jamais rappeler un fournisseur TTS payant juste pour un problème
d'upload) **disparaît entièrement**. `w.ttsGenerator.Generate` renvoie directement les deux URLs
finales, passées telles quelles à `application.CompleteAudioGeneration` — signature déjà inchangée
(`audioFileID, storageURL, timestampsURL string, duration time.Duration`), donc **zéro changement**
dans `internal/application/publish_script.go` et `internal/domain/`.

**Compromis accepté, à noter explicitement** : on perd la protection spécifique "retry d'upload S3
sans refacturer le fournisseur TTS" — avec Polly, un échec d'écriture S3 pendant la tâche asynchrone
fait simplement échouer la tâche, et une relance repart de `StartSpeechSynthesisTask` (donc re-facture
Polly, à quelques centimes près). C'était une protection dimensionnée pour un fournisseur cher
(ElevenLabs) ; au tarif Polly (~$0,02/1000 caractères), le risque financier d'un retry complet est
négligeable. Simplification assumée, pas un oubli.

## Nouvel adaptateur `internal/adapters/awspolly/generator.go`

- **Dépendance** : `github.com/aws/aws-sdk-go-v2/service/polly` (absente de `go.mod` aujourd'hui —
  seuls `config`, `credentials` et `service/s3` y sont ; même pattern d'ajout que `service/s3`).
- **Construction** : `NewGenerator(pollyClient pollyAPI, s3Client s3GetObjectAPI, bucket string)
  *Generator`, où `pollyAPI`/`s3GetObjectAPI` sont de petites interfaces locales au paquet
  (`StartSpeechSynthesisTask`/`GetSpeechSynthesisTask` ; `GetObject`) que `*polly.Client` et
  `*s3.Client` satisfont déjà structurellement — permet de tester avec un faux client Go plutôt que
  de simuler la signature de requêtes AWS via `httptest` (impossible proprement pour le SDK v2). Les
  deux clients réels se construisent depuis le même `awsCfg` (`config.LoadDefaultConfig`) déjà
  chargé dans `cmd/worker/main.go` — deux lignes de plus, pas une nouvelle chaîne de credentials. Le
  **client S3** sert uniquement à relire le fichier de marks une fois la tâche `json` terminée (pour
  calculer la durée précise, étape 6 ci-dessous) — pas à faire un quelconque upload, que Polly gère
  déjà lui-même.
- **Mapping langue → `LanguageCode` Polly**, verrouillé dans ce design (pas de voix précise, juste le
  code langue) :

  | `language` (paramètre existant) | `LanguageCode` Polly |
  |---|---|
  | `fr` | `fr-FR` |
  | `en` | `en-US` |
  | `es` | `es-ES` |
  | `pt` | `pt-BR` (pas `pt-PT` — guide de Rio/Brésil) |

- **`Generate`** :
  1. Résout `LanguageCode` depuis `language` (table ci-dessus) ; valeur inconnue → erreur immédiate,
     pas d'appel Polly (pas besoin d'attendre un 400 de leur côté pour une erreur qu'on peut détecter
     nous-mêmes).
  2. Lance les deux `StartSpeechSynthesisTask` **en parallèle** (`errgroup`, pas séquentiel — aucune
     dépendance entre les deux) : `Engine=neural`, `VoiceId=voiceID`, `LanguageCode`, `Text=text`,
     `TextType=text`, `OutputS3BucketName=bucket`, `OutputS3KeyPrefix` distinct par tâche (ex.
     `audio/{voiceID}/` pour le mp3, `timestamps/{voiceID}/` pour le json).
  3. Polle chaque tâche (`GetSpeechSynthesisTask`, intervalle de 2s) jusqu'à `TaskStatus=Completed`
     ou `Failed`.
  4. Erreurs Polly non récupérables (`LanguageNotSupportedException`, `TextLengthExceededException`,
     credentials invalides) → `ports.PermanentError` (même logique que le découpage 4xx
     d'ElevenLabs) ; erreurs réseau/polling transitoires → erreur simple, gérée par le retry générique
     déjà présent dans `worker.go` (`maxTTSAttempts`, inchangé).
  5. Construit `storageURL`/`timestampsURL` au format `s3://bucket/key` (convention déjà utilisée
     partout dans le code, ex. `s3://rio-audio-guide/abc123.mp3` dans les tests existants) en
     **parsant l'`OutputUri`** que Polly renvoie une fois chaque tâche `Completed` (URL HTTPS du
     type `https://s3.<region>.amazonaws.com/<bucket>/<clé>`) — plutôt que de reconstruire nous-mêmes
     le nom de fichier à partir de `OutputS3KeyPrefix`/`TaskId` : AWS ne documente pas publiquement
     ce schéma de nommage de façon garantie stable, alors qu'`OutputUri` est un champ contractuel de
     la réponse.
  6. **Durée** : calculée depuis le `time` (ms) du **dernier élément `type=word`** du fichier de
     marks — plus précis que l'estimation par nombre de mots (`len(text)/5 * 400ms`) utilisée par le
     stub et par ElevenLabs. Si le fichier de marks est vide ou injoignable, fallback sur l'ancienne
     estimation par mots plutôt que d'échouer toute la génération pour une donnée non critique.

## `cmd/worker/main.go`

- Retire l'import `rioaudioguide/backend/internal/adapters/elevenlabs` et la ligne
  `elevenlabs.NewGenerator(mustEnv("ELEVENLABS_API_KEY"))`.
- Ajoute `pollyClient := polly.NewFromConfig(awsCfg)` (juste après la construction de `s3Client`,
  même `awsCfg`) puis `ttsGenerator := awspolly.NewGenerator(pollyClient, s3Client, envOr("S3_BUCKET",
  "rio-audio-guide"))` — `s3Client` réutilisé, pas reconstruit (voir la note sur `s3GetObjectAPI`
  plus haut).
- `mustEnv("ELEVENLABS_API_KEY")` disparaît — le binaire ne doit plus exiger une clé qui ne sert
  plus à rien au démarrage.
- **Aucun changement Helm nécessaire** : `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`/
  `AWS_SESSION_TOKEN`/`AWS_REGION` sont déjà présents dans `worker-deployment.yaml` (optionnels,
  pour `kind` local — IRSA sur EKS réel), et `S3_BUCKET` est déjà le bucket réutilisé. La variable
  `ELEVENLABS_API_KEY` du secret Helm peut rester (elle devient juste inutilisée) ou être retirée du
  chart — au choix, sans urgence.

## Hors scope

- Chunking de texte : pas nécessaire, marge ×9 confirmée sur le pire cas réel.
- Choix des `VoiceId` définitifs par langue : décision fondatrice au moment de tester, comme pour
  ElevenLabs — ne bloque pas l'écriture de l'adaptateur (`voiceID` reste un paramètre d'appel).
- **Permissions IAM du rôle runtime** (`polly:StartSpeechSynthesisTask`, `polly:GetSpeechSynthesisTask`,
  et écriture S3 sur `rio-audio-guide` si ce n'est pas déjà couvert) : à ajouter côté AWS par la
  fondatrice, hors code — `rio-cicd-policy.json` actuel ne couvre que ECR/S3/EKS pour le pipeline CI,
  pas ce rôle runtime.
- Migration de l'audio déjà généré chez ElevenLabs (Cristo Redentor, 3/4 langues `ready` en base) :
  reste tel quel, pas re-généré.
- Import CSV → Postgres du reste du corpus (`cmd/import`, déjà construit le 16/08) : hors scope,
  déjà existant et indépendant de ce changement.

## Tests

- `internal/adapters/awspolly/generator_test.go` : faux `pollyAPI`/`s3GetObjectAPI` écrits à la main
  (pas de serveur `httptest` — le SDK v2 ne s'y prête pas proprement, contrairement à l'appel REST
  direct d'`elevenlabs.Generator`) — vérifie le mapping langue→LanguageCode, le lancement parallèle
  des deux tâches, le polling jusqu'à complétion, le parsing NDJSON des marks (durée = dernier
  `time`), le fallback sur l'estimation par mots si le fichier de marks est vide, et le découpage
  transitoire/permanent des erreurs.
- `internal/adapters/rabbitmq/worker_test.go` : les fakes existants (`fakeTTSGenerator`,
  `onceFailingTTSGenerator`, `alwaysFailingTTSGenerator`, `failingTTSGenerator`,
  `countingTTSGenerator`) sont réécrits pour la nouvelle signature à 4 valeurs de retour. Le test
  d'upload-retry-sans-re-facturation (`countingTTSGenerator` + `failingStorage`, ~ligne 432-541) est
  **supprimé**, pas adapté — le scénario qu'il vérifiait (retry d'upload indépendant du TTS) n'existe
  plus dans la nouvelle architecture (voir compromis accepté plus haut).
