import React from "react";
import { View, StyleSheet } from "react-native";
import MapView, { Marker } from "react-native-maps";
import type { Place } from "../data/types";
import type { LatLon } from "../utils/geo";
import { colors } from "../theme/tokens";

export type PlaceMapRegion = {
  latitude: number;
  longitude: number;
  latitudeDelta: number;
  longitudeDelta: number;
};

export type PlaceMapProps = {
  places: Place[];
  region: PlaceMapRegion;
  userLocation: LatLon | null;
  youAreHereLabel: string;
  onSelectPlace: (placeId: string) => void;
};

// Native implementation, react-native-maps. See PlaceMap.web.tsx for the
// web counterpart (Leaflet) -- same props, Metro picks whichever file
// matches the target platform, so Map.tsx just renders <PlaceMap ... />
// without any Platform branching of its own.
export function PlaceMap({ places, region, userLocation, youAreHereLabel, onSelectPlace }: PlaceMapProps) {
  return (
    <MapView style={StyleSheet.absoluteFill} initialRegion={region}>
      {places.map((p) => (
        <Marker
          key={p.id}
          coordinate={{ latitude: p.lat, longitude: p.lon }}
          onPress={() => onSelectPlace(p.id)}
          title={p.name}
        >
          <View style={styles.pin} />
        </Marker>
      ))}
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
  me: {
    width: 34,
    height: 34,
    borderRadius: 17,
    backgroundColor: "rgba(193,89,46,0.22)",
    alignItems: "center",
    justifyContent: "center",
  },
  meDot: {
    width: 15,
    height: 15,
    borderRadius: 7.5,
    backgroundColor: colors.terracotta,
    borderWidth: 2.5,
    borderColor: colors.cream,
  },
});
