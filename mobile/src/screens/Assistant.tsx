import React from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline, Circle, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "Assistant">;

export function AssistantScreen({ navigation }: Props) {
  const { t } = useLocale();

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
        <View style={styles.roadmap}>
          <Svg width={12} height={12} viewBox="0 0 24 24" fill="none">
            <Circle cx={12} cy={12} r={9} stroke={colors.roadmapText} strokeWidth={2.4} />
            <Polyline points="12 7 12 12 15.5 14" stroke={colors.roadmapText} strokeWidth={2.4} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
          <Text style={styles.roadmapText}>{t.assistant.roadmapBadge}</Text>
        </View>
      </View>

      <View style={styles.head}>
        <Text style={styles.title}>{t.assistant.title}</Text>
        <Text style={styles.subtitle}>{t.assistant.subtitle}</Text>
      </View>

      <View style={styles.chat}>
        <View style={styles.rowUser}>
          <View style={styles.bubbleUser}>
            <Text style={styles.bubbleUserText}>{t.assistant.exampleQuestion}</Text>
          </View>
        </View>
        <View style={styles.rowAi}>
          <View style={styles.bubbleAi}>
            <Text style={styles.bubbleAiText}>{t.assistant.exampleAnswer}</Text>
            <View style={styles.sourceChip}>
              <Svg width={11} height={11} viewBox="0 0 24 24" fill="none">
                <Path
                  d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"
                  stroke={colors.inkSoft}
                  strokeWidth={2.2}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
                <Polyline points="14 2 14 8 20 8" stroke={colors.inkSoft} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
              </Svg>
              <Text style={styles.sourceChipText}>{t.assistant.exampleSource}</Text>
            </View>
          </View>
        </View>
      </View>

      <View style={styles.inputBar} pointerEvents="none">
        <Text style={styles.inputPlaceholder}>{t.assistant.inputPlaceholder}</Text>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { flexDirection: "row", alignItems: "center", gap: 12, paddingHorizontal: 20, paddingTop: 8 },
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
  roadmap: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    backgroundColor: colors.roadmapBg,
    borderRadius: radii.pill,
    paddingVertical: 6,
    paddingHorizontal: 11,
  },
  roadmapText: { fontFamily: fonts.bodyBold, fontSize: 11.5, color: colors.roadmapText },
  head: { paddingHorizontal: 22, paddingTop: 20 },
  title: { fontFamily: fonts.display, fontSize: 24, color: colors.ink, marginBottom: 8 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, maxWidth: 320 },
  chat: { flex: 1, paddingHorizontal: 20, paddingTop: 22, gap: 14 },
  rowUser: { flexDirection: "row", justifyContent: "flex-end" },
  bubbleUser: {
    maxWidth: "78%",
    backgroundColor: colors.terracotta,
    borderRadius: 16,
    borderBottomRightRadius: 4,
    paddingVertical: 12,
    paddingHorizontal: 15,
  },
  bubbleUserText: { fontFamily: fonts.body, fontSize: 14.5, lineHeight: 21, color: colors.cream },
  rowAi: { flexDirection: "row", justifyContent: "flex-start" },
  bubbleAi: {
    maxWidth: "84%",
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: 16,
    borderBottomLeftRadius: 4,
    paddingVertical: 12,
    paddingHorizontal: 15,
  },
  bubbleAiText: { fontFamily: fonts.body, fontSize: 14.5, lineHeight: 21, color: colors.ink },
  sourceChip: {
    flexDirection: "row",
    alignItems: "center",
    gap: 5,
    marginTop: 10,
    backgroundColor: colors.sand,
    borderRadius: radii.pill,
    paddingVertical: 5,
    paddingHorizontal: 10,
    alignSelf: "flex-start",
  },
  sourceChipText: { fontFamily: fonts.bodyBold, fontSize: 11.5, color: colors.inkSoft },
  inputBar: {
    margin: 20,
    marginTop: 16,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 13,
    paddingHorizontal: 16,
    opacity: 0.55,
  },
  inputPlaceholder: { fontFamily: fonts.body, fontSize: 14.5, color: colors.inkFaint },
});
