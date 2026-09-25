import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import type { AppStackParamList } from "./types";
import { MapScreen } from "../screens/Map";
import { PlaceDetailScreen } from "../screens/PlaceDetail";
import { AssistantScreen } from "../screens/Assistant";
import { SettingsScreen } from "../screens/Settings";
import { AuthScreen } from "../screens/Auth";
import { VerifyEmailScreen } from "../screens/VerifyEmail";
import { ForgotPasswordScreen } from "../screens/ForgotPassword";
import { ResetPasswordScreen } from "../screens/ResetPassword";
import { EditProfileScreen } from "../screens/EditProfile";
import { ItinerariesListScreen } from "../screens/ItinerariesList";
import { ItineraryChatScreen } from "../screens/ItineraryChat";
import { ItineraryDetailScreen } from "../screens/ItineraryDetail";

const Stack = createNativeStackNavigator<AppStackParamList>();

export function AppNavigator() {
  return (
    <Stack.Navigator screenOptions={{ headerShown: false }}>
      <Stack.Screen name="Map" component={MapScreen} />
      <Stack.Screen name="PlaceDetail" component={PlaceDetailScreen} />
      <Stack.Screen name="Assistant" component={AssistantScreen} />
      <Stack.Screen name="Settings" component={SettingsScreen} />
      <Stack.Screen name="ItinerariesList" component={ItinerariesListScreen} />
      <Stack.Screen name="ItineraryChat" component={ItineraryChatScreen} />
      <Stack.Screen name="ItineraryDetail" component={ItineraryDetailScreen} />
      <Stack.Screen name="Auth" component={AuthScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="VerifyEmail" component={VerifyEmailScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="ForgotPassword" component={ForgotPasswordScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="ResetPassword" component={ResetPasswordScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="EditProfile" component={EditProfileScreen} />
    </Stack.Navigator>
  );
}
