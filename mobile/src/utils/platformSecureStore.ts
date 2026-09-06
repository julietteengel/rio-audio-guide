import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";

// expo-secure-store has no web implementation at all (its ExpoSecureStore.web.ts
// module is a literal empty object) -- setItemAsync/getItemAsync/deleteItemAsync
// throw on web the moment they're called, always AFTER a successful backend
// request, so the throw gets mislabeled as a network error by any caller that
// distinguishes AuthApiError from "something else went wrong". localStorage is
// the pragmatic web equivalent for this app (a testing fallback, not the primary
// target per mobile/AGENTS.md), same pattern as platformAlert.ts.
export async function secureSetItem(key: string, value: string): Promise<void> {
  if (Platform.OS === "web") {
    window.localStorage.setItem(key, value);
    return;
  }
  await SecureStore.setItemAsync(key, value);
}

export async function secureGetItem(key: string): Promise<string | null> {
  if (Platform.OS === "web") {
    return window.localStorage.getItem(key);
  }
  return SecureStore.getItemAsync(key);
}

export async function secureDeleteItem(key: string): Promise<void> {
  if (Platform.OS === "web") {
    window.localStorage.removeItem(key);
    return;
  }
  await SecureStore.deleteItemAsync(key);
}
