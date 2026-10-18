import React, { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import * as Location from "expo-location";
import type { Itinerary, ItineraryStop } from "../data/ItinerariesRepository";
import { placesRepository } from "../data/PlacesRepository";
import { haversineMeters, type LatLon } from "../utils/geo";

// Close enough on foot in a dense urban area to count as "arrived" -- tighter
// than the background GEOFENCE_RADIUS_METERS (150m) since this only runs
// while the app is foregrounded and actively tracking one active itinerary,
// not scanning dozens of candidates at once.
const ARRIVAL_RADIUS_METERS = 60;

// How often to re-check the user's position against the current stop while
// an itinerary is active. A plain interval, not watchPosition -- this only
// needs to notice "have I arrived yet", not track a smooth live trail.
const CHECK_INTERVAL_MS = 15_000;

export type ActiveItineraryState = {
  itinerary: Itinerary | null;
  /** Index into itinerary.stops of the stop the user is currently walking to. */
  currentStopIndex: number;
  /** 1-based count among "place" stops only -- matches the numbered badges shown in FeaturedItineraryDetail. */
  currentPlaceStepNumber: number;
  totalPlaceSteps: number;
  currentStop: ItineraryStop | null;
  distanceToCurrentStopMeters: number | null;
  justArrived: boolean;
  start: (itinerary: Itinerary) => void;
  end: () => void;
  // Called when playback of a place's narration finishes (expo-audio's
  // `didJustFinish`). Advances the itinerary the same way arriving within
  // ARRIVAL_RADIUS_METERS does, but on a better signal for "done with this
  // stop": GPS proximity alone can't tell a quick walk-by from someone who
  // actually stood there and listened. A no-op if placeId isn't the current
  // stop (e.g. the person is browsing a different place mid-walk).
  markStopListened: (placeId: string) => void;
};

const ActiveItineraryContext = createContext<ActiveItineraryState | undefined>(undefined);

function placeStepNumber(stops: ItineraryStop[], uptoIndex: number): number {
  let count = 0;
  for (let i = 0; i <= uptoIndex && i < stops.length; i++) {
    if (stops[i].kind === "place") count += 1;
  }
  return count;
}

function totalPlaceSteps(stops: ItineraryStop[]): number {
  return stops.filter((s) => s.kind === "place").length;
}

export function ActiveItineraryProvider({ children }: { children: React.ReactNode }) {
  const [itinerary, setItinerary] = useState<Itinerary | null>(null);
  const [currentStopIndex, setCurrentStopIndex] = useState(0);
  const [distanceToCurrentStopMeters, setDistanceToCurrentStopMeters] = useState<number | null>(null);
  const [justArrived, setJustArrived] = useState(false);
  // Place coordinates are fetched once per itinerary (not stored on the
  // itinerary itself, which only carries placeId/label/timing) and cached
  // here for the lifetime of the active run.
  const placeCoordsRef = useRef<Map<string, LatLon>>(new Map());

  const start = useCallback((newItinerary: Itinerary) => {
    setItinerary(newItinerary);
    setCurrentStopIndex(0);
    setDistanceToCurrentStopMeters(null);
    setJustArrived(false);
    placeCoordsRef.current = new Map();
  }, []);

  const end = useCallback(() => {
    setItinerary(null);
    setCurrentStopIndex(0);
    setDistanceToCurrentStopMeters(null);
    setJustArrived(false);
    placeCoordsRef.current = new Map();
  }, []);

  const currentStop = itinerary ? (itinerary.stops[currentStopIndex] ?? null) : null;

  const markStopListened = useCallback(
    (placeId: string) => {
      if (!itinerary) return;
      const stop = itinerary.stops[currentStopIndex];
      if (!stop || stop.kind !== "place" || stop.placeId !== placeId) return;

      setJustArrived(true);
      const isLast = currentStopIndex >= itinerary.stops.length - 1;
      if (!isLast) {
        setCurrentStopIndex((i) => i + 1);
        setDistanceToCurrentStopMeters(null);
      }
    },
    [itinerary, currentStopIndex],
  );

  useEffect(() => {
    if (!itinerary) return;
    let cancelled = false;

    async function checkOnce() {
      if (!itinerary) return;
      const stop = itinerary.stops[currentStopIndex];
      if (!stop) return;

      // A suggestion stop (e.g. a lunch break) has no place/coordinates to
      // arrive at -- advancing past it automatically keeps the "current
      // stop" always pointing at something a user can actually walk to.
      if (stop.kind !== "place") {
        if (!cancelled) setCurrentStopIndex((i) => Math.min(i + 1, itinerary.stops.length - 1));
        return;
      }

      const { status } = await Location.requestForegroundPermissionsAsync();
      if (status !== "granted" || cancelled) return;

      let coords = placeCoordsRef.current.get(stop.placeId);
      if (!coords) {
        const place = await placesRepository.getById(stop.placeId);
        if (!place || cancelled) return;
        coords = { latitude: place.lat, longitude: place.lon };
        placeCoordsRef.current.set(stop.placeId, coords);
      }

      const position = await Location.getCurrentPositionAsync({});
      if (cancelled) return;
      const here: LatLon = {
        latitude: position.coords.latitude,
        longitude: position.coords.longitude,
      };
      const distance = haversineMeters(here, coords);
      setDistanceToCurrentStopMeters(distance);

      if (distance <= ARRIVAL_RADIUS_METERS) {
        setJustArrived(true);
        const isLast = currentStopIndex >= itinerary.stops.length - 1;
        if (!isLast) {
          setCurrentStopIndex((i) => i + 1);
          setDistanceToCurrentStopMeters(null);
        }
      } else {
        setJustArrived(false);
      }
    }

    checkOnce();
    const interval = setInterval(checkOnce, CHECK_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [itinerary, currentStopIndex]);

  const value: ActiveItineraryState = {
    itinerary,
    currentStopIndex,
    currentPlaceStepNumber: itinerary ? placeStepNumber(itinerary.stops, currentStopIndex) : 0,
    totalPlaceSteps: itinerary ? totalPlaceSteps(itinerary.stops) : 0,
    currentStop,
    distanceToCurrentStopMeters,
    justArrived,
    start,
    end,
    markStopListened,
  };

  return <ActiveItineraryContext.Provider value={value}>{children}</ActiveItineraryContext.Provider>;
}

export function useActiveItinerary(): ActiveItineraryState {
  const ctx = useContext(ActiveItineraryContext);
  if (!ctx) throw new Error("useActiveItinerary must be used within an ActiveItineraryProvider");
  return ctx;
}
