// mobile/src/screens/FeaturedItineraryDetail.tsx
import React from "react";
import { View, Text, Pressable, ScrollView, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { formatDuration, formatItinerarySummary } from "../utils/itineraryFormat";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "FeaturedItineraryDetail">;

export function FeaturedItineraryDetailScreen({ route, navigation }: Props) {
  const { t } = useLocale();
  const { itinerary } = route.params;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <Text style={styles.title}>{itinerary.title}</Text>
        <Text style={styles.meta}>{formatItinerarySummary(itinerary, t)}</Text>

        <View style={styles.timeline}>
          {(() => {
            let placeNumber = 0;
            return itinerary.stops.map((stop, i) => {
              const isSuggestion = stop.kind === "suggestion";
              if (!isSuggestion) placeNumber += 1;
              const isLast = i === itinerary.stops.length - 1;
              return (
                <View key={i} style={styles.stopRow}>
                  <View style={styles.badgeColumn}>
                    <View style={[styles.badge, isSuggestion && styles.badgeSuggestion]}>
                      {isSuggestion ? (
                        <Text style={styles.badgeEmoji}>🍽️</Text>
                      ) : (
                        <Text style={styles.badgeText}>{placeNumber}</Text>
                      )}
                    </View>
                    {!isLast && <View style={styles.connector} />}
                  </View>
                  <View style={styles.stopBody}>
                    <Text style={[styles.stopLabel, isSuggestion && styles.stopLabelSuggestion]}>
                      {stop.label}
                    </Text>
                    {isSuggestion ? (
                      <Text style={styles.suggestionCaption}>{t.itineraries.suggestionLabel}</Text>
                    ) : (
                      <Text style={styles.stopMeta}>{formatDuration(stop.timeOnSiteMinutes)}</Text>
                    )}
                    {!isLast && stop.walkToNextMinutes > 0 && (
                      <Text style={styles.walkMeta}>
                        {t.itineraries.walkToNext.replace("{minutes}", String(stop.walkToNextMinutes))}
                      </Text>
                    )}
                  </View>
                </View>
              );
            });
          })()}
        </View>
      </ScrollView>
      <Pressable style={styles.startBtn} onPress={() => navigation.navigate("Map")}>
        <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
          <Path d="M5 12h14M13 5l7 7-7 7" stroke={colors.cream} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
        </Svg>
        <Text style={styles.startBtnText}>{t.itineraries.startItinerary}</Text>
      </Pressable>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { flexDirection: "row", alignItems: "center", paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  content: { paddingHorizontal: 22, paddingTop: 16, paddingBottom: 40 },
  title: { fontFamily: fonts.display, fontSize: 24, color: colors.ink, marginBottom: 6 },
  meta: { fontFamily: fonts.body, fontSize: 13.5, color: colors.inkSoft, marginBottom: 24 },
  timeline: {},
  stopRow: { flexDirection: "row", gap: 14 },
  badgeColumn: { alignItems: "center", width: 28 },
  badge: {
    width: 28,
    height: 28,
    borderRadius: 14,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  badgeSuggestion: {
    backgroundColor: "transparent",
    borderWidth: 1.5,
    borderColor: colors.inkFaint,
    borderStyle: "dashed",
  },
  badgeText: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.cream },
  badgeEmoji: { fontSize: 12 },
  connector: { width: 2, flex: 1, minHeight: 24, backgroundColor: colors.line, marginTop: 2 },
  stopBody: { flex: 1, paddingBottom: 22 },
  stopLabel: { fontFamily: fonts.bodySemiBold, fontSize: 15.5, color: colors.ink },
  stopLabelSuggestion: { fontStyle: "italic", color: colors.inkSoft },
  stopMeta: { fontFamily: fonts.body, fontSize: 12.5, color: colors.inkSoft, marginTop: 2 },
  suggestionCaption: { fontFamily: fonts.body, fontSize: 12, fontStyle: "italic", color: colors.inkFaint, marginTop: 2 },
  walkMeta: { fontFamily: fonts.body, fontSize: 12, color: colors.inkFaint, marginTop: 8 },
  startBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    backgroundColor: colors.terracotta,
    borderRadius: radii.md,
    marginHorizontal: 20,
    marginBottom: 16,
    paddingVertical: 16,
  },
  startBtnText: { fontFamily: fonts.bodyBold, fontSize: 15, color: colors.cream },
});
