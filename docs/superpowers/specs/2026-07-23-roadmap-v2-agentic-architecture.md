# Rio Audio Guide — Roadmap v2 (architecture agentique + couverture ville entière)

Ce document met à jour `2026-07-21-rio-audio-guide-design.md` sur deux points qui ont changé depuis :
périmètre géographique (25 lieux Santa Teresa/Lapa → toute la municipalité de Rio) et architecture
du contenu/backend (décision batch vs runtime agentique). Le design doc v1 reste valable pour tout
le reste (vision, budget, sources, concurrents) — ce document ne le remplace pas, il le complète.

## Pourquoi ce document existe

En comparant une méthodologie générique d'apprentissage agentique (roadmap LinkedIn à 12 étapes :
async → LLM → tool calling → mémoire → single-agent → multi-agent → human-in-the-loop → éval →
observabilité → sécurité → prod → portfolio) avec ce qu'on construit réellement, un point s'est
clarifié : **il y a deux systèmes très différents dans ce projet, qui ne méritent pas le même niveau
d'ambition agentique.**

## Principe directeur : batch vs runtime

| | Pipeline de contenu (batch) | Guide runtime (dans l'app) |
|---|---|---|
| Fréquence d'exécution | Quelques fois (génération initiale, ajout de lieux, régénération d'une langue) | En continu, par utilisateur, pour toujours |
| Ce qu'il fait | Lieu → grounding → narration → traduction | Conversation avec le touriste : mémoire de la visite, questions imprévues, voix |
| Architecture justifiée | Pipeline propre, async, + une étape de jugement anti-hallucination | Agent avec mémoire, outils, orchestration |
| Étages de la roadmap générique concernés | 1 (async), 8 (éval) | 3 (tool calling), 4 (mémoire), 5-6 (agent/orchestration), 7 (human-in-the-loop) |

**Ne pas suragentifier le batch.** Un agent ReAct autour d'un enchaînement fixe
(geosearch → si vide, OSM → si vide, IPHAN) est de la sur-ingénierie : une fonction simple est plus
claire, plus rapide, moins chère. La valeur pédagogique/portfolio de l'agentique se trouve dans le
guide runtime, pas dans la génération de contenu.

## Sous-système 1 — Sourcing (FAIT)

Pipeline Python (`pipeline/sourcing/`) : Overture Maps + Wikidata (IPHAN) + registre feiras,
déduplication, filtrage géographique. Étendu depuis à toute la ville de Rio (pas seulement
Santa Teresa/Lapa). Résultat après audit de pertinence (2026-07-22) : 2251 lieux candidats
nettoyés (doublons fusionnés, entrées hors zone retirées, coordonnées corrigées), dont 78 ont
une narration complète FR/EN/ES/PT.

## Sous-système 2 — Pipeline de contenu (batch)

**Statut au 2026-07-24 (fin de journée) : 331/2231 lieux narrés et traduits en 4 langues
(FR/EN/ES/PT-BR), synchronisés dans `curation/places_clean_v9.csv` et sa copie Bureau. Une première
passe de vérification anti-hallucination a été faite (voir plus bas) et a corrigé 5 problèmes réels.
Détail : 218 via grounding Wikipedia (SPARQL en masse + validation), ~113 via recherche web ciblée
sur les petits centres culturels. Reste ~204 centres culturels à traiter par recherche web,
actuellement bloqué par le quota de recherche web de la session — voir plus bas. Méthodologie de
sourcing du grounding revue en profondeur le 2026-07-23 — voir ci-dessous, ça remplace l'architecture
initialement prévue à base de requêtes Wikipedia une par une.**

### Grounding — passer d'une requête par lieu à une requête pour toute la ville

