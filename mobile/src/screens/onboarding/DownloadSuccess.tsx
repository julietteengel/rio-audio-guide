import React from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { OnboardingStackParamList } from "../../navigation/types";
import { useLocale } from "../../i18n/LocaleContext";
import { Dots } from "../../components/Dots";
import { CityCard } from "../../components/CityCard";
import { markOnboardingComplete } from "../../onboarding/onboardingStorage";
import { colors, fonts, spacing, radii } from "../../theme/tokens";

type Props = NativeStackScreenProps<OnboardingStackParamList, "DownloadSuccess">;

export function DownloadSuccessScreen({ navigation }: Props) {
  const { t } = useLocale();

  async function goToApp() {
    await markOnboardingComplete();
    navigation.getParent()?.reset({ index: 0, routes: [{ name: "App" as never }] });
  }

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.top}>
        <Text style={styles.eyebrow}>{t.downloadSuccess.eyebrow}</Text>
        <Text style={styles.title}>{t.downloadSuccess.title}</Text>
        <Text style={styles.lede}>{t.downloadSuccess.lede}</Text>
        <View style={styles.checkCircle}>
          <Svg width={26} height={26} viewBox="0 0 24 24" fill="none">
            <Polyline points="5 13 10 18 19 7" stroke={colors.cream} strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </View>
      </View>

      <CityCard city="Rio de Janeiro" meta={t.downloadSuccess.cityMeta} badgeLabel={t.downloadSuccess.badge} />
      <Text style={styles.hint}>{t.downloadSuccess.hint}</Text>

      <View style={styles.bottom}>
        <Dots total={3} activeIndex={2} />
        <Pressable style={styles.btn} onPress={goToApp}>
          <Text style={styles.btnText}>{t.downloadSuccess.cta}</Text>
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
  title: { fontFamily: fonts.display, fontSize: 30, lineHeight: 35, color: colors.ink, marginBottom: 12 },
  lede: { fontFamily: fonts.body, fontSize: 15, lineHeight: 24, color: colors.inkSoft, maxWidth: 300 },
  checkCircle: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 32,
    marginBottom: 28,
  },
  hint: {
    fontFamily: fonts.body,
    fontSize: 12.5,
    lineHeight: 18,
    color: colors.inkFaint,
    textAlign: "center",
    marginHorizontal: spacing.xl,
    marginTop: 18,
  },
  bottom: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xl, marginTop: "auto" },
  btn: { backgroundColor: colors.terracotta, borderRadius: radii.sm, paddingVertical: 16, alignItems: "center" },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
});
