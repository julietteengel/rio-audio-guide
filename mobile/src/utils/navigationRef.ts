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