L'approche initiale (`enrich_grounding_geosearch.py`, une requête Wikipedia `generator=geosearch`
par lieu) s'est heurtée aux nouvelles limites de débit Wikimedia 2026 (voir
[Wikimedia_APIs/Rate_limits](https://www.mediawiki.org/wiki/Wikimedia_APIs/Rate_limits)) : un
User-Agent non conforme (sans contact) fait basculer les requêtes dans un palier anti-scraping très
restrictif. Deux bugs supplémentaires ont été trouvés et corrigés dans ce script :
- `generator=geosearch` combiné à `prop=coordinates` ne renvoie **pas** de champ `dist` (contrairement
  à `list=geosearch`) — le tri par proximité était donc cassé, retombant toujours sur 9999m. Corrigé
  en calculant la distance réelle (haversine) entre les coordonnées du lieu et celles de l'article
  retrouvé.
- Sans `exlimit`, l'API `extracts` ne calcule le texte que pour le premier titre d'un lot de plusieurs
  — les autres reviennent avec un extrait vide. Corrigé en ajoutant `exlimit=max` et en limitant les
  lots à 20 titres (le plafond réel du module TextExtracts).
- Sans recoupement de nom entre le lieu et le titre de l'article trouvé, le "candidat le plus proche"
  peut être un bâtiment voisin totalement différent (risque de mauvaise attribution) — corrigé en
  n'acceptant un match que s'il y a un vrai recoupement de mot, ou une quasi-colocalisation (<20m).

**La bascule qui a vraiment débloqué la couverture** : au lieu d'interroger Wikipedia lieu par lieu
(des milliers de requêtes), une seule requête **SPARQL** sur le Wikidata Query Service
(`query.wikidata.org/sparql`, service `wikibase:around`, rayon 30km centré sur Rio) récupère en une
fois tous les QID Wikidata de la zone qui ont un article Wikipedia (`FILTER(BOUND(?artPT) || ...)`),
avec labels/descriptions natifs en pt/fr/en/es. Une jointure spatiale locale (Python, haversine
<100m) contre le CSV donne ensuite directement les correspondances. Résultat : 1398 entités
récupérées en 2 requêtes (la première a essuyé un vrai 429 "panne WDQS active, 1 req/min" — pas lié
au Wikimedia rate-limiting general, un problème d'infra ponctuel côté Wikimedia), 465 lieux du CSV
matchés à moins de 100m, 204 retenus après validation stricte par recoupement de nom.
Les textes d'extraits sont ensuite récupérés en lots de 20 titres (même correctif `exlimit`), pas un
par lieu.

Piste alternative documentée mais non testée pour la suite : Overpass (OSM), infrastructure
indépendante de Wikimedia, POI avec tags `wikidata`/`wikipedia` en masse sur une bbox — utile si le
WDQS retombe en panne. QLever comme endpoint SPARQL alternatif si `query.wikidata.org` est
indisponible.

### Recherche web ciblée pour les petits centres culturels/galeries sans Wikipedia

Beaucoup de petites galeries et centres culturels locaux (catégorie `cultural_center`, 318 lieux
sans correspondance Wikidata) n'ont tout simplement aucun article Wikipedia — pas une lacune de la
méthode, une réalité de la source. Le registre officiel municipal
("Estabelecimentos Culturais Municipais", ArcGIS Feature Service de la Prefeitura do Rio) existe
mais est verrouillé par authentification, non accessible publiquement.
Pour ces lieux, la seule option reste la recherche web individuelle (agents dédiés, un lot de ~30
lieux par agent, recherche + rédaction si source crédible trouvée, skip sinon). Taux de succès réel
observé sur les 3 premiers lots testés (2026-07-23) : environ deux tiers des lieux ont une source
exploitable (site officiel, réseau social, presse, registre culturel) — bien plus haut que redouté.

**Contrainte découverte le 2026-07-23, contournée le 2026-07-24** : le budget de recherche web
(`CLAUDE_CODE_MAX_WEB_SEARCHES_PER_SESSION`, ~200 requêtes) est partagé entre tous les agents d'une
même session, et s'épuise vite dès qu'on lance plusieurs lots de recherche en parallèle — pas
uniquement lié au volume d'un lot donné, c'est un quota total pour toute la session, cumulatif entre
agents. Une réinitialisation annoncée "10pm (Europe/Paris)" ne s'est pas produite comme attendu (le
lendemain, budget toujours à 0/200 dès le départ). Cause exacte non identifiée, pas un réglage
ajustable depuis l'agent lui-même.

**Contournement trouvé** : quand l'outil `WebSearch` est épuisé, `WebFetch` directement contre des
pages de résultats de moteurs de recherche (budget séparé) fonctionne comme repli.
Efficacité observée par ordre décroissant, mais tous finissent eux aussi par se faire limiter avec
l'usage cumulé sur la session (Brave 429 après un certain volume, DuckDuckGo se met à exiger un
CAPTCHA après ~10 appels) :
1. **Brave Search** (`https://search.brave.com/search?q=...`) — le meilleur au départ, mais se fait
   429 rapidement sous usage intensif.
