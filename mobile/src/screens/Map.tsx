import React, { useEffect, useMemo, useState } from "react";
import { View, Text, Pressable, TextInput, StyleSheet, Platform, ScrollView } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import * as Location from "expo-location";
import Svg, { Circle, Line, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { placesRepository } from "../data/PlacesRepository";
import { fetchCityManifest, RIO_CITY_SLUG } from "../data/downloadManager";
import type { Place } from "../data/types";
import { haversineMeters, formatDistance, type LatLon } from "../utils/geo";
import { colors, fonts, radii } from "../theme/tokens";

// react-native-maps has no real web implementation — a static `import`
// still gets bundled and executed by Metro's module system on every
// platform, so it must be loaded through a runtime `require()` gated on
// Platform.OS, not a top-level `import`, or the browser build throws at
// load time before anything renders. Native (iOS/Android) gets the real
// map; web gets a plain scrollable place list below (see WebPlaceList).
let MapView: any = null;
let Marker: any = null;
if (Platform.OS !== "web") {
  const Maps = require("react-native-maps");
  MapView = Maps.default;
  Marker = Maps.Marker;
}

type Props = NativeStackScreenProps<AppStackParamList, "Map">;

// Rio de Janeiro municipality, roughly centered — matches the backend's own
// bounding box for /places (rioMinLat/rioMaxLat in places_handler.go), not
// an arbitrary choice.
const RIO_REGION = {
  latitude: -22.9068,
  longitude: -43.1958,
  latitudeDelta: 0.35,
  longitudeDelta: 0.35,
};

export function MapScreen({ navigation }: Props) {
  const { t, locale } = useLocale();
  const [places, setPlaces] = useState<Place[]>([]);
  const [offlineCount, setOfflineCount] = useState(0);
  const [userLocation, setUserLocation] = useState<LatLon | null>(null);
  const [locationDenied, setLocationDenied] = useState(false);
  const [query, setQuery] = useState("");
  const [activeCategory, setActiveCategory] = useState<string | null>(null);
  const [audioOnly, setAudioOnly] = useState(false);
  // IDs with real, ready, published audio in the app's current language --
  // GET /cities/rio/manifest?language=X already computes exactly this
  // (script published AND audio ready, one cached call), reusing it here
  // instead of a per-place /audio call x252 or trusting a stale local flag.
  const [audioReadyIds, setAudioReadyIds] = useState<Set<string> | null>(null);

  useEffect(() => {
    fetchCityManifest(RIO_CITY_SLUG, locale).then((ready) => {
      setAudioReadyIds(new Set(ready.map((p) => p.id)));
    });
  }, [locale]);

  // Every category actually present in the loaded list, not a hardcoded
  // set -- stays correct if new categories show up without a frontend
  // change, and never offers a filter chip with zero matching places.
  const categories = useMemo(() => {
    const seen = new Set<string>();
    for (const p of places) seen.add(p.category);
    return Array.from(seen).sort();
  }, [places]);

  // Local filter over the already-loaded list, not a network call per
  // keystroke -- listNearby() already fetched every place once above, and
  // placesRepository.search() hitting the backend on every character typed
  // would be both slower and unnecessary for a list this size.
  const visiblePlaces = useMemo(() => {
    const q = query.trim().toLowerCase();
    return places.filter((p) => {
      if (q && !p.name.toLowerCase().includes(q)) return false;
      if (activeCategory && p.category !== activeCategory) return false;
      if (audioOnly && !audioReadyIds?.has(p.id)) return false;
      return true;
    });
  }, [places, query, activeCategory, audioOnly, audioReadyIds]);

  useEffect(() => {
    placesRepository.listNearby().then(setPlaces);
    placesRepository.downloadedCount().then(setOfflineCount);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const { status } = await Location.requestForegroundPermissionsAsync();
      if (status !== "granted") {
        if (!cancelled) setLocationDenied(true);
        return;
      }
      try {
        const position = await Location.getCurrentPositionAsync({});
        if (!cancelled) {
          setUserLocation({
            latitude: position.coords.latitude,
            longitude: position.coords.longitude,
          });
        }
      } catch {
        if (!cancelled) setLocationDenied(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // The nearest place to the user's real position, with a real distance —
  // only computable once location permission is granted and resolved. With
  // no location, fall back to the first place and no distance rather than
  // hiding the card entirely or showing a fabricated number.
  const nearby = useMemo(() => {
    if (places.length === 0) return null;
    if (!userLocation) return { place: places[0], distanceMeters: null as number | null };
    let closest = places[0];
    let closestDistance = haversineMeters(userLocation, { latitude: closest.lat, longitude: closest.lon });
    for (const p of places.slice(1)) {
      const d = haversineMeters(userLocation, { latitude: p.lat, longitude: p.lon });
      if (d < closestDistance) {
        closest = p;
        closestDistance = d;
      }
    }
    return { place: closest, distanceMeters: closestDistance };
  }, [places, userLocation]);

  return (
    <View style={styles.screen}>
      <SafeAreaView edges={["top"]} style={styles.header}>
        <Text style={styles.wordmark}>Memória Carioca</Text>
        <View style={styles.headerRight}>
          <View style={styles.badge}>
            <View style={styles.badgeDot} />
            <Text style={styles.badgeText}>
              {t.map.offlineBadge.replace("{count}", String(offlineCount))}
            </Text>
          </View>
          <Pressable style={styles.gearBtn} onPress={() => navigation.navigate("Settings")}>
            <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
              <Circle cx={12} cy={12} r={3} stroke={colors.ink} strokeWidth={2} />
              <Path
                d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
                stroke={colors.ink}
                strokeWidth={2}
              />
            </Svg>
          </Pressable>
        </View>
      </SafeAreaView>

      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        style={styles.filterRow}
        contentContainerStyle={styles.filterRowContent}
      >
        <Pressable
          style={[styles.chip, activeCategory === null && styles.chipActive]}
          onPress={() => setActiveCategory(null)}
        >
          <Text style={[styles.chipText, activeCategory === null && styles.chipTextActive]}>
            {t.map.allCategories}
          </Text>
        </Pressable>
        {categories.map((cat) => (
          <Pressable
            key={cat}
            style={[styles.chip, activeCategory === cat && styles.chipActive]}
            onPress={() => setActiveCategory(activeCategory === cat ? null : cat)}
          >
            <Text style={[styles.chipText, activeCategory === cat && styles.chipTextActive]}>
              {(t.categories as Record<string, string>)[cat] ?? cat}
            </Text>
          </Pressable>
        ))}
        <Pressable
          style={[styles.chip, styles.chipDivider, audioOnly && styles.chipActive]}
          onPress={() => setAudioOnly((v) => !v)}
        >
          <Text style={[styles.chipText, audioOnly && styles.chipTextActive]}>
            {t.map.audioOnlyFilter}
          </Text>
        </Pressable>
      </ScrollView>

      <View style={styles.map}>
        {Platform.OS === "web" ? (
          <ScrollView style={StyleSheet.absoluteFill} contentContainerStyle={styles.webListContent}>
            <Text style={styles.webListNotice}>{t.map.webMapUnavailable}</Text>
            {visiblePlaces.map((p) => (
              <Pressable
                key={p.id}
                style={styles.webListRow}
                onPress={() => navigation.navigate("PlaceDetail", { placeId: p.id })}
              >
                <View style={styles.pin} />
                <View style={styles.nearbyText}>
                  <Text style={styles.nearbyTitle}>{p.name}</Text>
                  <Text style={styles.nearbySub}>
                    {(t.categories as Record<string, string>)[p.category] ?? p.category}
                  </Text>
                </View>
              </Pressable>
            ))}
          </ScrollView>
        ) : (
          <MapView style={StyleSheet.absoluteFill} initialRegion={RIO_REGION}>
            {visiblePlaces.map((p) => (
              <Marker
                key={p.id}
                coordinate={{ latitude: p.lat, longitude: p.lon }}
                onPress={() => navigation.navigate("PlaceDetail", { placeId: p.id })}
                title={p.name}
              >
                <View style={styles.pin} />
              </Marker>
            ))}
            {userLocation ? (
              <Marker coordinate={userLocation} anchor={{ x: 0.5, y: 0.5 }} title={t.map.youAreHere}>
                <View style={styles.me}>
                  <View style={styles.meDot} />
                </View>
              </Marker>
            ) : null}
          </MapView>
        )}

        <View style={styles.search}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Circle cx={11} cy={11} r={7} stroke={colors.inkFaint} strokeWidth={2} />
            <Line x1={21} y1={21} x2={16.65} y2={16.65} stroke={colors.inkFaint} strokeWidth={2} strokeLinecap="round" />
          </Svg>
          <TextInput
            style={[styles.searchText, { color: colors.ink }]}
            placeholder={t.map.searchPlaceholder}
            placeholderTextColor={colors.inkFaint}
            value={query}
            onChangeText={setQuery}
          />
        </View>
      </View>

      {nearby ? (
        <Pressable
          style={styles.nearby}
          onPress={() => navigation.navigate("PlaceDetail", { placeId: nearby.place.id })}
        >
          <Svg width={20} height={20} viewBox="0 0 24 24" fill="none">
            <Path
              d="M12 21s-7-5.686-7-11a7 7 0 0 1 14 0c0 5.314-7 11-7 11z"
              stroke={colors.terracotta}
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <Circle cx={12} cy={10} r={2.5} stroke={colors.terracotta} strokeWidth={2} />
          </Svg>
          <View style={styles.nearbyText}>
            <Text style={styles.nearbyTitle}>{nearby.place.name}</Text>
            <Text style={styles.nearbySub}>
              {nearby.distanceMeters !== null
                ? t.map.nearbyDistance.replace("{distance}", formatDistance(nearby.distanceMeters))
                : locationDenied
                  ? t.map.locationDenied
                  : t.map.locatingYou}
            </Text>
          </View>
          <View style={styles.playBtn}>
            <Svg width={14} height={14} viewBox="0 0 24 24" fill={colors.cream}>
              <Path d="M6 4l14 8-14 8V4z" />
            </Svg>
          </View>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  header: {
    backgroundColor: colors.white,
    borderBottomWidth: 1,
    borderBottomColor: colors.line,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: 18,
    paddingVertical: 12,
  },
  wordmark: { fontFamily: fonts.display, fontSize: 15, color: colors.ink },
  headerRight: { flexDirection: "row", alignItems: "center", gap: 10 },
  badge: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    backgroundColor: colors.sand,
    borderRadius: radii.pill,
    paddingVertical: 6,
    paddingHorizontal: 11,
  },
  badgeDot: { width: 6, height: 6, borderRadius: 3, backgroundColor: colors.badgeDot },
  badgeText: { fontFamily: fonts.bodyBold, fontSize: 11.5, color: colors.terracottaDark },
  gearBtn: {
    width: 34,
    height: 34,
    borderRadius: 17,
    backgroundColor: colors.cream,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  filterRow: {
    backgroundColor: colors.white,
    borderBottomWidth: 1,
    borderBottomColor: colors.line,
  },
  filterRowContent: {
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 18,
    paddingVertical: 10,
  },
  chip: {
    borderRadius: radii.pill,
    paddingVertical: 7,
    paddingHorizontal: 14,
    backgroundColor: colors.sand,
  },
  chipDivider: { marginLeft: 6 },
  chipActive: { backgroundColor: colors.terracotta },
  chipText: { fontFamily: fonts.bodyBold, fontSize: 12.5, color: colors.inkSoft },
  chipTextActive: { color: colors.cream },
  map: { flex: 1, backgroundColor: colors.sand },
  search: {
    position: "absolute",
    top: 16,
    left: 16,
    right: 16,
    zIndex: 3,
    backgroundColor: colors.white,
    borderRadius: radii.md,
    paddingVertical: 12,
    paddingHorizontal: 14,
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    shadowColor: "#2B211B",
    shadowOpacity: 0.1,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 4 },
    elevation: 3,
  },
  searchText: { flex: 1, fontFamily: fonts.body, fontSize: 14, color: colors.inkFaint },
  webListContent: { paddingTop: 76, paddingHorizontal: 16, paddingBottom: 100, gap: 10 },
  webListNotice: {
    fontFamily: fonts.body,
    fontSize: 12.5,
    color: colors.inkFaint,
    marginBottom: 4,
  },
  webListRow: {
    backgroundColor: colors.white,
    borderRadius: radii.lg,
    padding: 14,
    flexDirection: "row",
    alignItems: "center",
    gap: 14,
    borderWidth: 1,
    borderColor: colors.line,
  },
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
  nearby: {
    position: "absolute",
    left: 16,
    right: 16,
    bottom: 24,
    backgroundColor: colors.white,
    borderRadius: radii.lg,
    padding: 14,
    flexDirection: "row",
    alignItems: "center",
    gap: 14,
    shadowColor: "#2B211B",
    shadowOpacity: 0.14,
    shadowRadius: 18,
    shadowOffset: { width: 0, height: 8 },
    elevation: 4,
  },
  nearbyText: { flex: 1 },
  nearbyTitle: { fontFamily: fonts.bodyBold, fontSize: 15, color: colors.ink, marginBottom: 2 },
  nearbySub: { fontFamily: fonts.body, fontSize: 12.5, color: colors.inkSoft },
  playBtn: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
});
