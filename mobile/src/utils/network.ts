import * as Network from "expo-network";

/**
 * expo-network's `isConnected` means "an active network interface exists"
 * (e.g. connected to Wi-Fi with no real internet behind it) -- not what this
 * app actually needs to decide "should I stream or use the offline cache".
 * Prefer `isInternetReachable` when the platform reports it, falling back to
 * `isConnected` on platforms/situations where it's undefined (expo-network's
 * own docs note this is platform-specific).
 */
export async function isOnline(): Promise<boolean> {
  const state = await Network.getNetworkStateAsync();
  return state.isInternetReachable ?? state.isConnected ?? false;
}
