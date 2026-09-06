// mobile/src/screens/PlaceDetail.tsx
import React, { useEffect, useRef, useState } from "react";
import { Animated, View, Text, Pressable, Image, StyleSheet, useWindowDimensions } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useAudioPlayer, useAudioPlayerStatus } from "expo-audio";
import Svg, { Polyline, Path, Line, Circle } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { placesRepository } from "../data/PlacesRepository";
import type { Place, AudioAvailability } from "../data/types";
import { colors, fonts, radii } from "../theme/tokens";
import { fetchWordMarks, type WordMark } from "../utils/syncedText";
import { SyncedNarration, type NarrationMode } from "../components/SyncedNarration";
import { AudioProgressBar } from "../components/AudioProgressBar";
import { alertInfo } from "../utils/platformAlert";

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
  // A fixed pixel size looks fine on a real phone's ~375-430px-wide screen
  // but reads as tiny on a wide desktop/tablet testing window -- sizing off
  // the actual window width keeps it proportionally the same "most of the
  // screen" square on both, capped so it doesn't become absurd on a large
  // display.
  const { width: windowWidth } = useWindowDimensions();
  const heroImageSize = Math.min(windowWidth * 0.72, 340);
  const [place, setPlace] = useState<Place | null>(null);
  const [audio, setAudio] = useState<AudioAvailability>({ state: "unavailable" });
  const [marks, setMarks] = useState<WordMark[] | null>(null);
  const [marksLoading, setMarksLoading] = useState(false);
  const [narrationMode, setNarrationMode] = useState<NarrationMode>("scroll");
  // Reading the first line before Play reads as a bug, not a preview --
  // once playback has genuinely started at least once, this stays true even
  // through a later pause/seek back to 0, so the placeholder never reappears
  // mid-listen.
  const [hasStarted, setHasStarted] = useState(false);

  useEffect(() => {
    let cancelled = false;
    placesRepository.getById(route.params.placeId).then((p) => {
      if (!cancelled) setPlace(p ?? null);
    });
    return () => {
      cancelled = true;
    };
  }, [route.params.placeId, locale]);

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
    if (audio.state !== "ready" || !audio.timestampsUrl) {
      setMarksLoading(false);
      return;
    }
    setMarksLoading(true);
    let cancelled = false;
    fetchWordMarks(audio.timestampsUrl).then((result) => {
      if (!cancelled) {
        setMarks(result);
        setMarksLoading(false);
      }
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

  useEffect(() => {
    if (status.playing) setHasStarted(true);
  }, [status.playing]);

  // React Navigation's own screen-transition animations don't run on web
  // (react-native-screens has no web implementation of them) -- this fades
  // the screen in on mount instead, entirely independent of the navigator,
  // so the jump from the rest of the app's light theme into this screen's
  // dark one reads as a deliberate dissolve rather than an abrupt cut on
  // every platform, not just wherever the native transition happens to work.
  const screenOpacity = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    Animated.timing(screenOpacity, { toValue: 1, duration: 350, useNativeDriver: true }).start();
  }, [screenOpacity]);

  if (!place) return null;

  const canPlay = audio.state === "ready";
  // Falls back to the raw backend value for any category not yet in the
  // dictionary, rather than showing nothing.
  const categoryLabel =
    (t.categories as Record<string, string>)[place.category] ?? place.category;
  const progress = status.duration > 0 ? status.currentTime / status.duration : 0;

  return (
    <Animated.View style={[styles.screen, { opacity: screenOpacity }]}>
      <SafeAreaView edges={["top"]} style={styles.topBar}>
        <Pressable style={styles.iconBtn} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.cream} strokeWidth={2.4} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </SafeAreaView>

      <View style={[styles.heroImageWrap, { width: heroImageSize, height: heroImageSize }]}>
        <Image
          source={require("../../assets/images/place-hero.jpg")}
          style={styles.heroImage}
          resizeMode="cover"
        />
      </View>

      <View style={styles.heroText}>
        <Text style={styles.eyebrow}>
          {place.neighborhood ? `${categoryLabel} · ${place.neighborhood}` : categoryLabel}
        </Text>
        <Text style={styles.title}>{place.name}</Text>
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
                style={[styles.toggleBtn, narrationMode === "scroll" && marks !== null && styles.toggleBtnActive]}
                onPress={() => setNarrationMode("scroll")}
              >
                <Svg width={17} height={17} viewBox="0 0 24 24" fill="none">
                  <Line x1={4} y1={7} x2={20} y2={7} stroke={narrationMode === "scroll" && marks !== null ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                  <Line x1={4} y1={12} x2={16} y2={12} stroke={narrationMode === "scroll" && marks !== null ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                  <Line x1={4} y1={17} x2={12} y2={17} stroke={narrationMode === "scroll" && marks !== null ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                </Svg>
              </Pressable>
              <Pressable
                style={[styles.toggleBtn, (narrationMode === "free" || marks === null) && styles.toggleBtnActive]}
                onPress={() => setNarrationMode("free")}
              >
                <Svg width={17} height={17} viewBox="0 0 24 24" fill="none">
                  <Path d="M4 5.5C4 5.5 6 4.5 9 4.5S13 5.5 13 5.5V18.5C13 18.5 11 17.5 9 17.5S4 18.5 4 18.5V5.5Z" stroke={narrationMode === "free" || marks === null ? colors.terracotta : DIM_45} strokeWidth={1.7} strokeLinejoin="round" />
                  <Path d="M20 5.5C20 5.5 18 4.5 15 4.5S11 5.5 11 5.5V18.5C11 18.5 13 17.5 15 17.5S20 18.5 20 18.5V5.5Z" stroke={narrationMode === "free" || marks === null ? colors.terracotta : DIM_45} strokeWidth={1.7} strokeLinejoin="round" />
                </Svg>
              </Pressable>
            </View>

            <View style={styles.lyricsArea}>
              <SyncedNarration
                text={place.body}
                marks={marks}
                marksLoading={marksLoading}
                hasStarted={hasStarted}
                pressPlayHint={t.placeDetail.pressPlayHint}
                currentTimeMs={status.currentTime * 1000}
                mode={narrationMode}
              />
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
          style={[styles.ask, place.narrationStatus !== "ready" && styles.askDisabled]}
          disabled={place.narrationStatus !== "ready"}
          onPress={() => navigation.navigate("Assistant", { placeId: place.id })}
        >
          <Svg width={22} height={22} viewBox="0 0 24 24" fill="none">
            <Path
              d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"
              stroke={colors.terracotta}
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </Svg>
          <Text style={styles.askText}>{t.placeDetail.ask}</Text>
          <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
            <Polyline points="9 6 15 12 9 18" stroke={DIM_45} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>

        <Pressable
          style={[styles.ask, styles.askLast]}
          onPress={() => alertInfo(t.placeDetail.itineraryComingSoonTitle, t.placeDetail.itineraryComingSoonBody)}
        >
          <Svg width={22} height={22} viewBox="0 0 24 24" fill="none">
            <Circle cx={12} cy={12} r={9} stroke={colors.terracotta} strokeWidth={2} />
            <Line x1={12} y1={8} x2={12} y2={16} stroke={colors.terracotta} strokeWidth={2} strokeLinecap="round" />
            <Line x1={8} y1={12} x2={16} y2={12} stroke={colors.terracotta} strokeWidth={2} strokeLinecap="round" />
          </Svg>
          <Text style={styles.askText}>{t.placeDetail.addToItinerary}</Text>
          <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
            <Polyline points="9 6 15 12 9 18" stroke={DIM_45} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </SafeAreaView>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: DARK_BG },
  topBar: { flexDirection: "row", justifyContent: "space-between", paddingHorizontal: 18, paddingTop: 8 },
  iconBtn: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: "rgba(250,245,238,0.1)",
    alignItems: "center",
    justifyContent: "center",
  },
  // A centered square image card, Spotify-album-art style, rather than a
  // full-bleed banner -- the generic hero photo (not yet a real per-place
  // image, see the backend gap list) reads better at this smaller,
  // deliberate size than stretched wide with text overlaid on top of it.
  heroImageWrap: {
    alignSelf: "center",
    borderRadius: radii.lg,
    overflow: "hidden",
    marginTop: 12,
    backgroundColor: colors.sand,
  },
  heroImage: { width: "100%", height: "100%" },
  heroText: { paddingHorizontal: 24, paddingTop: 20, paddingBottom: 4 },
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
  progressSection: { paddingHorizontal: 20, paddingTop: 16 },
  timeRow: { flexDirection: "row", justifyContent: "space-between", marginTop: 8 },
  timeText: { fontFamily: fonts.body, fontSize: 12, color: DIM_55 },
  playRow: { alignItems: "center", marginTop: 12 },
  playBtn: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  playBtnDisabled: { backgroundColor: "rgba(193,89,46,0.35)" },
  toggleRow: { flexDirection: "row", justifyContent: "flex-end", gap: 8, paddingHorizontal: 20, marginTop: 16 },
  toggleBtn: { width: 34, height: 34, borderRadius: radii.md, alignItems: "center", justifyContent: "center" },
  toggleBtnActive: { backgroundColor: TINT_ACTIVE },
  lyricsArea: { flex: 1, minHeight: 0, paddingHorizontal: 32, justifyContent: "center" },
  pendingText: { fontFamily: fonts.body, fontSize: 15, lineHeight: 24, color: DIM_55, textAlign: "center" },
  ask: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    backgroundColor: TINT_ACTIVE,
    borderWidth: 1,
    borderColor: DIM_18,
    borderRadius: radii.md,
    padding: 16,
    marginHorizontal: 20,
    marginTop: 12,
  },
  askText: { flex: 1, fontFamily: fonts.bodyBold, fontSize: 15, color: colors.cream },
  askLast: { marginBottom: 16 },
  askDisabled: { opacity: 0.4 },
});
