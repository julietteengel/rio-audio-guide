import React, { useEffect, useRef } from "react";
import { View, Text, ScrollView, StyleSheet, type LayoutChangeEvent } from "react-native";
import { findActiveWordIndex, type WordMark } from "../utils/syncedText";
import { colors, fonts } from "../theme/tokens";

type Props = {
  text: string;
  marks: WordMark[] | null;
  currentTimeMs: number;
};

// Fraction de la hauteur visible qui délimite la "bande confortable" au
// centre -- le scroll ne bouge que quand le mot actif en sort, pas à chaque
// mot (ça donnerait un défilement saccadé plutôt que l'effet "paroles
// synchronisées" recherché).
const COMFORT_BAND_TOP = 0.3;
const COMFORT_BAND_BOTTOM = 0.7;

export function SyncedNarration({ text, marks, currentTimeMs }: Props) {
  const scrollRef = useRef<ScrollView>(null);
  const scrollYRef = useRef(0);
  const viewportHeightRef = useRef(0);
  const wordYPositions = useRef<number[]>([]);

  const activeIndex = marks ? findActiveWordIndex(marks, currentTimeMs) : -1;

  // Runs before the scroll effect below on the same commit, so a `marks`
  // swap (place/language change, no remount) clears out y-positions from
  // the previous narration before they can be read as if they were the
  // new one's -- fresh onLayout calls for the new words land later,
  // asynchronously, so without this reset a stale position can survive
  // long enough to drive one wrong scroll.
  useEffect(() => {
    wordYPositions.current = [];
  }, [marks]);

  useEffect(() => {
    if (activeIndex < 0) return;
    const wordY = wordYPositions.current[activeIndex];
    if (wordY === undefined) return;
    const viewportHeight = viewportHeightRef.current;
    if (viewportHeight === 0) return;
    const bandTop = scrollYRef.current + viewportHeight * COMFORT_BAND_TOP;
    const bandBottom = scrollYRef.current + viewportHeight * COMFORT_BAND_BOTTOM;
    if (wordY < bandTop || wordY > bandBottom) {
      scrollRef.current?.scrollTo({
        y: Math.max(0, wordY - viewportHeight * COMFORT_BAND_TOP),
        animated: true,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- ne doit
    // re-déclencher que sur un changement de mot actif, pas à chaque frame.
  }, [activeIndex]);

  if (!marks) {
    return <Text style={styles.body}>{text}</Text>;
  }

  return (
    <ScrollView
      ref={scrollRef}
      onScroll={(e) => {
        scrollYRef.current = e.nativeEvent.contentOffset.y;
      }}
      onLayout={(e) => {
        viewportHeightRef.current = e.nativeEvent.layout.height;
      }}
      scrollEventThrottle={100}
    >
      <View style={styles.wordWrap}>
        {marks.map((mark, i) => (
          <Text
            key={i}
            onLayout={(e: LayoutChangeEvent) => {
              wordYPositions.current[i] = e.nativeEvent.layout.y;
            }}
            style={[styles.body, i === activeIndex && styles.activeWord]}
          >
            {mark.value + " "}
          </Text>
        ))}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  body: {
    fontFamily: fonts.body,
    fontSize: 15,
    lineHeight: 24,
    color: colors.inkSoft,
  },
  wordWrap: { flexDirection: "row", flexWrap: "wrap" },
  activeWord: { color: colors.terracotta, fontFamily: fonts.bodyBold },
});
