import React, { useCallback, useState } from "react";
import { View, Text, Pressable, FlatList, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useFocusEffect } from "@react-navigation/native";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { listItineraries, type Itinerary } from "../data/ItinerariesRepository";
import { formatDuration } from "../utils/itineraryFormat";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ItinerariesList">;

export function ItinerariesListScreen({ navigation }: Props) {
  const { t } = useLocale();
  const { token } = useAuth();
  const [itineraries, setItineraries] = useState<Itinerary[] | null>(null);

  // Refetches every time this screen regains focus (not just on first
  // mount) -- coming back here after creating a new itinerary in the chat
  // screen should show it without a manual pull-to-refresh.
  useFocusEffect(
    useCallback(() => {
      if (!token) return;
      let cancelled = false;
      listItineraries(token)
        .then((result) => {
          if (!cancelled) setItineraries(result);
        })
        .catch(() => {
          if (!cancelled) setItineraries([]);
        });
      return () => {
        cancelled = true;
      };
    }, [token]),
  );

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
        <Text style={styles.h1}>{t.itineraries.listTitle}</Text>
      </View>

      <Pressable style={styles.createBtn} onPress={() => navigation.navigate("ItineraryChat")}>
        <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
          <Path d="M12 5v14M5 12h14" stroke={colors.cream} strokeWidth={2.2} strokeLinecap="round" />
        </Svg>
        <Text style={styles.createBtnText}>{t.itineraries.createButton}</Text>
      </Pressable>

      {itineraries === null ? (
        <ActivityIndicator style={styles.loading} color={colors.terracotta} />
      ) : itineraries.length === 0 ? (
        <View style={styles.empty}>
          <Text style={styles.emptyTitle}>{t.itineraries.listEmptyTitle}</Text>
          <Text style={styles.emptyBody}>{t.itineraries.listEmptyBody}</Text>
        </View>
      ) : (
        <FlatList
          data={itineraries}
          keyExtractor={(item) => item.id}
          contentContainerStyle={styles.list}
          renderItem={({ item }) => (
            <Pressable
              style={styles.card}
              onPress={() => navigation.navigate("ItineraryDetail", { itineraryId: item.id })}
            >
              <Text style={styles.cardTitle}>{item.title}</Text>
              <Text style={styles.cardMeta}>
                {formatDuration(item.totalMinutes)} · {t.itineraries.stopCount.replace("{count}", String(item.placeCount))}
              </Text>
            </Pressable>
          )}
        />
      )}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { flexDirection: "row", alignItems: "center", gap: 14, paddingHorizontal: 20, paddingTop: 8 },
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
  h1: { fontFamily: fonts.display, fontSize: 22, color: colors.ink },
  createBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    backgroundColor: colors.terracotta,
    borderRadius: radii.md,
    marginHorizontal: 20,
    marginTop: 18,
    paddingVertical: 14,
  },
  createBtnText: { fontFamily: fonts.bodyBold, fontSize: 15, color: colors.cream },
  loading: { marginTop: 40 },
  empty: { paddingHorizontal: 32, marginTop: 48, alignItems: "center", gap: 8 },
  emptyTitle: { fontFamily: fonts.bodySemiBold, fontSize: 16, color: colors.ink, textAlign: "center" },
  emptyBody: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, textAlign: "center" },
  list: { paddingHorizontal: 20, paddingTop: 18, paddingBottom: 32, gap: 12 },
  card: {
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    padding: 16,
  },
  cardTitle: { fontFamily: fonts.bodySemiBold, fontSize: 16, color: colors.ink, marginBottom: 4 },
  cardMeta: { fontFamily: fonts.body, fontSize: 13, color: colors.inkSoft },
});
