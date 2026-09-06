import React, { useEffect, useMemo, useRef } from "react";
import { Text, View, ScrollView, StyleSheet, Animated, ActivityIndicator, Easing } from "react-native";
import { findActiveLineIndex, groupMarksIntoLines, type WordMark } from "../utils/syncedText";
import { colors, fonts } from "../theme/tokens";

export type NarrationMode = "scroll" | "free";

type Props = {
  text: string;
  marks: WordMark[] | null;
  // Distinguishes "no marks yet because the fetch is still in flight" from
  // "confirmed no marks exist" -- without this, scroll mode briefly rendered
  // the full free-text fallback (marks === null) and then snapped to the
  // synced panel the instant marks arrived, a jarring flicker on every load.
  marksLoading: boolean;
  currentTimeMs: number;
  mode: NarrationMode;
};

// Dimmed previous/next lyric lines, sitting either side of the active line.
const DIM_TEXT = "rgba(250,245,238,0.45)";
// Matching the ~85% used for the hero eyebrow elsewhere in PlaceDetail.tsx
// -- this component now always renders against the same dark background,
// never the light theme it originally shared with the rest of the
// (pre-redesign) screen.
const FREE_TEXT = "rgba(250,245,238,0.85)";

export function SyncedNarration({ text, marks, marksLoading, currentTimeMs, mode }: Props) {
  // Always called, even on the free/fallback/loading paths below, to
  // satisfy the rules of hooks -- 26 keeps a grouped line to a single
  // rendered line at the active-line font size (22px bold in a ~326pt-wide
  // column), so a wrapped two-line group never reintroduces the minHeight
  // jump dimLine guards against.
  const lines = useMemo(() => groupMarksIntoLines(marks ?? [], 26), [marks]);

  // Clamped to 0: before the first line's timestamp (findActiveLineIndex
  // returns -1), the first line displays as the upcoming/active one rather
  // than showing nothing. Computed unconditionally (even when the loading/
  // free paths below will render something else entirely) so the
  // cross-fade hooks right after, which depend on it, can also run
  // unconditionally.
  const activeIndex = Math.max(findActiveLineIndex(lines, currentTimeMs), 0);

  // Slides the whole 3-line block up (or down, when time jumps backward --
  // e.g. a seek) into place each time the active line changes, instead of
  // an opacity cross-fade -- a fade reads as "blink out, blink back in",
  // not the continuous upward scroll Spotify's lyrics view actually does.
  // LINE_SHIFT approximates one row's height + the gap between rows
  // (dimLine's 22 + panel's 14 gap) -- close enough for a short, subtle
  // slide; it doesn't need to be pixel-exact to read as "sliding into place".
  const translateY = useRef(new Animated.Value(0)).current;
  const lastActiveIndex = useRef(activeIndex);
  useEffect(() => {
    if (lastActiveIndex.current === activeIndex) return;
    const movedForward = activeIndex > lastActiveIndex.current;
    lastActiveIndex.current = activeIndex;
    translateY.setValue(movedForward ? LINE_SHIFT : -LINE_SHIFT);
    Animated.timing(translateY, {
      toValue: 0,
      duration: 320,
      easing: Easing.out(Easing.cubic),
      useNativeDriver: true,
    }).start();
  }, [activeIndex, translateY]);

  // Marks are still being fetched for a place that DOES have audio -- show
  // a brief loading state rather than the free-text fallback below, which
  // would otherwise flash on screen for a moment and then snap to this
  // panel the instant marks arrive.
  if (mode === "scroll" && marksLoading) {
    return (
      <View style={styles.panel}>
        <ActivityIndicator color={colors.terracotta} />
      </View>
    );
  }

  // "Lecture libre" is the same rendering whether or not marks exist (no
  // marks at all falls back here too, once we're sure loading is finished)
  // -- it's the full narration text, scrollable by hand, completely
  // decoupled from playback position. It needs its own ScrollView: the
  // *screen* around this component doesn't scroll (a deliberate, separate
  // decision — PlaceDetail is a fixed single-viewport player), but the
  // narration text itself can run far longer than the space available to
  // it, so this inner area scrolls locally, within its own fixed-height slot.
  if (mode === "free" || !marks) {
    return (
      <ScrollView style={styles.freeScroll} contentContainerStyle={styles.freeContent}>
        <Text style={styles.freeText}>{text}</Text>
      </ScrollView>
    );
  }

  const prevLine = activeIndex > 0 ? lines[activeIndex - 1].text : "";
  const activeLine = lines[activeIndex]?.text ?? "";
  const nextLine = activeIndex + 1 < lines.length ? lines[activeIndex + 1].text : "";

  return (
    <View style={styles.panelClip}>
      <Animated.View style={[styles.panel, { transform: [{ translateY }] }]}>
        <Text style={styles.dimLine}>{prevLine}</Text>
        <Text style={styles.activeLine}>{activeLine}</Text>
        <Text style={styles.dimLine}>{nextLine}</Text>
      </Animated.View>
    </View>
  );
}

const LINE_SHIFT = 36;

const styles = StyleSheet.create({
  freeScroll: { flex: 1, width: "100%" },
  freeContent: { paddingVertical: 4 },
  freeText: {
    fontFamily: fonts.body,
    fontSize: 15,
    lineHeight: 24,
    color: FREE_TEXT,
  },
  panelClip: {
    // Clips the slide animation to this box so a line sliding in from
    // LINE_SHIFT away never visibly pokes outside the lyrics area.
    overflow: "hidden",
  },
  panel: {
    alignItems: "center",
    justifyContent: "center",
    gap: 14,
  },
  dimLine: {
    fontFamily: fonts.body,
    fontSize: 16,
    lineHeight: 22,
    color: DIM_TEXT,
    textAlign: "center",
    // Reserves a stable line height even when empty (first/last line has
    // no neighbor on one side) so the active line doesn't visually jump
    // up or down at the very start or end of playback.
    minHeight: 22,
  },
  activeLine: {
    fontFamily: fonts.bodyBold,
    fontSize: 22,
    lineHeight: 30,
    color: colors.cream,
    textAlign: "center",
  },
});
