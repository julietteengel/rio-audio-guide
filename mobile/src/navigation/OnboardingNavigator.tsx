import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import type { OnboardingStackParamList } from "./types";
import { WelcomeScreen } from "../screens/onboarding/Welcome";
import { ProposeScreen } from "../screens/onboarding/Propose";
import { DownloadSuccessScreen } from "../screens/onboarding/DownloadSuccess";

const Stack = createNativeStackNavigator<OnboardingStackParamList>();

export function OnboardingNavigator() {
  return (
    <Stack.Navigator screenOptions={{ headerShown: false }}>
      <Stack.Screen name="Welcome" component={WelcomeScreen} />
      <Stack.Screen name="Propose" component={ProposeScreen} />
      <Stack.Screen name="DownloadSuccess" component={DownloadSuccessScreen} />
    </Stack.Navigator>
  );
}
