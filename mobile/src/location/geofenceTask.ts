import * as Location from "expo-location";
import * as TaskManager from "expo-task-manager";
import * as Notifications from "expo-notifications";
import { getAllCachedPlaces, getLastNotifiedAt, setLastNotifiedAt } from "../data/offlineStore";
import { selectNearestRegions, shouldReregister, isCooldownActive } from "./geofenceLogic";

export const GEOFENCE_TASK_NAME = "memoria-carioca-geofence";
export const LOCATION_UPDATE_TASK_NAME = "memoria-carioca-location-update";

// Module-scope, not component state -- deliberately survives across
// app-background/foreground cycles within one process lifetime. Reset to
// null (forcing an immediate re-registration) on every fresh
// startProximityMonitoring() call, which itself only runs once per
// permission grant or app launch, not per re-render.
let lastRegisteredCenter: { latitude: number; longitude: number } | null = null;

// Per expo-task-manager's own docs: must be called unconditionally at
// module scope, not inside any React lifecycle method, so both tasks are
// always defined the moment this module is imported (from App.tsx), on
// every app launch -- including a background launch with no UI mounted.
TaskManager.defineTask(GEOFENCE_TASK_NAME, async ({ data, error }) => {
  if (error) {
    console.warn("geofence task error", error);
    return;
  }
  const { eventType, region } = data as {
    eventType: Location.LocationGeofencingEventType;
    region: Location.LocationRegion;
  };
  if (eventType !== Location.LocationGeofencingEventType.Enter) return;

  const placeId = region.identifier;
  if (!placeId) return;

  try {
    const lastNotifiedAt = await getLastNotifiedAt(placeId);
    if (isCooldownActive(lastNotifiedAt, Date.now())) return;

    const places = await getAllCachedPlaces();
    const place = places.find((p) => p.id === placeId);
    if (!place) return;

    await Notifications.scheduleNotificationAsync({
      content: {
        title: place.name,
        body: "Écoute son histoire.",
        data: { placeId },
      },
      trigger: null,
    });
    await setLastNotifiedAt(placeId, Date.now());
  } catch (err) {
    console.warn("geofence: failed to schedule notification", err);
  }
});

TaskManager.defineTask(LOCATION_UPDATE_TASK_NAME, async ({ data, error }) => {
  if (error) {
    console.warn("location update task error", error);
    return;
  }
  const { locations } = data as { locations: Location.LocationObject[] };
  const latest = locations[locations.length - 1];
  if (!latest) return;

  const currentPosition = {
    latitude: latest.coords.latitude,
    longitude: latest.coords.longitude,
  };
  if (!shouldReregister(lastRegisteredCenter, currentPosition)) return;

  try {
    const places = await getAllCachedPlaces();
    const regions = selectNearestRegions(places, currentPosition);
    if (regions.length === 0) return;

    // stopGeofencingAsync before re-starting: startGeofencingAsync replaces
    // the monitored set, but going through stop first avoids relying on
    // that being safe to call twice with an already-active task under the
    // hood (undocumented either way -- this is the conservative order).
    await Location.stopGeofencingAsync(GEOFENCE_TASK_NAME).catch(() => {});
    await Location.startGeofencingAsync(GEOFENCE_TASK_NAME, regions);
    lastRegisteredCenter = currentPosition;
  } catch (err) {
    console.warn("geofence: failed to re-register regions", err);
  }
});

/**
 * Starts background location updates and registers the initial geofence
 * region set. Call only after `requestProximityPermissions()` has resolved
 * `true`. No-ops if there's nothing downloaded to monitor.
 */
export async function startProximityMonitoring(): Promise<void> {
  const places = await getAllCachedPlaces();
  if (places.length === 0) return;

  await Location.startLocationUpdatesAsync(LOCATION_UPDATE_TASK_NAME, {
    accuracy: Location.LocationAccuracy.Balanced,
    distanceInterval: 300,
    showsBackgroundLocationIndicator: false,
  });

  // Register a region set immediately, from the last known position (or,
  // failing that, the first cached place's own coordinates) -- otherwise
  // geofencing sits idle until the first location update fires, which could
  // be a while depending on distanceInterval and how much the user moves.
  const lastKnown = await Location.getLastKnownPositionAsync();
  const initialCenter = lastKnown
    ? { latitude: lastKnown.coords.latitude, longitude: lastKnown.coords.longitude }
    : { latitude: places[0].lat, longitude: places[0].lon };
  const regions = selectNearestRegions(places, initialCenter);
  await Location.startGeofencingAsync(GEOFENCE_TASK_NAME, regions);
  lastRegisteredCenter = initialCenter;
}

export async function stopProximityMonitoring(): Promise<void> {
  lastRegisteredCenter = null;
  if (await Location.hasStartedGeofencingAsync(GEOFENCE_TASK_NAME)) {
    await Location.stopGeofencingAsync(GEOFENCE_TASK_NAME);
  }
  if (await Location.hasStartedLocationUpdatesAsync(LOCATION_UPDATE_TASK_NAME)) {
    await Location.stopLocationUpdatesAsync(LOCATION_UPDATE_TASK_NAME);
  }
}

/**
 * Requests foreground location, then background location, then
 * notifications -- in that order, since each is a prerequisite the OS
 * expects before the next (background permission cannot be granted without
 * foreground already granted). Returns whether every one of the three was
 * granted.
 */
export async function requestProximityPermissions(): Promise<boolean> {
  const foreground = await Location.requestForegroundPermissionsAsync();
  if (foreground.status !== "granted") return false;
  const background = await Location.requestBackgroundPermissionsAsync();
  if (background.status !== "granted") return false;
  const notifications = await Notifications.requestPermissionsAsync();
  return notifications.granted;
}

export async function isProximityMonitoringActive(): Promise<boolean> {
  return Location.hasStartedGeofencingAsync(GEOFENCE_TASK_NAME);
}
