# Rio Audio Guide — Lecture synchronisée du texte façon "paroles Spotify"

## Contexte

L'intégration Amazon Polly (voir `2026-08-26-aws-polly-integration-design.md`) fait remonter pour la
première fois de vrais timestamps mot-par-mot (`timestamps_url`, un fichier NDJSON écrit par Polly à
côté du MP3, un objet JSON par mot : `{"time":ms,"type":"word","start":..,"end":..,"value":"..."}`).
Cette donnée existe déjà côté domaine (`GeneratedAudio.timestampsURL`) et est persistée en base depuis
le premier test réel (Cristo Redentor, 26/08), mais rien ne l'exploite encore : ni une route HTTP pour
l'exposer au client, ni une UI pour l'afficher. C'était noté "follow-up ticket, pas un blocage" dans la
review finale de l'intégration Polly — ce document couvre ce follow-up.

Objectif produit : pendant la lecture audio d'un lieu, surligner le mot en cours dans le texte de la
narration (mobile, écran `PlaceDetail`), avec un défilement automatique façon paroles synchronisées
(Spotify) — sans interaction de type tap-to-seek pour cette première version.

## Décisions actées (brainstorming du 26/08)

- Le surlignage **remplace** l'affichage statique actuel du texte, pas un mode alterné.
- **Défilement automatique** activé — l'effet "paroles synchronisées" demandé explicitement.
- **Pas de tap-to-seek** dans cette version — surlignage passif uniquement, pourra être ajouté plus
  tard sans changer l'architecture ici (le mapping mot → timestamp existera déjà).
- **Fallback silencieux** : narration sans timestamps (anciennes générations ElevenLabs, échec de
  fetch/parsing) → texte statique affiché exactement comme aujourd'hui, zéro régression.
- **Le texte affiché suit les marks, pas `place.body`** (détail technique clé, voir plus bas) — décidé
  pendant le design, pas une question ouverte.

## Backend — exposer `timestamps_url`

`internal/adapters/http/audio_handler.go`, `getPlaceAudio` :

```go
type audioResponse struct {
	URL           string  `json:"url"`
	TimestampsURL *string `json:"timestamps_url,omitempty"`
}
```

Après avoir présigné l'URL audio (code existant, inchangé), si `audioFile.Audio().TimestampsURL()` est
non vide : `parseS3Key` (déjà présent, réutilisé tel quel) puis `s.storage.PresignURL` avec le même
`presignExpiry` (15 min), résultat assigné à `TimestampsURL`. Si vide (générations ElevenLabs
d'avant, ou tout audio sans marks) : champ omis (`omitempty`), la réponse est identique à aujourd'hui
— **aucune régression pour l'existant**.

Le cache (`s.cache.Set`/`Get` sur la clé `audio:<placeID>:<language>`) contient déjà le JSON de
réponse tel quel — il met en cache la nouvelle forme automatiquement, rien à changer côté cache.

**Tests** : étendre `audio_handler_test.go` — un cas avec `timestampsURL` renseigné (vérifie la
présence et le presign du champ), un cas sans (vérifie l'absence du champ, pas juste une valeur vide —
`omitempty` doit vraiment omettre la clé JSON, pas la mettre à `null`).

## Mobile — récupération des données

`src/data/types.ts` : `AudioAvailability`'s état `"ready"` gagne un champ optionnel :

```ts
export type AudioAvailability =
  | { state: "ready"; url: string; timestampsUrl?: string }
  | { state: "pending" }
  | { state: "unavailable" };
```

`src/data/PlacesRepository.ts`, `getAudioUrl` (implémentation HTTP réelle) : lit
`body?.timestamps_url` en plus de `body?.url`, le passe tel quel dans le `{state: "ready", ...}`
renvoyé — absent si absent, pas de valeur par défaut inventée.

Nouvelle fonction pure + IO séparées, `src/utils/syncedText.ts` (voir plus bas pour le détail des
fonctions pures) : `fetchWordMarks(url: string): Promise<WordMark[] | null>` — télécharge le texte
brut du fichier NDJSON, le passe à `parseWordMarks` (pure). Toute erreur (réseau, parsing) est
attrapée et transformée en `null`, jamais une exception qui remonte à l'appelant — c'est ce qui fait
fonctionner le fallback silencieux décidé plus haut sans état d'erreur séparé à gérer côté composant.

## Le point technique clé : le texte suit les marks, pas `place.body`

Les offsets `start`/`end` de chaque mark pointent dans le texte **SSML** envoyé à Polly
(`<speak><prosody rate="90%">...texte...</prosody></speak>`, voir
`internal/adapters/awspolly/generator.go`), pas dans le texte brut `place.body` récupéré séparément
depuis Postgres. Essayer de réaligner ces offsets sur `place.body` serait fragile (décalage constant
dû aux balises d'ouverture, en plus de tout écart de espaces/ponctuation entre ce que Polly a
effectivement synthétisé et le texte source).

À la place : **quand les marks sont disponibles, le texte affiché est reconstruit directement à partir
d'elles** — `marks.filter(m => m.type === "word").map(m => m.value).join(" ")` en résumé (la vraie
implémentation garde les marks individuelles, pas juste le texte joint, pour le surlignage mot par
mot). C'est la transcription exacte de ce que Polly a réellement lu et chronométré, donc par
construction toujours alignée avec les timestamps. `place.body` reste la seule source utilisée avant
le début de la lecture et dans le cas fallback (pas de marks) — zéro changement dans ce cas, exactement
le rendu actuel.

