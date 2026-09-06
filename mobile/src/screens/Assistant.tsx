// mobile/src/screens/Assistant.tsx
import React, { useEffect, useState } from "react";
import {
  View,
  Text,
  Pressable,
  TextInput,
  StyleSheet,
  ScrollView,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { placesRepository } from "../data/PlacesRepository";
import { askAssistant, AssistantApiError, type ConversationTurn, type GroundingLevel } from "../data/AssistantRepository";
import { colors, fonts, radii } from "../theme/tokens";
import type { Dictionary } from "../i18n/dictionary";

type Props = NativeStackScreenProps<AppStackParamList, "Assistant">;

type Turn = { question: string; answer: string; groundingLevel: GroundingLevel };

function GroundingBadge({ level, t }: { level: GroundingLevel; t: Dictionary }) {
  const bg = level === "grounded" ? colors.groundBg : level === "mixed" ? colors.roadmapBg : colors.roadmapText;
  const text = level === "grounded" ? colors.groundText : level === "mixed" ? colors.roadmapText : colors.cream;
  const label = level === "grounded" ? t.assistant.groundedBadge : level === "mixed" ? t.assistant.mixedBadge : t.assistant.generalBadge;
  return (
    <View style={[styles.sourceChip, { backgroundColor: bg }]}>
      {level === "grounded" ? (
        <Svg width={11} height={11} viewBox="0 0 24 24" fill="none">
          <Polyline points="5 13 10 18 19 7" stroke={text} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
        </Svg>
      ) : (
        <Svg width={11} height={11} viewBox="0 0 24 24" fill="none">
          <Path d="M12 9v4M12 17h.01" stroke={text} strokeWidth={2.4} strokeLinecap="round" />
          <Path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" stroke={text} strokeWidth={1.8} strokeLinejoin="round" />
        </Svg>
      )}
      <Text style={[styles.sourceChipText, { color: text }]}>{label}</Text>
    </View>
  );
}

export function AssistantScreen({ route, navigation }: Props) {
  const { t, locale } = useLocale();
  const { token } = useAuth();
  const [available, setAvailable] = useState<boolean | null>(null);
  const [turns, setTurns] = useState<Turn[]>([]);
  const [pendingQuestion, setPendingQuestion] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    placesRepository
      .getById(route.params.placeId)
      .then((place) => {
        if (!cancelled) setAvailable(place?.narrationStatus === "ready");
      })
      .catch(() => {
        // Fail closed, not open -- if we can't even confirm the place has
        // published narration, treat it the same as "not available" rather
        // than showing a chat that might immediately 404.
        if (!cancelled) setAvailable(false);
      });
    return () => {
      cancelled = true;
    };
  }, [route.params.placeId, locale]);

  async function handleSend() {
    const question = input.trim();
    if (!question || !token || pendingQuestion) return;
    setInput("");
    setError(null);
    setPendingQuestion(question);
    try {
      // Capped to the last 6 turns client-side too, matching the backend's
      // own history cap (internal/application/ask_assistant.go) -- no point
      // shipping an ever-growing request body for turns the server would
      // truncate away anyway.
      const history: ConversationTurn[] = turns.slice(-6).map(({ question, answer }) => ({ question, answer }));
      const result = await askAssistant(token, route.params.placeId, locale, question, history);
      setTurns((prev) => [...prev, { question, answer: result.answer, groundingLevel: result.groundingLevel }]);
    } catch (err) {
      if (err instanceof AssistantApiError && err.status === 404) {
        setAvailable(false);
      } else {
        // Put the question back in the input rather than silently losing it --
        // the user can retry without retyping it.
        setInput(question);
        setError(t.assistant.sendError);
      }
    } finally {
      setPendingQuestion(null);
    }
  }

  return (
    <KeyboardAvoidingView style={styles.screen} behavior={Platform.OS === "ios" ? "padding" : undefined}>
      <SafeAreaView style={styles.flexOne} edges={["top", "bottom"]}>
        <View style={styles.topbar}>
          <Pressable style={styles.back} onPress={() => navigation.goBack()}>
            <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
              <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
          </Pressable>
        </View>

        {available === false ? (
          <View style={styles.unavailable}>
            <Text style={styles.unavailableTitle}>{t.assistant.unavailableTitle}</Text>
            <Text style={styles.unavailableBody}>{t.assistant.unavailableBody}</Text>
          </View>
        ) : (
          <>
            <View style={styles.head}>
              <Text style={styles.title}>{t.assistant.title}</Text>
              <Text style={styles.subtitle}>{t.assistant.subtitle}</Text>
            </View>

            <ScrollView style={styles.chat} contentContainerStyle={styles.chatContent}>
              {turns.length === 0 && !pendingQuestion && <Text style={styles.invite}>{t.assistant.emptyStateInvite}</Text>}
              {turns.map((turn, i) => (
                <View key={i}>
                  <View style={styles.rowUser}>
                    <View style={styles.bubbleUser}>
                      <Text style={styles.bubbleUserText}>{turn.question}</Text>
                    </View>
                  </View>
                  <View style={styles.rowAi}>
                    <View style={styles.bubbleAi}>
                      <Text style={styles.bubbleAiText}>{turn.answer}</Text>
                      <GroundingBadge level={turn.groundingLevel} t={t} />
                    </View>
                  </View>
                </View>
              ))}
              {pendingQuestion && (
                <View>
                  <View style={styles.rowUser}>
                    <View style={styles.bubbleUser}>
                      <Text style={styles.bubbleUserText}>{pendingQuestion}</Text>
                    </View>
                  </View>
                  <View style={styles.rowAi}>
                    <View style={styles.bubbleAi}>
                      <ActivityIndicator color={colors.terracotta} />
                    </View>
                  </View>
                </View>
              )}
            </ScrollView>

            {error && <Text style={styles.errorText}>{error}</Text>}

            <View style={styles.inputBar}>
              <TextInput
                style={styles.input}
                value={input}
                onChangeText={setInput}
                placeholder={t.assistant.inputPlaceholder}
                placeholderTextColor={colors.inkFaint}
                onSubmitEditing={handleSend}
                returnKeyType="send"
              />
              <Pressable style={[styles.sendBtn, (!input.trim() || !token) && styles.sendBtnDisabled]} disabled={!input.trim() || !!pendingQuestion || !token} onPress={handleSend}>
                <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
                  <Path d="M22 2 11 13" stroke={colors.cream} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
                  <Path d="M22 2 15 22 11 13 2 9 22 2Z" stroke={colors.cream} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
                </Svg>
              </Pressable>
            </View>
          </>
        )}
      </SafeAreaView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  flexOne: { flex: 1 },
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
  head: { paddingHorizontal: 22, paddingTop: 20 },
  title: { fontFamily: fonts.display, fontSize: 24, color: colors.ink, marginBottom: 8 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, maxWidth: 320 },
  chat: { flex: 1, paddingHorizontal: 20, paddingTop: 22 },
  chatContent: { gap: 14, paddingBottom: 8 },
  invite: { fontFamily: fonts.body, fontSize: 14.5, color: colors.inkFaint, textAlign: "center", marginTop: 40 },
  rowUser: { flexDirection: "row", justifyContent: "flex-end", marginBottom: 8 },
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
    borderRadius: radii.pill,
    paddingVertical: 5,
    paddingHorizontal: 10,
    alignSelf: "flex-start",
  },
  sourceChipText: { fontFamily: fonts.bodyBold, fontSize: 11.5 },
  errorText: { fontFamily: fonts.body, fontSize: 12.5, color: colors.terracottaDark, textAlign: "center", paddingBottom: 6 },
  inputBar: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    margin: 20,
    marginTop: 12,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 8,
    paddingHorizontal: 8,
    paddingLeft: 16,
  },
  input: { flex: 1, fontFamily: fonts.body, fontSize: 14.5, color: colors.ink },
  sendBtn: {
    width: 38,
    height: 38,
    borderRadius: 19,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  sendBtnDisabled: { backgroundColor: "rgba(193,89,46,0.35)" },
  unavailable: { flex: 1, paddingHorizontal: 32, justifyContent: "center", alignItems: "center", gap: 10 },
  unavailableTitle: { fontFamily: fonts.display, fontSize: 20, color: colors.ink, textAlign: "center" },
  unavailableBody: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, textAlign: "center" },
});
