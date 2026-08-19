import React, { useCallback, useEffect, useState } from "react";
import { View } from "react-native";
import { StatusBar } from "expo-status-bar";
import { NavigationContainer } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import * as SplashScreen from "expo-splash-screen";
import * as Font from "expo-font";
import { LocaleProvider } from "./src/i18n/LocaleContext";
import { AuthProvider } from "./src/auth/AuthContext";
import { RootNavigator } from "./src/navigation/RootNavigator";
import { colors } from "./src/theme/tokens";

SplashScreen.preventAutoHideAsync().catch(() => {});

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
            <NavigationContainer>
              <RootNavigator />
            </NavigationContainer>
          </AuthProvider>
        </LocaleProvider>
      </SafeAreaProvider>
      <StatusBar style="dark" />
    </View>
  );
}