2. **DuckDuckGo HTML** (`https://html.duckduckgo.com/html/?q=...`) — bon repli si Brave est
   limité, tient plus longtemps mais finit aussi par exiger un CAPTCHA.
3. Google, Mojeek : bloqués (consentement/403). Bing : sert une page en cache qui ignore la requête,
   inutilisable.

Conséquence pratique : espacer les lots dans le temps plutôt que tout lancer en parallèle (2-3 lots
concurrents semblent tenir, au-delà les moteurs de repli se dégradent aussi). Le faible taux de
succès sur certains lots vient surtout de la nature des lieux (petits espaces communautaires avec peu
ou pas de présence web), pas d'un manque d'effort de recherche.

### Génération de narration — le style compte autant que les faits

Première passe : narrations courtes (40-90 mots), factuelles, mais jugées **trop génériques et
répétitives** (formule d'ouverture "Vous voici..." systématique sur presque tous les lieux). Deuxième
calibrage, avec 3 exemples de référence écrits à la main et donnés tels quels aux agents de
rédaction : pas de longueur fixe (la richesse suit la densité réelle de la source — un extrait pauvre
donne une narration honnêtement courte, jamais gonflée), ouvertures variées d'un lieu à l'autre
(incongruité/observation visuelle, invitation sensorielle, angle humain, contraste passé/présent,
question, entrée in medias res — jamais deux fois la même formule), structure narrative avec un
vrai arc plutôt qu'une liste de faits mise bout à bout.

### Génération de narration — principe zéro-hallucination

Narration FR strictement à partir du texte de grounding récupéré (jamais de fait, date ou chiffre non
présent dans la source). **Un lieu sans grounding réel ne reçoit pas de narration** — ce n'est pas
une limitation technique, c'est la règle du projet. Cas particuliers rencontrés et traités : lieux
dont l'attraction principale est fermée (téléphérique du Complexo do Alemão, fermé depuis 2016 —
raconté comme histoire, jamais présenté comme visitable), lieux disparus (Morro do Castelo, rasé
dans les années 1920 — raconté comme récit, pas comme destination), lieux à l'accès incertain (Solar
das Laranjeiras, en restauration).

### Traduction EN/ES/PT

Même méthode que pour les 79 premiers lieux (registre oral, durée équivalente ±15%, noms propres non
traduits, adaptation culturelle par langue) — appliquée après stabilisation du texte source FR, pas
avant (les traductions des versions courtes ont été explicitement effacées lors du passage au style
riche, pour éviter un texte traduit désynchronisé de sa source).

### Juge anti-hallucination

Premier passage exécuté le 2026-07-24 sur les 131 lieux dont le texte source était encore disponible
(extraits Wikipedia du SPARQL en masse). Méthode : 4 agents en parallèle, chacun compare
narration_fr à source_extract phrase par phrase, verdict CLEAN/ISSUE.

**Résultat : 17 signalements, dont environ la moitié étaient des faux positifs** causés par un
défaut du harnais de vérification lui-même, pas par la narration : pour les lieux narrés à partir
d'une source différente et plus riche que celle stockée (la recherche vérifiée manuellement du
2026-07-23, ou une recherche web plus tardive), le juge comparait contre le mauvais texte de
référence. Leçon retenue : la prochaine passe de vérification doit d'abord établir quelle est la
vraie source utilisée pour chaque narration avant de la comparer, pas supposer qu'un seul fichier
d'extraits fait référence pour tout le corpus.

