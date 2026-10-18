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
import { FeaturedItineraryDetailScreen } from "../screens/FeaturedItineraryDetail";

const Stack = createNativeStackNavigator<AppStackParamList>();

export function AppNavigator() {
  return (
    <Stack.Navigator screenOptions={{ headerShown: false }}>
      <Stack.Screen name="Map" component={MapScreen} />
      {/* gestureEnabled: false -- the iOS edge-swipe-back gesture is a native
          UIKit gesture recognizer, entirely separate from React Native's JS
          touch responder system. Dragging the audio progress bar's own
          PanResponder (which does correctly claim the touch on the JS side)
          can't stop it from also firing, which read as the seek bar
          randomly navigating back to the map mid-drag. This screen already
          has its own visible back button, so losing the swipe gesture here
          costs nothing. */}
      <Stack.Screen name="PlaceDetail" component={PlaceDetailScreen} options={{ gestureEnabled: false }} />
      <Stack.Screen name="Assistant" component={AssistantScreen} />
      <Stack.Screen name="Settings" component={SettingsScreen} />
      <Stack.Screen name="ItinerariesList" component={ItinerariesListScreen} />
      <Stack.Screen name="ItineraryChat" component={ItineraryChatScreen} />
      <Stack.Screen name="ItineraryDetail" component={ItineraryDetailScreen} />
      <Stack.Screen name="FeaturedItineraryDetail" component={FeaturedItineraryDetailScreen} />
      <Stack.Screen name="Auth" component={AuthScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="VerifyEmail" component={VerifyEmailScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="ForgotPassword" component={ForgotPasswordScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="ResetPassword" component={ResetPasswordScreen} options={{ presentation: "modal" }} />
      <Stack.Screen name="EditProfile" component={EditProfileScreen} />
    </Stack.Navigator>
  );
}
