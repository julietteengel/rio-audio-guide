import React, { useEffect, useState } from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { OnboardingNavigator } from "./OnboardingNavigator";
import { AppNavigator } from "./AppNavigator";
import { isOnboardingComplete } from "../onboarding/onboardingStorage";

type RootStackParamList = {
  Onboarding: undefined;
  App: undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();

export function RootNavigator() {
  const [ready, setReady] = useState(false);
  const [initialRoute, setInitialRoute] = useState<keyof RootStackParamList>("Onboarding");

  useEffect(() => {
    isOnboardingComplete().then((done) => {
      setInitialRoute(done ? "App" : "Onboarding");
      setReady(true);
    });
  }, []);

  if (!ready) return null;

  return (
    <Stack.Navigator screenOptions={{ headerShown: false }} initialRouteName={initialRoute}>
      <Stack.Screen name="Onboarding" component={OnboardingNavigator} />
      <Stack.Screen name="App" component={AppNavigator} />
    </Stack.Navigator>
  );
}
