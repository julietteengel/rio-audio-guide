import { createNavigationContainerRef } from "@react-navigation/native";
import type { RootStackParamList } from "../navigation/RootNavigator";

// Held outside the component tree so a notification-tap listener (which
// runs from expo-notifications' own event system, not from any React
// component) can navigate without needing a ref threaded through props.
export const navigationRef = createNavigationContainerRef<RootStackParamList>();

export function navigateToPlace(placeId: string): void {
  if (!navigationRef.isReady()) return;
  navigationRef.navigate("App", { screen: "PlaceDetail", params: { placeId } });
}

// A cold start reads its launching notification before the container is ready
// (RootNavigator renders null until its async onboarding check resolves), so
// the target is parked here and flushed from NavigationContainer's onReady.
let pendingPlaceId: string | null = null;

export function setPendingPlaceId(placeId: string): void {
  pendingPlaceId = placeId;
}

export function flushPendingNavigation(): void {
  if (pendingPlaceId && navigationRef.isReady()) {
    const placeId = pendingPlaceId;
    pendingPlaceId = null;
    navigateToPlace(placeId);
  }
}
