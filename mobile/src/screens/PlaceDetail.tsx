// mobile/src/screens/PlaceDetail.tsx
import React, { useEffect, useState } from "react";
import { View, Text, Pressable, Image, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { LinearGradient } from "expo-linear-gradient";
import { useAudioPlayer, useAudioPlayerStatus } from "expo-audio";
import Svg, { Polyline, Path, Line } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { placesRepository } from "../data/PlacesRepository";
import type { Place, AudioAvailability } from "../data/types";
import { colors, fonts, radii } from "../theme/tokens";
import { fetchWordMarks, type WordMark } from "../utils/syncedText";
import { SyncedNarration, type NarrationMode } from "../components/SyncedNarration";
import { AudioProgressBar } from "../components/AudioProgressBar";

type Props = NativeStackScreenProps<AppStackParamList, "PlaceDetail">;

// PlaceDetail is an immersive "now playing" screen with its own dark
// palette, derived from the app's existing brand tokens rather than an
// invented one. #1C0E07 is not a new color: it's the existing hero
// gradient's base (rgba(28,14,7,...), below), reused as a solid fill so
// the whole screen reads as one continuous dark surface, not just the
// hero image. Scoped to this screen's own files only -- mobile/src/theme/tokens.ts,
// shared by every other screen, is untouched.
const DARK_BG = "#1C0E07";
const DIM_55 = "rgba(250,245,238,0.55)";
const DIM_45 = "rgba(250,245,238,0.45)";
const DIM_18 = "rgba(250,245,238,0.18)";
const TINT_ACTIVE = "rgba(193,89,46,0.18)";

// currentTime/duration come from expo-audio's AudioStatus in seconds
// (fractional while loading) -- "0:00" rather than "0:NaN" before a source
// has loaded.
function formatTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60)
    .toString()
    .padStart(2, "0");
  return `${m}:${s}`;
}

