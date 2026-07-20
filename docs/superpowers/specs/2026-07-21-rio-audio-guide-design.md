# Rio Audio Guide — Design Doc

## Vision

Un guide audio mains libres, multilingue (EN/PT/ES), à déclenchement géolocalisé, pour les lieux culturels et patrimoniaux de Rio de Janeiro — vendu en marque blanche à des hôtels/agences (B2B), tout en servant de vitrine technique fullstack + cloud + IA.

**Double objectif assumé** : un vrai produit testable/vendable à Rio, ET une démonstration de compétences pour candidatures/freelance. Les deux ne se recouvrent pas parfaitement (voir section IA) — c'est un choix conscient, pas un oubli.

### Ce que ce n'est pas

- Ce n'est **pas** une vitrine d'ingénierie RAG/agentique. Cette idée initiale (copilot de recherche multi-sources avec détection de contradictions, inspiré des mécanismes d'OpenRAG) a été explorée en détail mais volontairement écartée du produit final : forcer un chat RAG en direct dans un audioguide touristique aurait été du feature creep artificiel, pas un besoin réel. L'IA de ce projet est une **brique de production de contenu**, pas le produit lui-même.
- Ce n'est **pas** une copie du projet "OSM → podcast" évoqué en inspiration initiale (probablement un side-project non publié) — le concept "guide audio touristique" est une catégorie de produit établie (VoiceMap, izi.TRAVEL, GPSmyCity, Summer AI, StreetPhonia...), pas une idée propriétaire. La différenciation vient de l'exécution hyper-locale et de la distribution B2B, pas du concept.

## Analyse concurrentielle

**Passeio Carioca** (gratuit, soutenu par la mairie, ~1800 points) : gros volume mais UI traduite sans que le contenu le soit (textes en portugais malgré une interface en anglais), quasiment aucun vrai audio (guide écrit avant tout), carte saturée sans filtre par catégorie. Faible adoption réelle (23 avis App Store) malgré le soutien institutionnel.

**Concurrents internationaux** (Summer AI — explicitement "façon Pokémon Go" —, StreetPhonia, AI TourMate, MyGuide, Gamana) : le concept générique "audioguide IA géolocalisé" existe déjà et est assez peuplé. Aucun n'a de focus spécifique sur Rio avec un vrai contenu multilingue natif.

**Conclusion** : la différenciation ne peut pas être "on fait un audioguide IA" (déjà fait ailleurs), mais "traduction réelle du contenu (pas juste l'UI), audio comme expérience centrale, contenu vérifié par quelqu'un qui vit sur place, distribution B2B hôtels plutôt que B2C pur".

**Point légal important** : ne jamais scraper l'app ou la base de données d'un concurrent commercial (testé et explicitement écarté pour Passeio Carioca) — la comparaison se fait par revue manuelle d'un échantillon (captures d'écran), jamais par extraction automatisée. Les sites publics gouvernementaux (registres officiels) sont un cas différent — leur scraping en complément d'API est légitime (donnée de transparence publique, `robots.txt` vérifié permissif), voir plus bas.

## Sourcing des lieux

Aucune source unique, même officielle, ne couvre 100% des lieux culturels pertinents — vérifié empiriquement à travers 7 sources différentes. Le pipeline final est **volontairement en couches**, avec une étape manuelle permanente (pas un correctif temporaire) :

