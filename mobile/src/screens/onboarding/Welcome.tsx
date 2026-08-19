import React from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { LinearGradient } from "expo-linear-gradient";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { OnboardingStackParamList } from "../../navigation/types";
import { useLocale } from "../../i18n/LocaleContext";
import { Dots } from "../../components/Dots";
import { colors, fonts, spacing, radii } from "../../theme/tokens";

type Props = NativeStackScreenProps<OnboardingStackParamList, "Welcome">;

export function WelcomeScreen({ navigation }: Props) {
  const { t } = useLocale();

  return (
    <LinearGradient
      colors={["#7C3B20", "#B9552B", "#D2793F"]}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={styles.screen}
    >
      <SafeAreaView style={styles.safe}>
        <View style={styles.top}>
          <Text style={styles.eyebrow}>{t.welcome.eyebrow}</Text>
          <Text style={styles.title}>{t.welcome.title}</Text>
          <Text style={styles.gloss}>
            <Text style={styles.glossWord}>{t.welcome.glossWord}</Text>
            {t.welcome.glossRest}
          </Text>
          <Text style={styles.lede}>{t.welcome.lede}</Text>
        </View>

        <View style={styles.bottom}>
          <Dots total={3} activeIndex={0} activeColor={colors.cream} inactiveColor="rgba(250,245,238,0.35)" />
          <Pressable style={styles.btn} onPress={() => navigation.navigate("Propose")}>
            <Text style={styles.btnText}>{t.welcome.cta}</Text>
          </Pressable>
        </View>
      </SafeAreaView>
    </LinearGradient>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  safe: { flex: 1, justifyContent: "space-between" },
  top: { paddingHorizontal: spacing.xl, paddingTop: spacing.xl },
  eyebrow: {
    fontFamily: fonts.bodyBold,
    fontSize: 12,
    letterSpacing: 1.5,
    textTransform: "uppercase",
    color: "rgba(250,245,238,0.8)",
    marginBottom: 18,
  },
  title: { fontFamily: fonts.displayBlack, fontSize: 44, lineHeight: 48, color: colors.cream },
  gloss: { fontFamily: fonts.body, fontSize: 15, color: "rgba(250,245,238,0.78)", marginTop: 16 },
  glossWord: { fontFamily: fonts.bodyBold, fontStyle: "italic", color: colors.cream },
  lede: {
    fontFamily: fonts.body,
    fontSize: 16,
    lineHeight: 24,
    color: "rgba(250,245,238,0.92)",
    marginTop: 22,
    maxWidth: 280,
  },
  bottom: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xl },
  btn: {
    backgroundColor: colors.cream,
    borderRadius: radii.sm,
    paddingVertical: 16,
    alignItems: "center",
  },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.ink },
});
