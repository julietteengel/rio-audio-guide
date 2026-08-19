import Constants from "expo-constants";

/**
 * Base URL of the backend API (see rio-audio-guide's `backend` branch,
 * `internal/adapters/http`).
 *
 * No real value is hardcoded here or in app.json — a LAN IP is
 * machine/network-specific (right today, stale tomorrow) and doesn't belong
 * committed to git either way. Set it locally instead:
 *   1. `EXPO_PUBLIC_API_BASE_URL` in a local, gitignored `.env` file (or
 *      exported in your shell) — picked up automatically by Expo's bundler.
 *      Web / iOS Simulator on the same machine as the backend: "http://
 *      localhost:8080". Physical phone over Wi-Fi: the backend machine's
 *      LAN IP instead — "localhost" on a phone resolves to the phone
 *      itself, not the machine running the backend.
 *   2. `expo.extra.apiBaseUrl` in app.json, same caveat, checked second.
 * Falls back to localhost, right for local dev on one machine (web /
 * Simulator), never for a physical device.
 */
export const API_BASE_URL: string =
  process.env.EXPO_PUBLIC_API_BASE_URL ??
  (Constants.expoConfig?.extra?.apiBaseUrl as string | undefined) ??
  "http://localhost:8080";
