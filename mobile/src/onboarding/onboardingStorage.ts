import AsyncStorage from "@react-native-async-storage/async-storage";

const KEY = "memoria-carioca:onboarding-complete";

export async function isOnboardingComplete(): Promise<boolean> {
  return (await AsyncStorage.getItem(KEY)) === "true";
}

export async function markOnboardingComplete(): Promise<void> {
  await AsyncStorage.setItem(KEY, "true");
}
