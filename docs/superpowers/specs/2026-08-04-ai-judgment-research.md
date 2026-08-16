# AI judgment in software engineering — research summary

Recherche multi-sources, vérifiée de manière adversariale (24 sources fetchées, 108 affirmations
extraites, 25 vérifiées par vote 3-0 indépendant), sur la question : où est la limite entre laisser
l'IA tout faire et rester un "bon développeur".

Sert à cadrer la pratique de travail sur ce repo.

## Constat central

L'IA est sûre (et attendue) pour le boilerplate, l'exploration, le premier jet. Mais un développeur
qui veut progresser et être perçu comme ayant "du jugement" doit garder la propriété active de :
l'architecture, la revue/vérification du code, la compréhension du debugging. Ce sont exactement les
zones où la recherche montre qu'un usage non supervisé dégrade à la fois la compétence individuelle
et la stabilité du système livré.

## Affirmations vérifiées (confiance haute, sources primaires)

1. **DORA 2025 (Google, ~5000 répondants) — l'IA comme "amplificateur"** : "AI's primary role is as
   an amplifier, magnifying an organization's existing strengths and weaknesses." 90% utilisent l'IA,
   80%+ perçoivent un gain de productivité, mais 30% ne font toujours pas confiance au code généré.
   Une pratique de revue faible n'est pas compensée par l'IA — elle est amplifiée en pire.
   Sources : dora.dev, cloud.google.com/blog, itrevolution.com, jellyfish.co (tous 3-0).

2. **DORA 2025 — vitesse et qualité découplées** : relation positive avec le débit de livraison,
   négative avec la stabilité de livraison. Écart entre gain de productivité perçu (80%+) et gain de
   qualité perçu (59% seulement).

3. **Anthropic, RCT, 52 ingénieurs, janvier 2026** : les développeurs ayant utilisé l'IA ont obtenu
   17% de moins à un quiz de compréhension/debugging post-tâche que ceux ayant codé à la main.
   Nuance retenue par la vérification : échantillon majoritairement junior, assistant conversationnel
   (pas agentique type Claude Code), une seule lib inconnue, mesure immédiate pas longitudinale — à
   traiter comme signal de risque réel, pas comme prédiction précise pour un usage agentique soutenu.
   Source : anthropic.com/research/AI-assistance-coding-skills.

4. **Documentation officielle GitHub Copilot** : traiter tout code IA comme un brouillon non vérifié
   — chercher les hallucinations d'API, les tests supprimés plutôt que corrigés, et vérifier
   indépendamment que le code s'intègre à l'architecture existante, pas seulement qu'il tourne.
   Source : docs.github.com/en/copilot/tutorials/review-ai-generated-code.

5. **Mark Russinovich (CTO Azure) et Scott Hanselman (VP Microsoft), Communications of the ACM,
   avril 2026** : les organisations qui optimisent seulement l'efficacité court terme (embaucher des
   gens qui savent déjà "diriger" l'IA plutôt que former des ingénieurs hands-on) risquent de vider
   le pipeline de futurs talents seniors.

6. **Addy Osmani — "vibe coding" vs "AI-assisted engineering"** (confiance medium, avis de praticien
   largement repris, pas une étude) : *vibe coding* = accepter les suggestions IA sans revue profonde,
   vitesse avant justesse. *AI-assisted engineering* = l'humain reste responsable de l'architecture,
   relit et comprend chaque ligne, garantit sécurité/scalabilité/maintenabilité.

## Affirmations vérifiées puis réfutées — à ne pas citer

La vérification adversariale a tué 12 affirmations séduisantes mais non fondées, notamment des
chiffres très partagés mais non confirmés (source secondaire Faros.ai) : "+54% de bugs", "+242,7%
d'incidents par PR", "+441% de temps de revue", "31% de PRs mergées sans revue". La tendance
directionnelle (l'instabilité augmente avec l'adoption IA) reste vraie ; ces chiffres précis, non.
Ne pas les citer comme faits établis.

## Angle mort assumé

Aucune source n'a passé la vérification sur ce qui est concrètement évalué en pratique pour "sound
judgment in AI tools" en entreprise (quelques entreprises ont publié sur le sujet, mais leurs
affirmations n'ont pas survécu au vote à 3). C'est une pratique émergente, pas encore codifiée — à
traiter par principes généraux, pas par une réponse toute faite.

## Application à ce projet

- Confirme le principe déjà en place : revue humaine réelle avant tout merge (voir la correction sur
  `sourcing-pipeline`, jamais mergée à ce jour faute de cette revue).
- Confirme le choix d'écrire le guide runtime et le backend Go/hexagonal/DDD à la main plutôt que de
  les laisser générer — c'est exactement la zone non-délégable identifiée par la recherche.
- Utile pour soi : garder 2-3 exemples précis où une sortie IA a été rejetée, corrigée ou fortement
  éditée. Le rejet de Kafka/Mongo dans `2026-08-04-backend-stack-decision.md` en est un, daté et
  documenté — plus convaincant qu'une déclaration d'intention générale.
