import React, { useCallback, useEffect, useState } from "react";
import { View } from "react-native";
import { StatusBar } from "expo-status-bar";
import { NavigationContainer } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import * as SplashScreen from "expo-splash-screen";
import * as Font from "expo-font";
import * as Notifications from "expo-notifications";
import { LocaleProvider } from "./src/i18n/LocaleContext";
import { AuthProvider } from "./src/auth/AuthContext";
import { RootNavigator } from "./src/navigation/RootNavigator";
import {
  navigationRef,
  navigateToPlace,
  setPendingPlaceId,
  flushPendingNavigation,
} from "./src/utils/navigationRef";
import { colors } from "./src/theme/tokens";
// Side-effect import: geofenceTask's TaskManager.defineTask calls must run at
// module scope on every launch, including a headless background one. Importing
// it here rather than relying on Settings.tsx happening to be in the bundle's
// eagerly-evaluated import graph.
import "./src/location/geofenceTask";

SplashScreen.preventAutoHideAsync().catch(() => {});

// Must be set once, at module scope, before any notification could arrive
// (including one that woke the app from the background) -- controls how a
// notification presents while the app is in the foreground.
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

export default function App() {
  const [fontsLoaded, setFontsLoaded] = useState(false);

  useEffect(() => {
    Font.loadAsync({
      "PlayfairDisplay-Bold": require("./assets/fonts/PlayfairDisplay-Bold.ttf"),
      "PlayfairDisplay-Black": require("./assets/fonts/PlayfairDisplay-Black.ttf"),
      "Inter-Regular": require("./assets/fonts/Inter-Regular.ttf"),
      "Inter-Medium": require("./assets/fonts/Inter-Medium.ttf"),
      "Inter-SemiBold": require("./assets/fonts/Inter-SemiBold.ttf"),
      "Inter-Bold": require("./assets/fonts/Inter-Bold.ttf"),
    })
      .then(() => setFontsLoaded(true))
      .catch(() => setFontsLoaded(true));
  }, []);

  useEffect(() => {
    const subscription = Notifications.addNotificationResponseReceivedListener((response) => {
      const placeId = response.notification.request.content.data?.placeId;
      if (typeof placeId === "string") navigateToPlace(placeId);
    });
    return () => subscription.remove();
  }, []);

  // The listener above is only registered once JS is running, so it never sees
  // the tap that launched a killed app -- that response comes from here.
  useEffect(() => {
    Notifications.getLastNotificationResponseAsync().then((response) => {
      const placeId = response?.notification.request.content.data?.placeId;
      if (typeof placeId === "string") {
        setPendingPlaceId(placeId);
        flushPendingNavigation();
      }
    });
  }, []);

  const onLayoutRootView = useCallback(async () => {
    if (fontsLoaded) {
      await SplashScreen.hideAsync();
    }
  }, [fontsLoaded]);

  if (!fontsLoaded) return null;

  return (
    <View style={{ flex: 1, backgroundColor: colors.cream }} onLayout={onLayoutRootView}>
      <SafeAreaProvider>
        <LocaleProvider>
          <AuthProvider>
            <NavigationContainer
              ref={navigationRef}
              documentTitle={{ formatter: () => "Memória Carioca" }}
              onReady={flushPendingNavigation}
            >
              <RootNavigator />
            </NavigationContainer>
          </AuthProvider>
        </LocaleProvider>
      </SafeAreaProvider>
      <StatusBar style="dark" />
    </View>
  );
}
