import React, { useEffect, useMemo, useState } from "react";
import { View, Text, Pressable, TextInput, StyleSheet, ScrollView } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import * as Location from "expo-location";
import Svg, { Circle, Line, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { placesRepository } from "../data/PlacesRepository";
import { fetchCityManifest, RIO_CITY_SLUG } from "../data/downloadManager";
import type { Place } from "../data/types";
import { haversineMeters, formatDistance, type LatLon } from "../utils/geo";
import { normalizeForSearch } from "../utils/search";
import { listFeaturedItineraries, type Itinerary } from "../data/ItinerariesRepository";
import { formatItinerarySummary } from "../utils/itineraryFormat";
import { useActiveItinerary } from "../itinerary/ActiveItineraryContext";
import { colors, fonts, radii } from "../theme/tokens";
// Metro resolves PlaceMap.web.tsx on web, PlaceMap.tsx (react-native-maps)
// on native, automatically -- this import is platform-agnostic on purpose,
// see PlaceMap.tsx's own comment for why a plain top-level import of
// react-native-maps itself would break the web bundle.
import { PlaceMap } from "../components/PlaceMap";

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
  const { isLoggedIn } = useAuth();
  const {
    itinerary: activeItinerary,
    currentStop,
    currentPlaceStepNumber,
    totalPlaceSteps,
    distanceToCurrentStopMeters,
    justArrived,
    end: endActiveItinerary,
  } = useActiveItinerary();
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
    const q = normalizeForSearch(query);
    return places.filter((p) => {
      if (q) {
        // Place names stay in their original (Portuguese) spelling -- never
        // translated, only narration is -- so a query in the app's own
        // language rarely matches the name itself. Matching the category's
        // *translated* label too (the same text already shown on its
        // filter chip) lets "église"/"church"/"igreja" all find every
        // church regardless of which language the name happens to be in.
        const categoryLabel = (t.categories as Record<string, string>)[p.category] ?? p.category;
        const matchesName = normalizeForSearch(p.name).includes(q);
        const matchesCategory = normalizeForSearch(categoryLabel).includes(q);
        if (!matchesName && !matchesCategory) return false;
      }
      if (activeCategory && p.category !== activeCategory) return false;
      if (audioOnly && !audioReadyIds?.has(p.id)) return false;
      return true;
    });
  }, [places, query, activeCategory, audioOnly, audioReadyIds, t]);

  // 1-based step number per place, counting only "place" stops (matches the
  // numbered badges on ItineraryDetail/FeaturedItineraryDetail) -- undefined
  // for a place not in the active itinerary at all.
  const stepNumberByPlaceId = useMemo(() => {
    if (!activeItinerary) return null;
    const map = new Map<string, number>();
    let step = 0;
    for (const stop of activeItinerary.stops) {
      if (stop.kind !== "place") continue;
      step += 1;
      map.set(stop.placeId, step);
    }
    return map;
  }, [activeItinerary]);

  // While an itinerary is active, the map is "about this walk" -- showing
  // every other place in Rio alongside it just adds noise to exactly the
  // moment a clear numbered sequence matters most.
  const mapPlaces = useMemo(() => {
    if (!stepNumberByPlaceId) return visiblePlaces;
    return visiblePlaces.filter((p) => stepNumberByPlaceId.has(p.id));
  }, [visiblePlaces, stepNumberByPlaceId]);

  useEffect(() => {
    placesRepository.listNearby().then(setPlaces);
    placesRepository.downloadedCount().then(setOfflineCount);
  }, []);

  const [featuredItineraries, setFeaturedItineraries] = useState<Itinerary[]>([]);

  useEffect(() => {
    listFeaturedItineraries()
      .then(setFeaturedItineraries)
      .catch((err) => {
        // A failed fetch here just means the Discover section stays empty
        // -- it's a bonus surface, not core map functionality, so this
        // never blocks or degrades the rest of the screen. Still logged
        // (matching every other swallowed-but-non-blocking catch in this
        // codebase, e.g. geofenceTask.ts/downloadManager.ts/Settings.tsx)
        // so a real outage leaves a trace instead of just an empty row.
        console.warn("featured itineraries: fetch failed", err);
      });
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
          {/* This badge means "N places are available even with no signal" --
              it's a reassurance about what's downloaded, not a live connectivity
              status, so showing it for offlineCount === 0 (true for anyone who
              hasn't downloaded anything, i.e. most first-time visits) read as a
              bare, confusing "no connection" message with no count that made sense. */}
          {offlineCount > 0 && (
            <View style={styles.badge}>
              <View style={styles.badgeDot} />
              <Text style={styles.badgeText}>
                {t.map.offlineBadge.replace("{count}", String(offlineCount))}
              </Text>
            </View>
          )}
          <Pressable style={styles.gearBtn} onPress={() => navigation.navigate(isLoggedIn ? "ItinerariesList" : "Auth")}>
            <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
              <Circle cx={6} cy={7} r={2} stroke={colors.ink} strokeWidth={2} />
              <Circle cx={18} cy={17} r={2} stroke={colors.ink} strokeWidth={2} />
              <Path d="M8 8.5 16 15.5" stroke={colors.ink} strokeWidth={2} strokeLinecap="round" />
            </Svg>
          </Pressable>
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

      {activeItinerary && currentStop && (
        <View style={styles.progressBanner}>
          <View style={styles.progressBannerText}>
            <Text style={styles.progressBannerTitle} numberOfLines={1}>
              {activeItinerary.title}
            </Text>
            <Text style={styles.progressBannerStep}>
              {t.itineraries.progressStep
                .replace("{current}", String(currentPlaceStepNumber))
                .replace("{total}", String(totalPlaceSteps))}
            </Text>
            <Text style={styles.progressBannerLabel} numberOfLines={1}>
              {justArrived ? t.itineraries.arrivedAt : currentStop.label}
            </Text>
            {!justArrived && distanceToCurrentStopMeters !== null && (
              <Text style={styles.progressBannerDistance}>
                {formatDistance(distanceToCurrentStopMeters)}
              </Text>
            )}
          </View>
          <Pressable style={styles.progressBannerEnd} onPress={endActiveItinerary}>
            <Svg width={14} height={14} viewBox="0 0 24 24" fill="none">
              <Line x1={6} y1={6} x2={18} y2={18} stroke={colors.cream} strokeWidth={2.2} strokeLinecap="round" />
              <Line x1={18} y1={6} x2={6} y2={18} stroke={colors.cream} strokeWidth={2.2} strokeLinecap="round" />
            </Svg>
          </Pressable>
        </View>
      )}

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

      {/* Pointless (and confusing -- "discover other itineraries" while
          already following one) once an itinerary is active: this row is
          for choosing where to go, not for someone already underway. */}
      {!activeItinerary && featuredItineraries.length > 0 ? (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          style={styles.discoverRow}
          contentContainerStyle={styles.discoverRowContent}
        >
          <Text style={styles.discoverSectionTitle}>{t.map.discoverTitle}</Text>
          {featuredItineraries.map((it) => (
            <Pressable
              key={it.id}
              style={styles.discoverCard}
              onPress={() => navigation.navigate("FeaturedItineraryDetail", { itinerary: it })}
            >
              <Text style={styles.discoverCardTitle}>{it.title}</Text>
              <Text style={styles.discoverCardMeta}>{formatItinerarySummary(it, t)}</Text>
            </Pressable>
          ))}
        </ScrollView>
      ) : null}

      <View style={styles.map}>
        <PlaceMap
          places={mapPlaces}
          stepNumbers={stepNumberByPlaceId ?? undefined}
          region={RIO_REGION}
          userLocation={userLocation}
          youAreHereLabel={t.map.youAreHere}
          onSelectPlace={(placeId) => navigation.navigate("PlaceDetail", { placeId })}
        />

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
          {query.length > 0 && (
            <Pressable style={styles.searchClear} onPress={() => setQuery("")} hitSlop={8}>
              <Svg width={14} height={14} viewBox="0 0 24 24" fill="none">
                <Line x1={6} y1={6} x2={18} y2={18} stroke={colors.inkFaint} strokeWidth={2.2} strokeLinecap="round" />
                <Line x1={18} y1={6} x2={6} y2={18} stroke={colors.inkFaint} strokeWidth={2.2} strokeLinecap="round" />
              </Svg>
            </Pressable>
          )}
        </View>
      </View>

      {/* Showing "the nearest place" while following an itinerary competes
          with the one thing that screen is for: the current stop, already
          front and center in the progress banner above. Dropped here, not
          just visually hidden, so it can't also fire PlaceDetail navigation
          by accident. */}
      {!activeItinerary && nearby ? (
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
  progressBanner: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: colors.terracotta,
    paddingHorizontal: 16,
    paddingVertical: 10,
    gap: 10,
  },
  progressBannerText: { flex: 1 },
  progressBannerTitle: {
    fontFamily: fonts.bodyBold,
    fontSize: 12,
    color: colors.cream,
    marginBottom: 2,
  },
  progressBannerStep: {
    fontFamily: fonts.bodySemiBold,
    fontSize: 11,
    color: colors.cream,
    textTransform: "uppercase",
    letterSpacing: 0.4,
  },
  progressBannerLabel: { fontFamily: fonts.bodyBold, fontSize: 14.5, color: colors.cream, marginTop: 1 },
  progressBannerDistance: { fontFamily: fonts.body, fontSize: 12, color: colors.cream, marginTop: 1, opacity: 0.9 },
  progressBannerEnd: {
    width: 28,
    height: 28,
    borderRadius: 14,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "rgba(255,255,255,0.2)",
  },
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
    // ScrollView defaults to flexGrow:1 on react-native-web (unlike native,
    // where a horizontal ScrollView sizes to its content's height by
    // default) -- without this, it stretched to fill the space meant for
    // the map/list below, and the chips stretched with it into giant
    // vertical pills.
    flexGrow: 0,
    flexShrink: 0,
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
  discoverRow: {
    flexGrow: 0,
    flexShrink: 0,
    backgroundColor: colors.cream,
    borderBottomWidth: 1,
    borderBottomColor: colors.line,
  },
  discoverRowContent: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    paddingHorizontal: 18,
    paddingVertical: 12,
  },
  discoverSectionTitle: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.inkSoft, marginRight: 4 },
  discoverCard: {
    backgroundColor: colors.white,
    borderRadius: radii.md,
    borderWidth: 1,
    borderColor: colors.line,
    paddingVertical: 10,
    paddingHorizontal: 14,
    minWidth: 160,
  },
  discoverCardTitle: { fontFamily: fonts.bodySemiBold, fontSize: 13.5, color: colors.ink, marginBottom: 2 },
  discoverCardMeta: { fontFamily: fonts.body, fontSize: 11.5, color: colors.inkSoft },
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
  searchClear: { padding: 2 },
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
