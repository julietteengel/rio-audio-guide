import { Alert, Platform } from "react-native";

// react-native-web's Alert.alert is a documented no-op (see its own source:
// `static alert() {}`) -- calling it on web silently does nothing at all,
// not even a console warning. Every simple info message in this app must
// go through this instead, on every platform.
export function alertInfo(title: string, message?: string): void {
  if (Platform.OS === "web") {
    window.alert(message ? `${title}\n\n${message}` : title);
    return;
  }
  Alert.alert(title, message);
}
