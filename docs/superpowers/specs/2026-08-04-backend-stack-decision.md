# Rio Audio Guide — Décision : stack backend et portfolio

Ce document tranche un point qui n'était pas encore décidé dans `2026-07-21-rio-audio-guide-design.md`
ni dans `2026-07-23-roadmap-v2-agentic-architecture.md` : **la stack technique précise du backend**
(sous-système 5, jusqu'ici "détaillé au moment venu"). Il ne remplace ni l'un ni l'autre.

## Contexte de la décision

En cherchant un projet portfolio pour une candidature (Senior Software Engineer, Powens, équipe EMI —
Go, hexagonal, DDD, Kubernetes, RabbitMQ, SQL/NoSQL, Redis, Kafka, Docker), une piste a été explorée
dans une conversation séparée : construire un projet fintech dédié (ledger de paiements) pour coller
exactement à la fiche de poste. Cette piste a été écartée — la fintech n'est pas un intérêt réel, et un
projet qu'on ne trouve pas intéressant ne se termine pas.

La piste retenue à la place : **l'audioguide reste le projet unique**, et son backend (jamais construit
à ce jour — seul le pipeline Python de sourcing/curation existe) est conçu pour couvrir légitimement les
compétences ciblées, sans ajouter de technologie qui ne sert pas un vrai besoin du produit.

**Point de vigilance explicite** : une proposition initiale pour ce backend listait Postgres + MongoDB +
Redis + RabbitMQ + Kafka + Kubernetes simultanément, présentés comme des besoins du produit alors qu'ils
étaient choisis pour matcher une liste de mots-clés de fiche de poste. Ce risque est nommé ici pour ne
pas se reproduire : une techno qui ne se justifie qu'en entretien par "c'était dans l'offre d'emploi" est
disqualifiante, pas valorisante, pour un poste qui évalue explicitement le "sound judgment" — et contredit
le principe déjà posé dans le design doc v1 (hébergement pragmatique, ne pas sur-dimensionner pour un
produit naissant).

## Stack backend retenue

