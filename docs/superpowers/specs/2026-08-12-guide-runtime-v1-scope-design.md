# Rio Audio Guide — Décision : scope v1 du guide runtime et frontière de service

Ce document tranche un point resté flou entre `2026-07-21-rio-audio-guide-design.md` et
`2026-07-23-roadmap-v2-agentic-architecture.md` : ce que contient réellement le "guide runtime" en v1,
et s'il justifie un service serveur séparé (avec la question de langage que ça soulèverait). Il ne
remplace ni l'un ni l'autre — il résout une tension entre les deux.

## Le problème

`mission.md` contenait deux affirmations contradictoires au même niveau de confiance :
- Scope v1 : "**live chat/Q&A** explicitement hors scope".
- Architecture at a glance : "Guide runtime (the agentic core) ... **Not started. Highest learning
  priority**", décrit par le roadmap v2 avec une composante "Questions imprévues" — c'est-à-dire le
  chat Q&A que la ligne précédente exclut.

Le roadmap v2 avait réintroduit l'ambition agentique complète (mémoire, tool calling, human-in-the-loop,
mode vocal, questions improvisées) sans mettre à jour la ligne de scope du design doc v1, qui avait
pourtant déjà tranché explicitement d'écarter le chat live comme feature creep (`2026-07-21-rio-audio-
guide-design.md:94` : "Décision volontaire de ne pas le forcer dans le produit juste pour 'montrer du
RAG' — à reconsidérer en Phase 2 si un vrai besoin utilisateur émerge").

## Décision 1 — scope du guide runtime v1

**Le guide runtime v1 se limite à la proximité GPS + la mémoire de tour.** Pas de questions
improvisées, pas de tool calling, pas de human-in-the-loop en v1. Fonctionnellement : à l'approche d'un
lieu, proposer sa narration (dans la langue/voix choisie), retenir ce qui a déjà été écouté pour éviter
les répétitions.

Le Q&A live reste documenté comme piste Phase 2, réactivable seulement si un vrai besoin utilisateur
émerge — ce qui était déjà la position du design doc v1, jamais formellement contredite, seulement
recouverte par l'ambition du roadmap v2.

## Décision 2 — frontière de service

**Pas de service serveur séparé pour le guide runtime en v1.** Cette logique vit dans l'app mobile
(React Native), pas dans un nouveau sous-système backend.

Raison déterminante : le mode hors-ligne est déjà **requis, pas optionnel**, pour v1
(`2026-07-21-rio-audio-guide-design.md:89-91`) — le touriste télécharge son parcours au wifi, puis "la
détection de proximité tourne ensuite entièrement en local" (`react-native-background-geolocation`,
sans réseau). Une fois le Q&A écarté (décision 1), il ne reste plus de besoin qui exigerait un aller-
retour serveur au moment de l'écoute. Créer un service serveur pour ça irait à l'encontre d'une
contrainte produit déjà actée.

Le backend Go n'est pas affecté dans son périmètre : il reste responsable de fournir les données
téléchargeables (lieux, scripts, audio, timestamps karaoké) et du workflow de publication — exactement
le rôle déjà défini dans `2026-08-04-backend-stack-decision.md`. Aucune logique de guide runtime ne lui
est ajoutée ni retirée : elle n'y a jamais résidé.

## Décision 3 — coût d'outillage polyglotte

Sans objet pour v1, puisqu'il n'y a pas de second service à écrire. **Fermée, pas résolue** : à rouvrir
seulement si/quand un besoin réel de Q&A live en Phase 2 nécessite un service côté serveur — le choix de
langage et la frontière se retrancheront alors avec de vraies contraintes, pas par anticipation. Même
principe que celui déjà appliqué à Redis/Kafka dans `2026-08-04-backend-stack-decision.md`.

## Conséquences documentaires

- `mission.md`, table "Architecture at a glance", ligne "Guide runtime" : à corriger — ce n'est plus un
  sous-système serveur "TBD at build time", c'est une feature de l'app mobile, scope v1 limité à
  proximité + mémoire.
- `mission.md`, séquencement : le guide runtime cesse d'être une phase serveur à part qui bloquerait le
  backend ou en dépendrait — les deux peuvent avancer indépendamment, l'app ayant seulement besoin que
  le backend serve de vraies données à un moment donné.
- Le chat Q&A live reste dans "Hors scope v1" sans changement — cette décision ne fait que retirer la
  contradiction, pas rouvrir le sujet.

## Ce qui reste ouvert

- La conception précise de la mémoire de tour côté app (stockage local, structure de données) — sujet
  d'implémentation, pas de ce document.
- Le déclenchement exact de la Phase 2 (quel signal utilisateur justifierait de relancer le Q&A live) —
  non défini, volontairement, pour éviter de retomber dans le biais déjà nommé dans
  `2026-08-04-backend-stack-decision.md` (construire par anticipation plutôt que sur besoin mesuré).