**Vrais problèmes trouvés et corrigés :**
- `Parque Nacional da Tijuca` : superlatif exagéré ("une des plus grandes forêts urbaines du monde"
  alors que la source dit "4e plus grande zone verte urbaine du pays") — corrigé.
- `Submarino-Museu Riachuelo S-22` : confusion entre le 7e navire (le sous-marin-musée existant) et
  le 8e (un navire différent, en construction) — corrigé.
- `Morro do Corcovado` : date (1931) ajoutée sans être dans la source fournie — corrigé (reformulé
  sans invoquer cette date précise).
- `Serra do Mendanha` : "roche volcanique" alors que la source parle de roches alcalines (syénite) —
  corrigé.
- `Casa de Cultura de Nova Iguaçu` : violation de périmètre découverte a posteriori (le lieu est
  bien à Nova Iguaçu, pas dans la municipalité de Rio) — narration supprimée.

Reste à faire : étendre la vérification aux ~200 lieux narrés sans extrait stocké (les 79 premiers +
les 20 vérifiés manuellement + les lieux trouvés par recherche web), en associant d'abord chaque
narration à sa vraie source avant de lancer le juge.

## Sous-système 3 — Guide runtime (le vrai cœur agentique)

C'est ici, et seulement ici, que l'architecture agentique se justifie pleinement — et que se
trouve la vraie différenciation par rapport à OpenRAG (qui est un RAG batch, pas un compagnon de
visite avec état). Composants :
- **Mémoire de tour** : ce que l'utilisateur a déjà entendu, sa langue, ses préférences — évite les
  répétitions, adapte le ton.
- **Questions imprévues** : le touriste demande "raconte-moi plus sur cette statue" → l'agent
  répond hors script, ancré sur le grounding du lieu (jamais d'invention hors de ce corpus).
- **Mode vocal** : Whisper en entrée, réponse vocale en sortie.
- **Garde anti-injection** : les questions utilisateur sont une surface d'attaque (prompt
  injection) — filtrage/sanitization avant qu'elles n'atteignent l'agent.

Étages de la roadmap générique concernés : 3 (tool calling structuré), 4 (mémoire), 5-6
(single-agent puis multi-agent si besoin), 7 (human-in-the-loop pour les cas incertains).

## Sous-système 4 — Ops / plateforme (déploiement, observabilité, sécurité)

**Décision explicite (2026-07-23) : construire ces étages en profondeur réelle, y compris
canary releases, stratégie de rollback et volet compliance — pas une version minimaliste.**
L'objectif du projet est autant l'apprentissage et la démonstration de compétences que la
livraison d'une app fonctionnelle ; ces étages font partie de ce qu'on veut savoir montrer.

- CI/CD avec canary release + rollback automatisé
- Observabilité distribuée (tracing coût/latence par requête, dashboards)
- Guardrails de sécurité (anti-injection, filtrage de sortie, éventuel PII redaction)
- Conformité (politique de confidentialité, gestion des données de géolocalisation)
- Déploiement K8s (EKS ponctuel pour la démo technique, VPS Scaleway pour la prod réelle — choix
  déjà tranché dans le design doc v1)

## Sous-système 5 — Admin dashboard + mobile app

Inchangé par rapport au plan initial ; détaillé au moment venu.

## Séquencement

1. **Maintenant** : compléter le corpus de contenu (sous-système 2) — lancer l'enrichissement à
   grande échelle sur les ~2173 lieux restants (throttle Wikidata/Wikipedia levé le 2026-07-23),
   générer les narrations FR pour tout ce qui obtient un grounding réel, traduire en 4 langues.
2. **Ensuite** : scaffolder le guide runtime (sous-système 3) — schémas Pydantic
   (Place/Grounding/Narration/TourState), squelette d'agent avec mémoire.
3. **En parallèle ou après** : construire la profondeur ops (sous-système 4) comme brique dédiée
   d'apprentissage/démo, pas comme après-coup.
4. **Enfin** : dashboard admin + app mobile (sous-système 5).
