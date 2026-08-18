import React from "react";
import { View, Text, Pressable, ScrollView, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { SUPPORTED_LOCALES, Locale } from "../i18n/dictionary";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "Settings">;

export function SettingsScreen({ navigation }: Props) {
  const { t, locale, setLocale } = useLocale();

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>
      <Text style={styles.h1}>{t.settings.title}</Text>

      <ScrollView contentContainerStyle={{ paddingBottom: 40 }}>
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>{t.settings.offlineDataSection}</Text>
          <View style={styles.group}>
            <View style={styles.row}>
              <View style={{ flex: 1 }}>
                <Text style={styles.rowLabel}>Rio de Janeiro</Text>
                <Text style={styles.rowSub}>42 lieux · 184 Mo</Text>
              </View>
              {/* No real persisted download exists yet (mock data layer only,
                  see the final report) — this is a visual affordance, not wired
                  to a real delete operation. */}
              <Pressable onPress={() => {}}>
                <Text style={styles.link}>{t.settings.delete}</Text>
              </Pressable>
            </View>
          </View>
        </View>

        <View style={styles.section}>
          <Text style={styles.sectionLabel}>{t.settings.languageSection}</Text>
          <View style={styles.group}>
            {SUPPORTED_LOCALES.map((l: Locale, i) => (
              <Pressable
                key={l}
                style={[styles.row, i === SUPPORTED_LOCALES.length - 1 && styles.rowLast]}
                onPress={() => setLocale(l)}
              >
                <Text style={styles.rowLabel}>{t.settings.languageNames[l]}</Text>
                {l === locale ? (
                  <View style={styles.check}>
                    <Svg width={11} height={11} viewBox="0 0 24 24" fill="none">
                      <Polyline points="5 13 10 18 19 7" stroke={colors.cream} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
                    </Svg>
                  </View>
                ) : null}
              </Pressable>
            ))}
          </View>
        </View>

        <View style={styles.section}>
          <Text style={styles.sectionLabel}>{t.settings.aboutSection}</Text>
          <View style={styles.group}>
            <View style={styles.row}>
              <Text style={styles.rowLabel}>{t.settings.version}</Text>
              <Text style={styles.rowValue}>1.0.0</Text>
            </View>
            <View style={styles.row}>
              <Text style={styles.rowLabel}>{t.settings.privacy}</Text>
              <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
                <Polyline points="9 6 15 12 9 18" stroke={colors.inkFaint} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
              </Svg>
            </View>
            <View style={[styles.row, styles.rowLast]}>
              <Text style={styles.rowLabel}>{t.settings.terms}</Text>
              <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
                <Polyline points="9 6 15 12 9 18" stroke={colors.inkFaint} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
              </Svg>
            </View>
          </View>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  h1: { fontFamily: fonts.bodyBold, fontSize: 28, color: colors.ink, marginHorizontal: 28, marginTop: 20, marginBottom: 28 },
  section: { marginBottom: 28 },
  sectionLabel: {
    fontFamily: fonts.bodyBold,
    fontSize: 12,
    letterSpacing: 1,
    textTransform: "uppercase",
    color: colors.inkFaint,
    marginHorizontal: 28,
    marginBottom: 10,
  },
  group: {
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    marginHorizontal: 20,
    overflow: "hidden",
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    paddingVertical: 15,
    paddingHorizontal: 16,
    borderBottomWidth: 1,
    borderBottomColor: colors.line,
  },
  rowLast: { borderBottomWidth: 0 },
  rowLabel: { flex: 1, fontFamily: fonts.bodySemiBold, fontSize: 15, color: colors.ink },
  rowSub: { fontFamily: fonts.body, fontSize: 12.5, color: colors.inkSoft, marginTop: 2 },
  rowValue: { fontFamily: fonts.body, fontSize: 14, color: colors.inkSoft },
  link: { fontFamily: fonts.bodyBold, fontSize: 14, color: colors.terracotta },
  check: {
    width: 20,
    height: 20,
    borderRadius: 10,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
});
