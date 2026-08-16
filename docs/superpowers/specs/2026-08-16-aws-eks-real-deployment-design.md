# Rio Audio Guide — Déploiement AWS réel : état des lieux et ce qui reste

Ce document capture rétroactivement ce qui a été fait à la main, en direct, le 2026-08-16, pour préparer
le passage du cluster `kind` local (déjà validé, voir README backend) à un vrai cluster EKS — et ce qui
reste à faire. Écrit après coup précisément parce que ce travail ne l'avait pas été au moment où il s'est
fait, ce qui l'a rendu impossible à rejouer ou à confier à une autre fenêtre de contexte sans repartir de
zéro.

## Compte AWS

Compte réel créé (pas un AWS Academy Learner Lab — ceux-ci ont des credentials temporaires qui expirent,
inutilisables pour un CI/CD ou une démo qui dure plus de quelques heures). Compte ID `352206183080`.

## Ressources créées

- **ECR** : deux dépôts, `rio-api` et `rio-worker`, région `us-east-1`. Vides pour l'instant — aucune
  image n'y a encore été poussée (le workflow `docker-build.yml` le fait au prochain push réussi une fois
  le CI vert, voir section CI/CD plus bas).
- **S3** : bucket `rio-audio-guide`, région `us-east-1`. Contient déjà 2 fichiers audio réels (générés
  pendant les tests manuels contre le cluster `kind`, pas contre EKS — S3 est un service global/régional,
  pas lié à un cluster K8s en particulier, donc ce bucket sert aussi bien `kind` local qu'un futur EKS).
- **IAM — utilisateur `rio-cicd`** : créé avec une policy minimale (`rio-cicd-minimal`, inline) scopée à :
  push/pull sur les deux dépôts ECR ci-dessus, lecture/écriture sur le bucket S3 ci-dessus,
  `eks:DescribeCluster` sur `arn:aws:eks:us-east-1:352206183080:cluster/rio-audio-guide` (le nom du futur
  cluster, qui n'existe pas encore — une policy IAM peut référencer une ressource pas encore créée).

## Décision assumée — credentials actuellement utilisés : root, pas `rio-cicd`

**État réel actuel** : le secret K8s (`rio-backend-secrets`, clés `aws-access-key-id`/
`aws-secret-access-key`) et les secrets GitHub Actions (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`)
contiennent les credentials du **compte root**, pas ceux de `rio-cicd`.

**Pourquoi** : une confusion en créant les clés de `rio-cicd` (trois Access Key ID différents sont apparus
successivement — celui affiché à la création, celui réellement actif selon `aws iam list-access-keys`, et
un troisième dans `~/.aws/credentials` — jamais élucidé complètement) a rendu la paire secrète de
`rio-cicd` indisponible sans la régénérer. Proposé de régénérer proprement ; refusé explicitement pour
aller plus vite — décision assumée, pas un oubli. `rio-cicd` et sa policy minimale existent toujours côté
IAM, inutilisés pour l'instant.

**Coût si ce choix se révèle mauvais** : le secret GitHub Actions (exposition continue, risque le plus
élevé) a un accès total au compte, pas seulement ECR/S3/EKS-describe. Bascule vers `rio-cicd` possible à
tout moment : régénérer sa clé secrète (`aws iam create-access-key --user-name rio-cicd` après avoir
supprimé l'ancienne), puis remplacer les mêmes secrets K8s/GitHub par les nouvelles valeurs — aucun autre
changement nécessaire, la policy est déjà en place.

**Une clé Access Key ID de `rio-cicd` (`AKIAVEAI73KUPQ2HKFWM`, celle affichée à la création, maintenant
invalide) est apparue en clair dans une conversation Claude Code** — traité comme un non-événement dans la
mesure où cette clé précise n'est plus active côté IAM (remplacée par une autre avant même d'avoir servi),
et où les credentials réellement utilisés aujourd'hui sont ceux de root, pas ceux-là.

## Validé ce soir contre le cluster `kind` local (pas encore EKS)

Avec les credentials root actuels, le worker consommant `tts_jobs` a réussi un cycle complet réel :
génération ElevenLabs → upload S3 → `Script` publié — testé et confirmé avec un vrai fichier MP3 valide
téléchargé et écouté. Un bug réel a été trouvé au passage : les erreurs d'upload S3 (`InvalidAccessKeyId`,
survenu pendant les tâtonnements de credentials ci-dessus) ne sont pas classées transitoire/permanent
comme le sont déjà les erreurs ElevenLabs — a bouclé en retry pendant ~9 minutes avant intervention
manuelle (redémarrage du pod). Correctif prévu dans le spec du chantier route audio + Redis (voir
`2026-08-16-redis-cache-and-audio-route-design.md`).

## Ce qui reste — pas encore fait

- **Le cluster EKS lui-même n'existe pas.** Rien dans ce repo ne le crée (`k8s-deploy.yml` suppose qu'il
  existe déjà, ne le provisionne jamais). Reste à lancer `eksctl create cluster` (ou équivalent), avec des
  permissions bien plus larges que celles de `rio-cicd` (VPC, EC2, CloudFormation, rôles IAM) — décision
  déjà prise de garder ça une action root, ponctuelle et supervisée, distincte du secret CI/CD courant.
- **Mapping d'accès EKS** : une fois le cluster créé, `rio-cicd` (ou root) doit être explicitement ajouté
  au contrôle d'accès du cluster (EKS access entries) pour que `kubectl`/Helm y fonctionnent —
  `eks:DescribeCluster` seul ne donne que les infos de connexion, pas le droit d'y déployer.
- **CI/CD actuellement rouge sur la PR backend**, pour deux raisons indépendantes de ce déploiement :
  - `lint` : `golangci-lint` (compilé avec Go 1.24) refuse de tourner sur un module ciblant Go 1.25 —
    problème de version d'outil, pas de code.
  - `security` : `govulncheck` trouve de vraies CVE dans la stdlib Go 1.25.12 (corrigées en 1.25.13) et
    dans `golang.org/x/text`/`golang.org/x/net`.
  - PR #1 (backend) en conflit (`mergeable: CONFLICTING`) avec `master` — à résoudre avant tout merge.
- **Aucune image poussée sur ECR** — les dépôts sont vides, en attente que le CI passe.
- **Aucun test réel contre EKS** — tout ce qui a été validé ce soir l'a été contre `kind`, pas contre un
  vrai cluster cloud.

## Répartition prévue

Ce chantier (créer le cluster, réparer le CI/CD, pousser les images, déployer et tester pour de vrai sur
EKS) est prévu pour une fenêtre de contexte séparée de celle qui traite le chantier route audio + Redis —
les deux touchent le même worktree `backend`, doivent donc être séquencés plutôt que menés en simultané
sans coordination.