| Source | Rôle | Limite connue |
|---|---|---|
| **Overture Maps** (thème `places`, requêtable via DuckDB + httpfs/spatial sur le parquet public S3, ~15s/requête) | Source principale en volume, filtrée par catégories pertinentes (`landmark_and_historical_building`, `monument`, `museum`, `history_museum`, `art_museum`, `botanical_garden`, `national_park`, `topic_concert_venue`, `cultural_center`) | Catégorise mal certains lieux culturels informels (ex: un sanctuaire classé IPHAN étiqueté `business_advertising`) — ne jamais inclure les catégories bruit (`business_advertising`, `bar`, `restaurant`) en bloc |
| **Wikidata (SPARQL)** — biens avec désignation patrimoniale (P1435) situés à Rio (P131*) et coordonnées (P625) | 131 biens IPHAN, 392 INEPAC, 101 IRPH, 1357 à l'inventaire municipal des monuments — déjà géolocalisés, quasi gratuit en effort | Décalage avec les classements récents (un sanctuaire classé en 2022 n'y est pas encore) |
| **Registre officiel des feiras livres** (Secretaria Municipal de Ordem Pública, PDF, 165 marchés actifs) | Couvre les événements culturels récurrents (marchés hebdomadaires), angle mort total des bases de POI classiques | Adresses textuelles seulement, pas de coordonnées — géocodage requis (ex: Nominatim), et pas de nom populaire (croisement manuel nécessaire) |
| **IRPH ArcGIS** (institut du patrimoine municipal, services Feature Server publics) | 10 505+ biens protégés, utile pour croiser des adresses | Basé sur adresses (`LOGRADOURO`), pas de recherche par nom |
| **Riotur** (portail officiel tourisme) | Vérification croisée finale | Sélection curée, partielle, pas exhaustive |
| **MuseusBr / IBRAM** (registre fédéral, 3948 musées au Brésil) | Complément pour les musées manquants | Pas d'API publique accessible trouvée — navigation manuelle sur le site (`robots.txt` l'autorise) |
| **Curation manuelle (toi)** | Étape permanente, pas un pis-aller | Nécessaire même après 7 sources — cas réel testé : le Santuário do Zé Pelintra (classé Patrimoine National IPHAN 2022) n'apparaît correctement dans aucune des sources automatisées |

## Sourcing du contenu (description, histoire) — "grounded generation"

Pas de RAG classique (pas de base vectorielle, pas de recherche sur corpus) — inutile pour une liste fixe de lieux. Le mécanisme, testé en direct sur 4 lieux réels :

1. **Récupération de la source** : API Wikipedia extracts (pt, puis en en repli) par nom de lieu.
   - Résultat mesuré : Cristo Redentor (2238 caractères, riche), Escadaria Selarón (427, correct), Museu da Chácara do Céu (182, pauvre), Santuário do Zé Pelintra (0, aucun article).
2. **Classification automatique du niveau de source** (riche / correct / pauvre / absent) — la longueur/qualité de la source détermine la suite, pas l'inverse.
3. **Génération conditionnée** : uniquement si une source existe. Prompt qui contraint explicitement le LLM à n'utiliser que les faits du texte source, sans inventer, avec une longueur de script proportionnelle à la richesse de la source.
4. **Lieux sans source** (cas réel, pas rare) : **aucune génération automatique**. Direction vers une file "à rédiger manuellement" (toi, ou recherche web ciblée au cas par cas).
5. **Relecture humaine obligatoire** de tout script avant synthèse vocale, y compris ceux générés à partir de bonnes sources — passages incertains mis en évidence en priorité.

**LLM de génération** : Claude (Sonnet) ou GPT-4o, à comparer concrètement sur 3-4 lieux tests avant de trancher. Le coût du modèle n'est pas le facteur limitant vu le volume (quelques centaines de générations, one-shot) — le critère de choix est la fidélité aux faits sources (pas d'invention) et la qualité multilingue naturelle (PT/ES/EN, pas des traductions mot à mot).

**Public visé** : une seule version adulte par lieu/langue pour la v1 (pas de variantes enfant/ado). Multiplier par 3 les scripts et l'audio par lieu triplerait la charge de relecture humaine et le coût TTS pour un MVP — à reconsidérer en Phase 2 si une vraie demande émerge (ex: un partenaire hôtel famille).

## Photos

- **Photos personnelles** (Santa Teresa accessible à pied) en priorité : zéro risque légal, authenticité comme différenciateur.
- **Wikimedia Commons** en complément (licence CC BY-SA vérifiée, haute résolution confirmée en test réel — 2497×3744 sur un exemple) pour les lieux hors de portée.
- **Jamais** : scraping Google Images, Instagram. **Google Places API** : usage à la volée toléré par leurs conditions, mais export/stockage permanent en base propre interdit.

## Audio / TTS

