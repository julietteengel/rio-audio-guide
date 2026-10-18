# 2026-10-18 — Live testing session: status and next steps

Point-in-time status from a live field-testing session (Juliette walking Rio
with the standalone EAS build, reporting bugs in real time). Not a design
doc — a handoff snapshot. See git log on `pipeline/narration-grounding-for-verification`
and the `featured-itineraries` worktree for the actual commits.

## Done this session

- **Production deployment**: the app now points to a real, publicly reachable
  HTTPS backend (`https://52-4-110-155.nip.io`, Caddy + Let's Encrypt on the
  pre-existing `rio-audio-guide-test` EC2 instance), backed by the real RDS
  instance.
- **AI itinerary generation fixed**: `ANTHROPIC_API_KEY` in the server's
  `docker-compose.yml` was stale/wrong; replaced with the correct key from
  `backend/.env` (the "rio-audioguide" key in the Anthropic console) and the
  `api` container force-recreated. Confirmed working from the app.
- **Transcription fix (Praça XV de Novembro, PT)**: Whisper had mis-heard
  "Praça XV" as "Traça 15" throughout the narration, plus ~10 related errors
  (Piaçaba→Piaçada, Paço→Passo, Pharoux→faroques, etc.) — all confirmed
  against the real script text and fixed directly in the S3
  `timestamps.json` (no redeploy needed, already live).
- **Mobile UI fixes** (code done, **not yet in a shipped build** — see
  below): synced-narration text no longer truncates with "…" on the active
  line (was cutting off words mid-sentence); place title capped at 2 lines
  so a long name can't shrink the lyrics panel and cause clipping/overlap
  near the toggle buttons.
- **Bulk audio generation**: resumed after the user added OpenAI credit;
  generated audio for ~75 more places this run before credit ran out again.
  Local dev DB now has 203 ready audio files (up from 128 at the start of
  tonight's monitoring).
- **Production DB sync**: local DB (with tonight's new audio/fixes) dumped
  and restored onto the production RDS instance — see commit/chat log for
  the exact dump timestamp used.

## Still open

- **205 places still have no narration audio at all** (out of ~226 that
  needed it). Blocked purely on OpenAI credit (TTS + Whisper both draw from
  the same account balance). Resume by re-running the bulk generation
  script once credit is topped up.
- **Whisper timestamps for the newest audio batch are not generated yet**
  (same OpenAI-credit blocker). Needed before the new places' captions sync
  in the app.
- **Mobile UI fixes above need a new EAS build** (`eas-cli build --profile
  preview --platform ios --non-interactive`) before Juliette can actually
  see them on her phone — the installed build is a standalone preview
  build with no OTA/live-reload.
- **Transcript audit is not reliable yet**: an automated script compared
  every ready (place, language) pair's script text against its Whisper
  transcript, flagging capitalized words missing from the transcript. It
  produced 481 "findings" across 183 pairs, but the signal-to-noise ratio
  is too low to act on directly (common capitalized words — sentence-start
  French nouns like "Passé", "Empire" — dominate the list). The one
  confirmed real bug found tonight (Praça XV) was found by manually reading
  the script against the transcript, not by this script. A follow-up pass
  using an LLM to actually judge each flagged instance (rather than literal
  string matching) would be needed to make this audit worth running at
  scale — not done tonight.
- **Housekeeping from earlier in the session, still unresolved**:
  - A temporary `0.0.0.0/0` port-22 ingress rule was added to the
    Terraform-managed EC2 security group (`backend/deploy/terraform/ec2.tf`)
    for EC2 Instance Connect debugging. Should be reviewed/closed.
  - The Terraform-created `rio-audio-guide-app` EC2 instance (distinct from
    the manually-created one actually in use) is stopped, not destroyed.
    Decide whether to destroy it or keep it for a future retry.

## Next

- **README screenshots**: done for tonight. 8 of 12 planned shots are in
  (`docs/images/feature-*.png`, wired into the README's "App features"
  section). Dropped for now, not worth doing tonight: "My Itineraries" list,
  offline-download screen, account/profile screen, and the AI-generated
  itinerary result (blocked on the Anthropic key below anyway). Re-add
  these rows to the README whenever those screenshots get taken.
- **Still broken: AI itinerary generation in production**. The Anthropic
  key in `~/rio-audio-guide/docker-compose.yml` on the server tested
  invalid again late tonight (`curl --http1.1 .../v1/messages` →
  `authentication_error: invalid x-api-key`), despite several attempts to
  fix it via `nano` (suspected corruption from copy/pasting a long secret
  over SSH on a mobile terminal — the container's env value kept coming out
  126 characters instead of the real key's 108). Not resolved before the
  session ended. Next attempt should avoid `nano` entirely — use a single
  `read -r` prompt + `sed -i` instead (given mid-message, not yet
  confirmed working):
  ```bash
  read -r -p "Paste the Anthropic key here: " NEWKEY
  sed -i "s|ANTHROPIC_API_KEY:.*|ANTHROPIC_API_KEY: \"$NEWKEY\"|" ~/rio-audio-guide/docker-compose.yml
  unset NEWKEY
  cd ~/rio-audio-guide && sudo docker compose up -d --force-recreate api
  sudo docker exec rio-audio-guide-api-1 printenv ANTHROPIC_API_KEY | tr -d '\n' | wc -c
  ```
  Should print `108`. Then retry generating an itinerary in the app.
