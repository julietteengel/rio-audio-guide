#!/usr/bin/env bash
# Demo tool, not part of the tested application. Fires a burst of script-review
# requests at the API so tts_jobs fills up fast enough to watch KEDA scale the
# worker Deployment up (and, once the queue drains, back to zero).
#
# Usage:
#   DATABASE_URL=... API_URL=http://localhost:8081 VOICE_ID=<voice_id> \
#     ./burst_load.sh [BATCH_SIZE]
#
# While this runs, watch scaling live in two other terminals:
#   kubectl get pods -l app=rio-worker -w
#   open http://localhost:15672   (RabbitMQ management UI, if port-forwarded)
set -euo pipefail

: "${DATABASE_URL:?set DATABASE_URL (e.g. via kubectl port-forward svc/demo-postgres-postgresql 5434:5432)}"
: "${API_URL:?set API_URL (e.g. via kubectl port-forward svc/rio-api 8081:80, then API_URL=http://localhost:8081)}"
: "${VOICE_ID:?set VOICE_ID to an ElevenLabs voice_id (library voice or a cloned one)}"

BATCH_SIZE="${1:-50}"

echo "Fetching up to ${BATCH_SIZE} draft script IDs..."
mapfile -t SCRIPT_IDS < <(psql "$DATABASE_URL" -t -A -c \
  "SELECT id FROM scripts WHERE status = 'draft' LIMIT ${BATCH_SIZE};")

if [ "${#SCRIPT_IDS[@]}" -eq 0 ]; then
  echo "No draft scripts found — run cmd/import first, or re-run cmd/import after a prior burst" \
       "(reviewed scripts won't show up again)." >&2
  exit 1
fi

echo "Firing ${#SCRIPT_IDS[@]} review requests at ${API_URL} (voice_id=${VOICE_ID})..."

for id in "${SCRIPT_IDS[@]}"; do
  curl -s -o /dev/null -w "%{http_code} ${id}\n" -X POST "${API_URL}/scripts/${id}/review" \
    -H "Content-Type: application/json" \
    -d "{\"reviewer\":\"demo\",\"voice_id\":\"${VOICE_ID}\"}" &
done
wait

echo "Burst sent. Watch: kubectl get pods -l app=rio-worker -w"