| Brique | Décision | Justification produit réelle |
|---|---|---|
| **Go**, architecture hexagonale + DDD | Retenu (déjà dans le design doc v1) | Domaine réel et non trivial : workflow de publication de contenu (voir invariants ci-dessous) |
| **PostgreSQL + PostGIS** | Retenu (déjà dans le design doc v1) | Requêtes géospatiales natives ("lieux à moins de N mètres"), stockage des métadonnées (lieux, scripts, statut de relecture) |
| **RabbitMQ** | Retenu (nouveau) | File de tâches TTS : génération audio longue (jusqu'à ~40s/fichier), coûteuse, avec retries et DLQ — sémantique tâche/file, pas événementielle |
| **Redis** | Écarté pour l'instant (revu le 2026-08-04) | Voir section dédiée ci-dessous |
| **Kubernetes + Helm + AWS (EKS ponctuel)** | Retenu (déjà dans le design doc v1 — "démo technique EKS, prod réelle Scaleway") | Rien de nouveau à justifier ; KEDA peut scaler les workers TTS sur la profondeur de file plutôt que sur le CPU (cas d'usage réel, pas artificiel) |
| **Docker** | Retenu | Prérequis de tout ce qui précède |

## Explicitement écarté (et pourquoi)

- **Apache Kafka** — un deuxième broker en plus de RabbitMQ, pour une seule application à volume naissant,
  n'a pas de justification produit. RabbitMQ seul couvre le seul besoin réel identifié (file de tâches TTS).
  Un event-streaming pour la télémétrie d'écoute pourrait être un vrai besoin un jour, mais ce jour n'est
  pas venu — à reconsidérer seulement si un besoin d'analytics temps réel apparaît concrètement en usage.
- **MongoDB** — le besoin invoqué ("stocker les payloads bruts hétérogènes OSM/Wikidata") existe, mais un
  type `JSONB` dans PostgreSQL le couvre déjà, dans la même base que le reste, sans opérer un second
  système de stockage. Ajouter une base NoSQL ici serait une démonstration de technologie, pas une décision
  d'architecture.

**Principe retenu pour trancher ce genre de cas à l'avenir** : une techno n'entre dans le scope que si un
besoin produit concret l'exige *et* qu'aucune brique déjà présente ne le couvre aussi bien. Sinon, elle
reste documentée ici comme option écartée, avec la raison — pas ajoutée "pour le portfolio".

## Redis — revu et écarté (2026-08-04)

Une première version de ce document gardait Redis (cache des requêtes géo chaudes, rate limiting),
jugé "petit coût, cas d'usage réel". En relisant ce choix à la lumière du même principe que pour
Kafka/MongoDB, il ne tient pas mieux qu'eux : **il n'y a aujourd'hui aucune requête mesurée lente et
aucun trafic réel à rate-limiter** — le produit n'a pas encore d'utilisateurs. C'est le même biais
que Kafka/MongoDB sous une forme plus discrète (un "petit coût" reste un coût, et une brique de plus
à opérer, tester, et expliquer en entretien si elle n'a jamais servi à rien de concret).

PostGIS seul encaisse largement le volume attendu au lancement (2230 lieux, requêtes géo simples).
Redis est retardé, pas supprimé : à ajouter seulement quand un besoin mesuré apparaît (une requête
identifiée comme lente en usage réel, ou un abus constaté sur un endpoint public) — jamais de manière
anticipée. C'est cohérent avec la recherche du même jour sur le jugement IA/dev
(`2026-08-04-ai-judgment-research.md`) : la documentation GitHub citée y insiste sur la vérification
du besoin réel avant d'accepter une suggestion, IA ou non — le même standard s'applique à mes propres
propositions passées, pas seulement à celles d'un outil IA.

## Le domaine (pourquoi le DDD n'est pas décoratif ici)

Le bounded context central du backend est le **workflow de publication de contenu**, avec des invariants
réels, découverts par l'expérience pendant la construction du pipeline de sourcing/curation, pas inventés
pour l'exercice :

- Un lieu ne se publie pas sans relecture humaine (règle déjà en vigueur dans le pipeline de contenu,
  voir `2026-07-23-roadmap-v2-agentic-architecture.md`, "zéro-hallucination").
- Une variante linguistique ne se publie pas sans audio généré associé.
- La déduplication entre sources reste conservatrice — l'expérience réelle du pipeline (ex. les faux
  positifs du juge anti-hallucination, la nécessité de ne jamais fusionner automatiquement en cas
  d'ambiguïté) a montré concrètement le coût d'une fusion trop agressive.

## Ce qui n'existe pas encore (correction d'une confusion)

Pour éviter de répéter une inexactitude ailleurs : à la date de ce document, **aucun code Go, aucune base
de données montée, aucune file de messages et aucune intégration TTS n'existent dans le repo.** Seuls le
pipeline de sourcing (`pipeline/sourcing/`, branche `sourcing-pipeline`) et les scripts de curation/
narration Python existent réellement. Tout le contenu de ce document est un plan, pas un état des lieux.

## Séquencement (mise à jour de celui du roadmap v2)

Le séquencement du roadmap v2 reste valable ; ce document précise seulement le contenu technique du
sous-système 5 (ici renommé backend, distinct du dashboard admin/app mobile) quand il sera attaqué :

1. Compléter le corpus de contenu (sous-système 2 — en cours).
2. Guide runtime agentique (sous-système 3) — reste la priorité d'apprentissage la plus haute, à écrire
   à la main en premier.
3. Backend (Go/hexagonal/DDD/Postgres+PostGIS/RabbitMQ/Redis/K8s, ce document) — écrit à la main pour
   les mêmes raisons : c'est la partie directement évaluée par la fiche de poste visée.
4. Ops en profondeur (sous-système 4), dashboard admin + app mobile (sous-système 5 restant).

## Kafka et Redis — déclencheurs de reconsidération, précisés (2026-08-16)

Reconsidérés une nouvelle fois le 2026-08-16 (préparation entretien Powens) et écartés à nouveau, pour
la même raison qu'au 2026-08-04. Ce qui manquait jusqu'ici : un déclencheur assez précis pour savoir
*quand* revenir dessus, pas juste "un jour peut-être". Précisé ici pour ne pas avoir à redécouvrir le
raisonnement à chaque fois que la question revient (elle est déjà revenue trois fois) :

- **Kafka** — déclencheur : un besoin réel de télémétrie d'écoute (quels lieux sont écoutés, dans quelle
  langue, quand) avec **plusieurs consommateurs indépendants** du même flux (ex. moteur de
  recommandation + dashboard analytics + déclencheur marketing, chacun lisant le flux à son propre
  rythme, avec besoin de rejouer l'historique). RabbitMQ ne couvre pas bien ce cas (pas de rejeu, un
  seul consommateur prévu par message) — Kafka le couvrirait légitimement. N'existe pas tant qu'il n'y
  a pas d'utilisateurs réels ni de pipeline de télémétrie construit.
- **Redis** — deux déclencheurs séparés, l'un ou l'autre suffit : (1) une requête identifiée comme
  **mesurée lente** en usage réel (pas supposée lente) sur les lieux/recherche géo — cache avec TTL ;
  (2) un **abus constaté** (pas anticipé) sur un endpoint public une fois l'API exposée — rate limiting.
  PostGIS seul suffit tant qu'aucun des deux n'est mesuré.

Aucun des deux déclencheurs n'est atteint à cette date — zéro utilisateur réel, zéro trafic public,
zéro requête mesurée lente. Documenté ici précisément pour qu'une future relecture (la mienne ou un
entretien) trouve une réponse vérifiable plutôt qu'un principe vague.
