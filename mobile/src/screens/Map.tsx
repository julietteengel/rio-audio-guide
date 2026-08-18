import React, { useEffect, useState } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Circle, Line, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { placesRepository } from "../data/PlacesRepository";
import type { Place } from "../data/types";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "Map">;

// Fixed placeholder positions matching the approved prototype — this is not a
// real map (no map SDK is wired up yet, see the final report's backend/gap notes).
const PIN_POSITIONS = [
  { left: 66, top: 140 },
  { left: 158, top: 150 },
  { left: 108, top: 200 },
  { left: 258, top: 128 },
  { left: 78, top: 268 },
  { left: 228, top: 288 },
];

export function MapScreen({ navigation }: Props) {
  const { t } = useLocale();
  const [nearby, setNearby] = useState<Place | null>(null);
  const [offlineCount, setOfflineCount] = useState(0);

  useEffect(() => {
    placesRepository.listNearby().then((places) => setNearby(places[0] ?? null));
    placesRepository.downloadedCount().then(setOfflineCount);
  }, []);

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

      <View style={styles.map}>
        <View style={styles.search}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Circle cx={11} cy={11} r={7} stroke={colors.inkFaint} strokeWidth={2} />
            <Line x1={21} y1={21} x2={16.65} y2={16.65} stroke={colors.inkFaint} strokeWidth={2} strokeLinecap="round" />
          </Svg>
          <Text style={styles.searchText}>{t.map.searchPlaceholder}</Text>
        </View>

        {PIN_POSITIONS.map((pos, i) => (
          <View key={i} style={[styles.pin, { left: pos.left, top: pos.top }]} />
        ))}
        <View style={[styles.me, { left: 172, top: 218 }]}>
          <View style={styles.meDot} />
        </View>
      </View>

      {nearby ? (
        <Pressable
          style={styles.nearby}
          onPress={() => navigation.navigate("PlaceDetail", { placeId: nearby.id })}
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
            <Text style={styles.nearbyTitle}>{nearby.name}</Text>
            <Text style={styles.nearbySub}>
              {t.map.nearbyDistance.replace("{distance}", `${nearby.distanceMeters} m`)}
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
  searchText: { fontFamily: fonts.body, fontSize: 14, color: colors.inkFaint },
  pin: {
    position: "absolute",
    width: 12,
    height: 12,
    borderRadius: 6,
    backgroundColor: colors.terracottaDark,
    borderWidth: 2,
    borderColor: colors.cream,
  },
  me: {
    position: "absolute",
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