- **MVP : API cloud payante à l'usage** (type ElevenLabs), pas de self-hosting GPU.
- **Pourquoi** : à l'échelle du MVP (~60 lieux × 3 langues × ~1500 caractères ≈ 270 000 caractères, un batch ponctuel), le calcul économique favorise nettement l'API (~30-80$ au total) contre le risque d'un pod GPU auto-hébergé laissé actif par erreur (~360-580$/mois pour rien — probablement l'origine du coût élevé mentionné par mmaudet avec `voice-factory`). Le seuil de rentabilité du self-hosting se situe vers 5-10M caractères/mois, très au-dessus du besoin MVP.
- `voice-factory` (le projet TTS auto-hébergé de mmaudet) reste une piste crédible pour une **Phase 2** (batch ponctuel sur pod éphémère, démarré/arrêté à la demande) comme démonstration DevOps supplémentaire — pas un prérequis v1.
- Langues au lancement : **anglais, portugais, espagnol**.
- **Source des voix** : clonage de voix de proches (1-2 min d'enregistrement propre par langue), via la fonctionnalité de clonage intégrée à l'API cloud déjà choisie (type ElevenLabs) — pas de self-hosting nécessaire pour ça non plus, même pipeline que la génération audio. **Nécessite un accord écrit simple de consentement** pour chaque personne enregistrée, puisqu'il s'agit d'un usage commercial (vendu aux hôtels).
- **Mécanisme de clonage** (vérifié dans la doc ElevenLabs) : clonage instantané par conditionnement, pas d'entraînement de modèle — upload de l'échantillon (`POST /v1/voices/add`) → `voice_id` disponible immédiatement, capable de parler 32+ langues. Génération ensuite par `POST /v1/text-to-speech/{voice_id}` avec le texte du script validé → MP3. Appelé une fois par lieu/langue au moment de la génération batch, jamais à l'écoute utilisateur (le MP3 est stocké, pas streamé à la demande).

## Stockage

- **Fichiers audio** : stockage objet compatible S3 — AWS S3 pour la démo technique, Scaleway Object Storage pour la production réelle (cohérent avec le choix d'hébergement). Organisation : `audio/{lieu_id}/{langue}.mp3`.
- **Métadonnées** (infos lieu, texte du script, source utilisée, statut de relecture) : PostgreSQL, dans le même backend que le reste.

## Produit / UX

- **App mobile** (React Native) avec géolocalisation temps réel, déclenchement de proximité façon "découverte" à l'approche d'un lieu, calcul d'itinéraire optionnel vers un point choisi.
- **Mode hors-ligne dès la v1 (requis, pas optionnel)** : le GPS fonctionne sans connexion data (système satellite indépendant du réseau mobile), mais l'app doit avoir en local les coordonnées des lieux, les fichiers audio et les tuiles de carte pour fonctionner pendant la balade. Le touriste télécharge son parcours/quartier au wifi (hôtel) avant de partir ; la détection de proximité tourne ensuite entièrement en local. Beaucoup de touristes à Rio n'ont pas de forfait data brésilien — ce n'est pas un nice-to-have.
  - **Carte hors-ligne** : `MapLibre React Native` (open source) — `OfflineManager` permet de télécharger une région (zoom/style/emprise configurables) à l'avance. Pattern déjà éprouvé : un guide audio existant utilise MapLibre GL JS (web) + MapLibre Native (React Native) avec tuiles servies par Martin.
  - **Géofencing en arrière-plan** : `react-native-background-geolocation` — lib la plus citée/éprouvée du secteur, détection de mouvement par accéléromètre/gyroscope pour économiser la batterie, déclenche un événement à l'entrée dans le rayon d'un lieu, entièrement en local.
- **Dashboard admin** : validation éditoriale (scripts générés vs vérifiés), gestion des partenaires (branding par hôtel/agence, marque blanche B2B), statistiques d'écoute.
- **Chat "pose une question" en direct** : explicitement **hors scope v1**. Nécessiterait du RAG live, contrairement au pipeline de génération statique ci-dessus. Décision volontaire de ne pas le forcer dans le produit juste pour "montrer du RAG" — à reconsidérer en Phase 2 si un vrai besoin utilisateur émerge.

## Backend / cloud

- **Backend** : Go + PostgreSQL/PostGIS (requêtes géospatiales "lieux à moins de N mètres").
- **Hébergement, approche pragmatique et documentée comme choix délibéré** :
  - **Production réelle** (v1, budget limité, peu d'utilisateurs) : VPS Scaleway — simple, peu coûteux.
  - **Démonstration technique K8s/Terraform/AWS** : cluster EKS ponctuel (monté pour une démo, détruit après via `terraform destroy`) ou k3s sur une instance EC2, **pas** un control plane EKS permanent (~73$/mois rien que pour le control plane, disproportionné pour un produit naissant). Présenté explicitement comme "démo technique sur EKS, production réelle sur Scaleway" — un choix d'architecture assumé, pas une incohérence.
- **Conformité** : politique de confidentialité obligatoire avant soumission aux stores (usage de géolocalisation). Budget stores : 99$/an Apple Developer Program, 25$ one-time Google Play.

## Hors scope v1 (explicite, pas oublié)

- Bars et restaurants (recentrage décidé sur lieux patrimoniaux + événements culturels récurrents uniquement)
- Chat RAG en direct pendant la visite
- Monétisation par affiliation/publicité locale (Phase 2)
- Couverture de la ville entière — démarrage sur un périmètre de quartiers restreint (Santa Teresa/Lapa en priorité)
- Infrastructure TTS auto-hébergée

## Annexe technique — accès concret aux sources, croisement, concurrents

### A. Références et méthodes d'accès (une par source, toutes testées en direct dans cette session)

| Source | Méthode d'accès | Détail technique |
|---|---|---|
| **Overture Maps** | API/requête, pas de scraping | Parquet public sur S3 (`s3://overturemaps-us-west-2/release/<version>/theme=places/type=place/*.parquet`), requêtable sans téléchargement complet via DuckDB (`INSTALL spatial; INSTALL httpfs;`) filtré par bbox + `category`. Version testée : `2026-06-17.0`, à vérifier/mettre à jour au moment de l'implémentation. |
| **OpenStreetMap (Overpass)** | API | `https://overpass-api.de/api/interpreter`, requêtes Overpass QL par tags (`tourism=*`, `historic=*`, etc.) — **superseded par Overture Maps** qui fusionne déjà OSM avec d'autres sources ; gardé en référence pour comprendre pourquoi Overture est préféré (OSM seul loupe Copacabana, Jardim Botânico...). Attention : l'endpoint `/api/status` renvoie 406 en HTTPS sans rapport avec un vrai blocage — ne pas s'y fier comme test de santé, tester directement `/api/interpreter`. Fair-use policy : espacer les requêtes, prévoir des retries. |
| **Wikidata** | API (SPARQL) | `https://query.wikidata.org/sparql` — propriétés clés : `wdt:P1435` (désignation patrimoniale), `wdt:P625` (coordonnées), `wdt:P131*` (localisation, filtrer sur `wd:Q8678` = Rio de Janeiro ville). Éviter les requêtes `CONTAINS`/`LCASE` en texte libre sur tout Wikidata (timeout) — filtrer d'abord par propriété structurée. Pour une recherche ponctuelle par nom, utiliser l'API de recherche (`action=wbsearchentities`), beaucoup plus rapide qu'un scan SPARQL. |
| **Wikipedia** | API | `https://pt.wikipedia.org/w/api.php` (`action=query&prop=extracts&exintro=1&explaintext=1&titles=<nom>&redirects=1`), repli sur `en.wikipedia.org` si vide. |
| **Registre feiras livres** | Téléchargement direct (PDF public) | `https://ordempublica.prefeitura.rio/wp-content/uploads/sites/30/2024/10/Relacao-feira-livre-atualizada.pdf` — à re-vérifier périodiquement (URL avec date, peut changer). Extraction texte + géocodage (Nominatim par adresse + quartier) requis en aval. |
| **IRPH ArcGIS** | API (ArcGIS REST, pas de clé requise pour les couches publiques) | Organisation `OlP4dGNtIcnD3RYf`. Recherche de couches : `https://www.arcgis.com/sharing/rest/search?q=orgid:OlP4dGNtIcnD3RYf AND ...&f=json`. Couche principale testée : `Bens_Protegidos_Areas_Protecao` (`.../FeatureServer/0/query?where=1=1&outFields=*&f=json`), 10 505 entrées, système de coordonnées projeté (wkid 29183/29193) — demander `outSR=4326` pour du lat/lon standard. Attention : certaines couches utiles (ex: `Equipamentos Culturais Não Municipais`) existent mais sont verrouillées par authentification municipale — non accessibles publiquement, à ne pas re-tester. |
| **Riotur** | Navigation manuelle / scraping léger toléré (site officiel de tourisme, page d'archive paginée) | `https://riotur.rio/en/o-que-fazer/museums-and-cultural-centers/` (et équivalents `/que_fazer/`) — liste curée, paginée ("ver mais"), pas d'API structurée trouvée. |
| **MuseusBr / IBRAM** | Scraping toléré (site public gouvernemental, `robots.txt` vérifié permissif : seul `/wp-admin/` est interdit) | `https://museusbr.museus.gov.br/` — pas d'API publique trouvée (le point d'entrée `dados.gov.br` testé renvoie 401). Navigation/scraping des pages de recherche public légitime. |
| **Wikimedia Commons** (photos) | API | `https://commons.wikimedia.org/w/api.php` (`action=query&list=search&srnamespace=6` pour chercher, puis `prop=imageinfo&iiprop=url|size|extmetadata` pour récupérer URL/licence/résolution). Toujours vérifier `LicenseShortName` dans `extmetadata` avant réutilisation. |

**Pistes testées et écartées** (documentées pour ne pas perdre de temps à les retester) : CDURP "Turístico e Cultural 2012" (service ArcGIS discontinué, lien mort), scraping de l'app Passeio Carioca (écarté pour raisons légales, voir Analyse concurrentielle).

### B. Méthode de croisement / déduplication entre sources

Avec 5+ sources actives, un même lieu apparaît souvent plusieurs fois sous des formes différentes (ex: "Museu Nacional" est ressorti sous 8 variantes de nom dans un seul test Overture). Processus recommandé :

1. **Normalisation des noms** : minuscules, suppression des accents, retrait des préfixes génériques ("Museu de", "Igreja de"...) avant toute comparaison.
2. **Rapprochement géographique** : pour les sources avec coordonnées (Overture, Wikidata, IRPH), regrouper les entrées à moins de ~50-100m les unes des autres comme doublons probables.
3. **Géocodage préalable** pour les sources en adresses textuelles seules (feiras, IRPH `LOGRADOURO`) — via Nominatim — avant de pouvoir appliquer le rapprochement géographique du point 2.
4. **Clé de déduplication prioritaire** : identifiant Wikidata (QID) si disponible en premier choix (source la plus fiable), sinon regroupement nom normalisé + proximité.
5. **Ne jamais fusionner automatiquement en cas d'ambiguïté** — un score de correspondance faible doit être routé vers une revue humaine plutôt que fusionné silencieusement (on a vu que la sur-confiance dans le matching automatique, comme le filtrage par catégorie, produit des faux négatifs sur des lieux réels).

### C. Concurrents — références

| Concurrent | URL | Portée |
|---|---|---|
| Passeio Carioca | *(app mobile — non vérifiée par une recherche web directe dans cette session, analysée via captures d'écran fournies par l'utilisatrice ; à confirmer l'URL/fiche store avant toute citation publique)* | Rio, gratuit, soutenu par la mairie |
| VoiceMap | voicemap.me | International, payant à l'unité |
| izi.TRAVEL | izi.travel | International, 2500 villes, mixte gratuit/payant |
| GPSmyCity | gpsmycity.com | International |
| Summer AI | summer.ai | International, façon "Pokémon Go" |
| StreetPhonia | streetphonia.com | International, déclenchement GPS auto |
| AI TourMate | aitourmate.eu | International, 14 langues |
| MyGuide | myguide.cc | International |
| Gamana | gamana.app | International |

## Points ouverts pour la suite

- Nombre et liste exacts de lieux pour le lancement v1 (quartier(s) précis à trancher)
- Qui relit les traductions ES/PT (étape non encore assignée)
- Test de géolocalisation en conditions réelles (précision GPS en terrain dense à Santa Teresa/Lapa, consommation batterie du géofencing en arrière-plan) — non encore fait, à faire tôt
- Vigilance sur la filiation CC BY-SA des textes Wikipedia utilisés en génération conditionnée (réécriture, pas copie, mais à garder à l'esprit)