## Logique pure — `src/utils/syncedText.ts`

```ts
export type WordMark = { time: number; value: string };

export function parseWordMarks(ndjson: string): WordMark[] {
  // découpe par ligne, JSON.parse chaque ligne non vide, filtre type==="word",
  // ignore silencieusement toute ligne qui ne parse pas (une ligne corrompue
  // ne doit pas faire échouer tout le fichier) -- garde time/value uniquement,
  // le composant n'a pas besoin de start/end/type une fois filtré.
}

export function findActiveWordIndex(marks: WordMark[], currentTimeMs: number): number {
  // recherche binaire : le dernier indice dont marks[i].time <= currentTimeMs.
  // -1 si currentTimeMs est avant le premier mark (lecture pas encore démarrée
  // ou marks vides).
}
```

**Tests** (`src/utils/__tests__/syncedText.test.ts`, même convention que
`downloadManager.test.ts`) : `parseWordMarks` sur un NDJSON valide, sur une ligne corrompue au milieu
(doit continuer), sur une entrée vide ; `findActiveWordIndex` sur les bornes (avant le premier mot,
après le dernier, exactement sur un timestamp, entre deux).

## Composant `SyncedNarration`

Nouveau fichier `src/components/SyncedNarration.tsx`. Props :

```ts
type Props = { text: string; marks: WordMark[] | null; currentTimeMs: number };
```

- `marks === null` → rend `<Text style={styles.body}>{text}</Text>`, identique au rendu actuel dans
  `PlaceDetail.tsx` (même style, juste déplacé dans ce composant).
- `marks !== null` → calcule `activeIndex = findActiveWordIndex(marks, currentTimeMs)`, rend chaque
  mot comme un `<Text>` individuel (pas du texte imbriqué dans un seul `<Text>` parent — React Native
  n'expose pas de layout fiable par enfant pour du texte inline imbriqué) à l'intérieur d'une `<View
  style={{flexDirection:"row", flexWrap:"wrap"}}>`, avec un espace après chaque mot rendu séparément.
  Le mot à `activeIndex` porte un style de surlignage (couleur d'accent existante du thème, ex.
  `colors.terracotta`, cohérent avec le reste de l'écran — pas une nouvelle couleur à inventer).
- Chaque `<Text>` de mot a un `onLayout` qui enregistre sa position Y dans un tableau `ref`
  (`wordYPositions.current[i] = event.nativeEvent.layout.y`).
- `useEffect` sur `activeIndex` : si la position Y du mot actif sort d'une bande "confortable" autour
  du centre de la zone visible (ex. en dehors de `[scrollY + 0.3*viewportHeight, scrollY +
  0.7*viewportHeight]`), appelle `scrollViewRef.current.scrollTo({y: ..., animated: true})` pour la
  ramener dans cette bande — pas un recentrage systématique à chaque mot (qui serait saccadé), le
  "garder visible" que font les vraies UI de paroles synchronisées.

## Intégration dans `PlaceDetail.tsx`

- Remplace `<Text style={styles.body}>{place.body}</Text>` (ligne ~168 actuellement) par
  `<SyncedNarration text={place.body} marks={marks} currentTimeMs={status.currentTime * 1000} />`.
- Nouvel état `const [marks, setMarks] = useState<WordMark[] | null>(null)`, rempli par un nouveau
  `useEffect` sur `(place?.id, playerLocale, audio)` — se déclenche quand `audio.state === "ready" &&
  audio.timestampsUrl`, appelle `fetchWordMarks(audio.timestampsUrl)`, même style d'effet
  annulable (`cancelled` flag) que l'effet existant pour `getAudioUrl` juste au-dessus. Remis à `null`
  immédiatement quand `playerLocale` change (avant que le nouveau fetch ne réponde), pour ne jamais
  surligner un texte dans la mauvaise langue pendant la transition.

## Hors scope (explicitement, pour ce document)

- Tap-to-seek (mot → position de lecture) — décidé hors scope pour cette version, l'architecture ci-
  dessus (mapping marks ↔ mots déjà en place) n'empêche pas de l'ajouter plus tard.
- Téléchargement hors-ligne des timestamps — `downloadManager.ts` ne télécharge pas encore les bytes
  audio eux-mêmes (uniquement les métadonnées), donc rien à synchroniser hors-ligne pour l'instant ;
  le jour où le téléchargement audio réel existe, il faudra revisiter ce point.
- Toute autre langue/écran que `PlaceDetail` — l'écran `Assistant` ou d'autres futurs lecteurs audio ne
  sont pas dans ce périmètre.
