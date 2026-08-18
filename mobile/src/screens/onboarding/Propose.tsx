import React, { useEffect, useState } from "react";
import { View, Text, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Path, Polyline, Line } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { OnboardingStackParamList } from "../../navigation/types";
import { useLocale } from "../../i18n/LocaleContext";
import type { Locale } from "../../i18n/dictionary";
import { Dots } from "../../components/Dots";
import { CityCard } from "../../components/CityCard";
import { markOnboardingComplete } from "../../onboarding/onboardingStorage";
import {
  fetchCityManifest,
  formatApproxSize,
  planCityDownload,
  estimateDownloadSizeBytes,
  downloadCity,
  RIO_CITY_SLUG,
} from "../../data/downloadManager";
import { colors, fonts, spacing, radii } from "../../theme/tokens";

type Props = NativeStackScreenProps<OnboardingStackParamList, "Propose">;

const CITY_NAME = "Rio de Janeiro";

// Même ordre/étiquettes que le sélecteur par lieu de PlaceDetail — le
// téléchargement ne récupère jamais qu'une seule langue à la fois (voir
// downloadManager : "never requests a language the user did not select"),
// donc c'est ici, avant de lancer le téléchargement, qu'il faut choisir
// laquelle. Choisir une langue ici change la langue par défaut de toute
// l'app (setLocale global) — il n'y a qu'une seule notion de "langue" côté
// app, pas une langue de téléchargement distincte de la langue d'interface.
const LANG_ORDER: Locale[] = ["pt", "en", "fr", "es"];
const LANG_LABEL: Record<Locale, string> = { pt: "PT", en: "EN", fr: "FR", es: "ES" };

export function ProposeScreen({ navigation }: Props) {
  const { t, locale, setLocale } = useLocale();
  const [preview, setPreview] = useState<{ count: number; sizeLabel: string } | null>(null);
  const [downloading, setDownloading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setPreview(null); // le compte/la taille dépendent de la langue — jamais affiché pendant qu'on recharge pour une autre
    fetchCityManifest(RIO_CITY_SLUG, locale).then((places) => {
      if (cancelled) return;
      const files = planCityDownload(places, CITY_NAME, locale);
      setPreview({
        count: places.length,
        sizeLabel: formatApproxSize(estimateDownloadSizeBytes(files), locale),
      });
    });
    return () => {
      cancelled = true;
    };
  }, [locale]);

  async function goToApp() {
    await markOnboardingComplete();
    navigation.getParent()?.reset({ index: 0, routes: [{ name: "App" as never }] });
  }

  async function startDownload() {
    setDownloading(true);
    await downloadCity(RIO_CITY_SLUG, CITY_NAME, locale);
    setDownloading(false);
    navigation.navigate("DownloadSuccess");
  }

  const cityMeta = preview
    ? t.propose.cityMeta
        .replace("{count}", String(preview.count))
        .replace("{size}", preview.sizeLabel)
    : undefined;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.top}>
        <Text style={styles.eyebrow}>{t.propose.eyebrow}</Text>
        <Text style={styles.title}>{t.propose.title}</Text>
        <Text style={styles.lede}>{t.propose.lede}</Text>
        <View style={styles.iconCircle}>
          <Svg width={24} height={24} viewBox="0 0 24 24" fill="none">
            <Path
              d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"
              stroke={colors.terracotta}
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <Polyline points="7 10 12 15 17 10" stroke={colors.terracotta} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
            <Line x1={12} y1={15} x2={12} y2={3} stroke={colors.terracotta} strokeWidth={2} strokeLinecap="round" />
          </Svg>
        </View>
      </View>

      <View style={styles.langSection}>
        <Text style={styles.langLabel}>{t.propose.languageLabel}</Text>
        <View style={styles.langRow}>
          {LANG_ORDER.map((l) => (
            <Pressable
              key={l}
              onPress={() => setLocale(l)}
              style={[styles.pill, l === locale && styles.pillActive]}
            >
              <Text style={[styles.pillText, l === locale && styles.pillTextActive]}>
                {LANG_LABEL[l]}
              </Text>
            </Pressable>
          ))}
        </View>
      </View>

      {cityMeta ? (
        <CityCard city={CITY_NAME} meta={cityMeta} />
      ) : (
        <View style={styles.loadingRow}>
          <ActivityIndicator color={colors.terracotta} />
        </View>
      )}
      <Text style={styles.note}>{t.propose.note}</Text>

      <View style={styles.bottom}>
        <Dots total={3} activeIndex={1} />
        <Pressable style={styles.btn} onPress={startDownload} disabled={downloading}>
          {downloading ? (
            <ActivityIndicator color={colors.cream} />
          ) : (
            <Text style={styles.btnText}>{t.propose.download}</Text>
          )}
        </Pressable>
        <Pressable style={styles.ghostBtn} onPress={goToApp} disabled={downloading}>
          <Text style={styles.ghostBtnText}>{t.propose.skip}</Text>
        </Pressable>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream, justifyContent: "space-between" },
  top: { paddingHorizontal: spacing.xl, paddingTop: spacing.lg },
  eyebrow: {
    fontFamily: fonts.bodyBold,
    fontSize: 12,
    letterSpacing: 1.5,
    textTransform: "uppercase",
    color: colors.inkFaint,
    marginBottom: 16,
  },
  title: { fontFamily: fonts.display, fontSize: 30, lineHeight: 35, color: colors.ink, marginBottom: 14 },
  lede: { fontFamily: fonts.body, fontSize: 15, lineHeight: 24, color: colors.inkSoft, maxWidth: 300 },
  iconCircle: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.sand,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 30,
    marginBottom: 26,
  },
  langSection: { marginHorizontal: spacing.xl, marginBottom: 18 },
  langLabel: {
    fontFamily: fonts.bodyBold,
    fontSize: 12,
    letterSpacing: 1,
    textTransform: "uppercase",
    color: colors.inkFaint,
    marginBottom: 10,
  },
  langRow: { flexDirection: "row", gap: 8 },
  pill: { borderRadius: radii.pill, paddingVertical: 8, paddingHorizontal: 16, backgroundColor: colors.sand },
  pillActive: { backgroundColor: colors.terracotta },
  pillText: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.inkSoft },
  pillTextActive: { color: colors.cream },
  loadingRow: { marginHorizontal: spacing.xl, paddingVertical: 20, alignItems: "flex-start" },
  note: {
    fontFamily: fonts.body,
    fontSize: 12.5,
    lineHeight: 18,
    color: colors.inkFaint,
    marginHorizontal: spacing.xl,
    marginTop: 16,
  },
  bottom: { paddingHorizontal: spacing.xl, paddingBottom: spacing.lg, marginTop: "auto" },
  btn: { backgroundColor: colors.terracotta, borderRadius: radii.sm, paddingVertical: 16, alignItems: "center" },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
  ghostBtn: { paddingVertical: 14, alignItems: "center" },
  ghostBtnText: { fontFamily: fonts.bodySemiBold, fontSize: 14.5, color: colors.inkSoft },
});
