import React from "react";
import { View, Text, StyleSheet } from "react-native";
import MapView, { Marker } from "react-native-maps";
import type { Place } from "../data/types";
import type { LatLon } from "../utils/geo";
import { colors, fonts } from "../theme/tokens";

export type PlaceMapRegion = {
  latitude: number;
  longitude: number;
  latitudeDelta: number;
  longitudeDelta: number;
};

export type PlaceMapProps = {
  places: Place[];
  // Place id -> 1-based position in the active itinerary. When present, a
  // place's pin shows this number instead of the plain dot -- undefined
  // (no active itinerary) keeps the plain dot for everyone.
  stepNumbers?: Map<string, number>;
  region: PlaceMapRegion;
  userLocation: LatLon | null;
  youAreHereLabel: string;
  onSelectPlace: (placeId: string) => void;
};

// Native implementation, react-native-maps. See PlaceMap.web.tsx for the
// web counterpart (Leaflet or Google Maps, depending on configuration) --
// same props, Metro picks whichever file matches the target platform, so
// Map.tsx just renders <PlaceMap ... /> without any Platform branching of
// its own.
export function PlaceMap({ places, stepNumbers, region, userLocation, youAreHereLabel, onSelectPlace }: PlaceMapProps) {
  return (
    <MapView style={StyleSheet.absoluteFill} initialRegion={region}>
      {places.map((p) => {
        const step = stepNumbers?.get(p.id);
        return (
          <Marker
            key={p.id}
            coordinate={{ latitude: p.lat, longitude: p.lon }}
            onPress={() => onSelectPlace(p.id)}
            title={p.name}
          >
            {step ? (
              <View style={styles.stepPin}>
                <Text style={styles.stepPinText}>{step}</Text>
              </View>
            ) : (
              <View style={styles.pin} />
            )}
          </Marker>
        );
      })}
      {userLocation ? (
        <Marker coordinate={userLocation} anchor={{ x: 0.5, y: 0.5 }} title={youAreHereLabel}>
          <View style={styles.me}>
            <View style={styles.meDot} />
          </View>
        </Marker>
      ) : null}
    </MapView>
  );
}

const styles = StyleSheet.create({
  pin: {
    width: 12,
    height: 12,
    borderRadius: 6,
    backgroundColor: colors.terracottaDark,
    borderWidth: 2,
    borderColor: colors.cream,
  },
  stepPin: {
    width: 26,
    height: 26,
    borderRadius: 13,
    backgroundColor: colors.terracottaDark,
    borderWidth: 2,
    borderColor: colors.cream,
    alignItems: "center",
    justifyContent: "center",
  },
  stepPinText: { fontFamily: fonts.bodyBold, fontSize: 12, color: colors.cream },
  // A distinct blue (colors.groundText, already used elsewhere for the
  // "sources verified" badge) rather than terracotta -- the user's own
  // position must never be confused with a place pin at a glance.
  me: {
    width: 34,
    height: 34,
    borderRadius: 17,
    backgroundColor: "rgba(46,90,172,0.22)",
    alignItems: "center",
    justifyContent: "center",
  },
  meDot: {
    width: 15,
    height: 15,
    borderRadius: 7.5,
    backgroundColor: colors.groundText,
    borderWidth: 2.5,
    borderColor: colors.cream,
  },
});
