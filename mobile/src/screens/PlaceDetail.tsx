import React, { useEffect, useState } from "react";
import { View, Text, Pressable, ScrollView, Image, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { LinearGradient } from "expo-linear-gradient";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import type { Locale } from "../i18n/dictionary";
import { useLocale } from "../i18n/LocaleContext";
import { placesRepository } from "../data/PlacesRepository";
import type { Place } from "../data/types";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "PlaceDetail">;

const LANG_ORDER: Locale[] = ["pt", "en", "fr", "es"];
const LANG_LABEL: Record<Locale, string> = { pt: "PT", en: "EN", fr: "FR", es: "ES" };

const WAVE_HEIGHTS_PLAYED = [8, 16, 24, 14, 22];
const WAVE_HEIGHTS_REST = [10, 18, 26, 12, 20, 9, 16];

export function PlaceDetailScreen({ route, navigation }: Props) {
  const { t } = useLocale();
  const [place, setPlace] = useState<Place | null>(null);
  const [playerLocale, setPlayerLocale] = useState<Locale>("pt");

  useEffect(() => {
    placesRepository.getById(route.params.placeId).then((p) => setPlace(p ?? null));
  }, [route.params.placeId]);

  if (!place) return null;

  const hasDuration = typeof place.audioDurationSeconds === "number";
  const minutes = hasDuration ? Math.floor(place.audioDurationSeconds! / 60) : 0;
  const seconds = hasDuration ? String(place.audioDurationSeconds! % 60).padStart(2, "0") : "--";

  return (
    <ScrollView style={styles.screen} contentContainerStyle={{ paddingBottom: 40 }}>
      <View style={styles.hero}>
        <Image
          source={require("../../assets/images/place-hero.jpg")}
          style={StyleSheet.absoluteFill}
          resizeMode="cover"
        />
        <LinearGradient
          colors={["rgba(28,14,7,0.72)", "rgba(28,14,7,0.15)", "rgba(28,14,7,0)"]}
          start={{ x: 0, y: 1 }}
          end={{ x: 0, y: 0 }}
          style={StyleSheet.absoluteFill}
        />
        <SafeAreaView edges={["top"]}>
          <Pressable style={styles.back} onPress={() => navigation.goBack()}>
            <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
              <Polyline points="15 6 9 12 15 18" stroke={colors.cream} strokeWidth={2.4} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
          </Pressable>
        </SafeAreaView>
        <View style={styles.heroText}>
          <Text style={styles.eyebrow}>
            {place.neighborhood ? `${place.category} · ${place.neighborhood}` : place.category}
          </Text>
          <Text style={styles.title}>{place.name}</Text>
        </View>
      </View>

      <View style={styles.langRow}>
        {LANG_ORDER.map((l) => (
          <Pressable
            key={l}
            onPress={() => setPlayerLocale(l)}
            style={[styles.pill, l === playerLocale && styles.pillActive]}
          >
            <Text style={[styles.pillText, l === playerLocale && styles.pillTextActive]}>
              {LANG_LABEL[l]}
            </Text>
          </Pressable>
        ))}
      </View>

      <View style={styles.player}>
        <View style={styles.playerTop}>
          <View style={styles.playBtn}>
            <Svg width={14} height={14} viewBox="0 0 24 24" fill={colors.cream}>
              <Path d="M6 4l14 8-14 8V4z" />
            </Svg>
          </View>
          <View style={styles.wave}>
            {WAVE_HEIGHTS_PLAYED.map((h, i) => (
              <View key={`p${i}`} style={[styles.bar, styles.barPlayed, { height: h }]} />
            ))}
            {WAVE_HEIGHTS_REST.map((h, i) => (
              <View key={`r${i}`} style={[styles.bar, { height: h }]} />
            ))}
          </View>
        </View>
        <View style={styles.timeRow}>
          <Text style={styles.timeText}>0:42</Text>
          <Text style={styles.timeText}>
            {minutes}:{seconds}
          </Text>
        </View>
      </View>

      {place.narrationStatus === "ready" ? (
        <>
          <View style={styles.ground}>
            <Svg width={12} height={12} viewBox="0 0 24 24" fill="none">
              <Polyline points="5 13 10 18 19 7" stroke={colors.groundText} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
            <Text style={styles.groundText}>{t.placeDetail.groundBadge}</Text>
          </View>
          <Text style={styles.body}>{place.body}</Text>
        </>
      ) : (
        <Text style={styles.body}>
          {place.narrationStatus === "pending"
            ? t.placeDetail.narrationPending
            : t.placeDetail.narrationUnavailable}
        </Text>
      )}

      <Pressable
        style={styles.ask}
        onPress={() => navigation.navigate("Assistant", { placeId: place.id })}
      >
        <Svg width={20} height={20} viewBox="0 0 24 24" fill="none">
          <Path
            d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"
            stroke={colors.terracotta}
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </Svg>
        <Text style={styles.askText}>{t.placeDetail.ask}</Text>
        <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
          <Polyline points="9 6 15 12 9 18" stroke={colors.inkFaint} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
        </Svg>
      </Pressable>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  hero: { height: 320, backgroundColor: colors.sand, justifyContent: "space-between" },
  back: {
    marginLeft: 18,
    marginTop: 8,
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: "rgba(28,14,7,0.4)",
    alignItems: "center",
    justifyContent: "center",
  },
  heroText: { paddingHorizontal: 20, paddingBottom: 18 },
  eyebrow: {
    fontFamily: fonts.bodyBold,
    fontSize: 11.5,
    letterSpacing: 1,
    textTransform: "uppercase",
    color: "rgba(250,245,238,0.85)",
    marginBottom: 6,
  },
  title: { fontFamily: fonts.displayBlack, fontSize: 30, color: colors.cream },
  langRow: { flexDirection: "row", gap: 8, paddingHorizontal: 20, paddingTop: 18 },
  pill: { borderRadius: radii.pill, paddingVertical: 8, paddingHorizontal: 16, backgroundColor: colors.sand },
  pillActive: { backgroundColor: colors.terracotta },
  pillText: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.inkSoft },
  pillTextActive: { color: colors.cream },
  player: {
    marginHorizontal: 20,
    marginTop: 18,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.lg,
    padding: 16,
  },
  playerTop: { flexDirection: "row", alignItems: "center", gap: 14 },
  playBtn: {
    width: 44,
    height: 44,
    borderRadius: 22,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  wave: { flex: 1, flexDirection: "row", alignItems: "center", gap: 3, height: 28 },
  bar: { width: 3, borderRadius: 2, backgroundColor: colors.sand },
  barPlayed: { backgroundColor: colors.terracotta },
  timeRow: { flexDirection: "row", justifyContent: "space-between", marginTop: 10 },
  timeText: { fontFamily: fonts.body, fontSize: 12, color: colors.inkFaint },
  ground: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    alignSelf: "flex-start",
    backgroundColor: colors.groundBg,
    borderRadius: radii.pill,
    paddingVertical: 7,
    paddingHorizontal: 12,
    marginHorizontal: 20,
    marginTop: 16,
  },
  groundText: { fontFamily: fonts.bodyBold, fontSize: 12, color: colors.groundText },
  body: {
    fontFamily: fonts.body,
    fontSize: 15,
    lineHeight: 24,
    color: colors.inkSoft,
    marginHorizontal: 20,
    marginTop: 18,
  },
  ask: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    padding: 14,
    marginHorizontal: 20,
    marginTop: 22,
  },
  askText: { flex: 1, fontFamily: fonts.bodyBold, fontSize: 14.5, color: colors.ink },
});