export function PlaceDetailScreen({ route, navigation }: Props) {
  const { t, locale } = useLocale();
  const [place, setPlace] = useState<Place | null>(null);
  const [audio, setAudio] = useState<AudioAvailability>({ state: "unavailable" });
  const [marks, setMarks] = useState<WordMark[] | null>(null);
  const [narrationMode, setNarrationMode] = useState<NarrationMode>("scroll");

  useEffect(() => {
    placesRepository.getById(route.params.placeId).then((p) => setPlace(p ?? null));
  }, [route.params.placeId]);

  // Narration audio and text both follow the app's own reading locale --
  // there is no more per-place language override (the removed language
  // pills). Changing language happens once, in Settings.
  useEffect(() => {
    if (!place) return;
    let cancelled = false;
    placesRepository.getAudioUrl(place.id, locale).then((result) => {
      if (!cancelled) setAudio(result);
    });
    return () => {
      cancelled = true;
    };
  }, [place?.id, locale]);

  // Vidé immédiatement, avant même que le fetch ne réponde -- sinon les
  // marks de l'ancienne langue resteraient affichées un instant pendant la
  // transition, surlignant les mauvais mots.
  useEffect(() => {
    setMarks(null);
    if (audio.state !== "ready" || !audio.timestampsUrl) return;
    let cancelled = false;
    fetchWordMarks(audio.timestampsUrl).then((result) => {
      if (!cancelled) setMarks(result);
    });
    return () => {
      cancelled = true;
    };
  }, [audio]);

  // Hooks must run unconditionally on every render -- source is null until
  // audio.state is "ready", which useAudioPlayer accepts (no source loaded
  // yet, not an error).
  const player = useAudioPlayer(audio.state === "ready" ? audio.url : null);
  const status = useAudioPlayerStatus(player);

  if (!place) return null;

  const canPlay = audio.state === "ready";
  // Falls back to the raw backend value for any category not yet in the
  // dictionary, rather than showing nothing.
  const categoryLabel =
    (t.categories as Record<string, string>)[place.category] ?? place.category;
  const progress = status.duration > 0 ? status.currentTime / status.duration : 0;

  return (
    <View style={styles.screen}>
      <View style={styles.hero}>
        <Image
          source={require("../../assets/images/place-hero.jpg")}
          style={StyleSheet.absoluteFill}
          resizeMode="cover"
        />
        <LinearGradient
          colors={["rgba(28,14,7,0.78)", "rgba(28,14,7,0.22)", "rgba(28,14,7,0)"]}
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
            {place.neighborhood ? `${categoryLabel} · ${place.neighborhood}` : categoryLabel}
          </Text>
          <Text style={styles.title}>{place.name}</Text>
        </View>
      </View>

      <SafeAreaView edges={["bottom"]} style={styles.rest}>
        <View style={styles.progressSection}>
          <AudioProgressBar
            progress={progress}
            onSeek={(fraction) => {
              if (status.duration > 0) player.seekTo(fraction * status.duration);
            }}
          />
          <View style={styles.timeRow}>
            <Text style={styles.timeText}>{formatTime(status.currentTime)}</Text>
            <Text style={styles.timeText}>
              {canPlay
                ? formatTime(status.duration)
                : audio.state === "pending"
                  ? t.placeDetail.narrationPending
                  : t.placeDetail.narrationUnavailable}
            </Text>
          </View>
        </View>

        <View style={styles.playRow}>
          <Pressable
            style={[styles.playBtn, !canPlay && styles.playBtnDisabled]}
            disabled={!canPlay}
            onPress={() => (status.playing ? player.pause() : player.play())}
          >
            {status.playing ? (
              <Svg width={20} height={20} viewBox="0 0 24 24" fill={colors.cream}>
                <Path d="M6 4h4v16H6zM14 4h4v16h-4z" />
              </Svg>
            ) : (
              <Svg width={20} height={20} viewBox="0 0 24 24" fill={colors.cream}>
                <Path d="M6 4l14 8-14 8V4z" />
              </Svg>
            )}
          </Pressable>
        </View>

        {place.narrationStatus === "ready" ? (
          <>
            <View style={styles.toggleRow}>
              <Pressable
                style={[styles.toggleBtn, narrationMode === "scroll" && styles.toggleBtnActive]}
                onPress={() => setNarrationMode("scroll")}
              >
                <Svg width={17} height={17} viewBox="0 0 24 24" fill="none">
                  <Line x1={4} y1={7} x2={20} y2={7} stroke={narrationMode === "scroll" ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                  <Line x1={4} y1={12} x2={16} y2={12} stroke={narrationMode === "scroll" ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                  <Line x1={4} y1={17} x2={12} y2={17} stroke={narrationMode === "scroll" ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                </Svg>
              </Pressable>
              <Pressable
                style={[styles.toggleBtn, narrationMode === "free" && styles.toggleBtnActive]}
                onPress={() => setNarrationMode("free")}
              >
                <Svg width={17} height={17} viewBox="0 0 24 24" fill="none">
                  <Path d="M4 5.5C4 5.5 6 4.5 9 4.5S13 5.5 13 5.5V18.5C13 18.5 11 17.5 9 17.5S4 18.5 4 18.5V5.5Z" stroke={narrationMode === "free" ? colors.terracotta : DIM_45} strokeWidth={1.7} strokeLinejoin="round" />
                  <Path d="M20 5.5C20 5.5 18 4.5 15 4.5S11 5.5 11 5.5V18.5C11 18.5 13 17.5 15 17.5S20 18.5 20 18.5V5.5Z" stroke={narrationMode === "free" ? colors.terracotta : DIM_45} strokeWidth={1.7} strokeLinejoin="round" />
                </Svg>
              </Pressable>
            </View>

            <View style={styles.lyricsArea}>
              <SyncedNarration
                text={place.body}
                marks={marks}
                currentTimeMs={status.currentTime * 1000}
                mode={narrationMode}
              />
            </View>

            <View style={styles.ground}>
              <Svg width={12} height={12} viewBox="0 0 24 24" fill="none">
                <Polyline points="5 13 10 18 19 7" stroke={colors.groundText} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
              </Svg>
              <Text style={styles.groundText}>{t.placeDetail.groundBadge}</Text>
            </View>
          </>
        ) : (
          <View style={styles.lyricsArea}>
            <Text style={styles.pendingText}>
              {place.narrationStatus === "pending"
                ? t.placeDetail.narrationPending
                : t.placeDetail.narrationUnavailable}
            </Text>
          </View>
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
            <Polyline points="9 6 15 12 9 18" stroke={DIM_45} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </SafeAreaView>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: DARK_BG },
  hero: {
    height: 320,
    backgroundColor: colors.sand,
    justifyContent: "space-between",
    // Without this, react-native-web's absolutely-positioned cover Image
    // (and its gradient overlay) can render taller than this fixed-height
    // container and bleed into the content below on web -- native clips
    // this automatically, web doesn't.
    overflow: "hidden",
  },
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
  rest: { flex: 1 },
  progressSection: { paddingHorizontal: 20, paddingTop: 24 },
  timeRow: { flexDirection: "row", justifyContent: "space-between", marginTop: 8 },
  timeText: { fontFamily: fonts.body, fontSize: 12, color: DIM_55 },
  playRow: { alignItems: "center", marginTop: 20 },
  playBtn: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  playBtnDisabled: { backgroundColor: "rgba(193,89,46,0.35)" },
  toggleRow: { flexDirection: "row", justifyContent: "flex-end", gap: 8, paddingHorizontal: 20, marginTop: 28 },
  toggleBtn: { width: 34, height: 34, borderRadius: radii.md, alignItems: "center", justifyContent: "center" },
  toggleBtnActive: { backgroundColor: TINT_ACTIVE },
  lyricsArea: { flex: 1, minHeight: 0, paddingHorizontal: 32, justifyContent: "center" },
  pendingText: { fontFamily: fonts.body, fontSize: 15, lineHeight: 24, color: DIM_55, textAlign: "center" },
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
  ask: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    borderWidth: 1,
    borderColor: DIM_18,
    borderRadius: radii.md,
    padding: 14,
    marginHorizontal: 20,
    marginTop: 16,
    marginBottom: 12,
  },
  askText: { flex: 1, fontFamily: fonts.bodyBold, fontSize: 14.5, color: colors.cream },
});
