import Constants from "expo-constants";

/**
 * Base URL of the backend API (see rio-audio-guide's `backend` branch,
 * `internal/adapters/http`), e.g. "http://192.168.0.45:8080".
 *
 * A physical phone running Expo Go cannot reach "localhost" — that resolves
 * to the phone itself, not the machine running the backend — so this must be
 * the backend machine's LAN IP while both devices are on the same Wi-Fi.
 *
 * Override without editing this file, in order of precedence:
 *   1. `EXPO_PUBLIC_API_BASE_URL` env var (e.g. in a local `.env` file) —
 *      the simplest override, picked up automatically by Expo's bundler.
 *   2. `expo.extra.apiBaseUrl` in app.json.
 *   3. The hardcoded fallback below, which is only ever right on the exact
 *      machine/network this was last configured for.
 */
export const API_BASE_URL: string =
  process.env.EXPO_PUBLIC_API_BASE_URL ??
  (Constants.expoConfig?.extra?.apiBaseUrl as string | undefined) ??
  "http://192.168.0.45:8080";
