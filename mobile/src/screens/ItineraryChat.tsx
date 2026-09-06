import React, { useState } from "react";
import { View, Text, Pressable, TextInput, ScrollView, StyleSheet, ActivityIndicator, KeyboardAvoidingView, Platform } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { createItinerary, type Itinerary } from "../data/ItinerariesRepository";
import { formatDuration } from "../utils/itineraryFormat";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ItineraryChat">;

// result is never actually null here -- a failed generation never gets
// pushed into `turns` at all (see the catch block in handleSend), so this
// type stays non-optional rather than carrying a dead null-handling branch.
type Turn = { question: string; result: Itinerary };

export function ItineraryChatScreen({ navigation }: Props) {
  const { t } = useLocale();
  const { token } = useAuth();
  const [turns, setTurns] = useState<Turn[]>([]);
  const [pendingQuestion, setPendingQuestion] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function handleSend() {
    const question = input.trim();
    if (!question || !token || pendingQuestion) return;
    setInput("");
    setError(null);
    setPendingQuestion(question);
    try {
      const result = await createItinerary(token, question);
      setTurns((prev) => [...prev, { question, result }]);
    } catch {
      setInput(question);
      setError(t.itineraries.chatSendError);
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

        {!token ? (
          <View style={styles.empty}>
            <Text style={styles.emptyBody}>{t.itineraries.pleaseLogIn}</Text>
          </View>
        ) : (
          <>
            <View style={styles.head}>
              <Text style={styles.title}>{t.itineraries.chatTitle}</Text>
              <Text style={styles.subtitle}>{t.itineraries.chatSubtitle}</Text>
            </View>

            <ScrollView style={styles.chat} contentContainerStyle={styles.chatContent}>
              {turns.map((turn, i) => {
                let placeNumber = 0;
                return (
                  <View key={i}>
                    <View style={styles.rowUser}>
                      <View style={styles.bubbleUser}>
                        <Text style={styles.bubbleUserText}>{turn.question}</Text>
                      </View>
                    </View>
                    <View style={styles.rowAi}>
                      <Pressable
                        style={styles.itineraryCard}
                        onPress={() => navigation.navigate("ItineraryDetail", { itineraryId: turn.result.id })}
                      >
                        <Text style={styles.itineraryCardTitle}>{turn.result.title}</Text>
                        <Text style={styles.itineraryCardMeta}>
                          {formatDuration(turn.result.totalMinutes)} · {t.itineraries.stopCount.replace("{count}", String(turn.result.placeCount))}
                        </Text>
                        {turn.result.stops.map((stop, si) => {
                          const isSuggestion = stop.kind === "suggestion";
                          if (!isSuggestion) placeNumber += 1;
                          return (
                            <Text key={si} style={isSuggestion ? styles.stopLineSuggestion : styles.stopLine}>
                              {isSuggestion ? stop.label : `${placeNumber}. ${stop.label}`}
                              {isSuggestion ? ` (${t.itineraries.suggestionLabel})` : ""}
                            </Text>
                          );
                        })}
                        <Text style={styles.viewFullLink}>{t.itineraries.viewFullItinerary}</Text>
                      </Pressable>
                    </View>
                  </View>
                );
              })}
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
                placeholder={t.itineraries.chatInputPlaceholder}
                placeholderTextColor={colors.inkFaint}
                onSubmitEditing={handleSend}
                returnKeyType="send"
              />
              <Pressable
                style={[styles.sendBtn, (!input.trim() || !!pendingQuestion) && styles.sendBtnDisabled]}
                disabled={!input.trim() || !!pendingQuestion}
                onPress={handleSend}
              >
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
  empty: { paddingHorizontal: 32, marginTop: 48, alignItems: "center" },
  emptyBody: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, textAlign: "center" },
  head: { paddingHorizontal: 22, paddingTop: 16 },
  title: { fontFamily: fonts.display, fontSize: 22, color: colors.ink, marginBottom: 6 },
  subtitle: { fontFamily: fonts.body, fontSize: 13.5, lineHeight: 19, color: colors.inkSoft },
  chat: { flex: 1, paddingHorizontal: 20, paddingTop: 18 },
  chatContent: { gap: 14, paddingBottom: 8 },
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
  itineraryCard: {
    maxWidth: "90%",
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: 16,
    borderBottomLeftRadius: 4,
    padding: 15,
    gap: 6,
  },
  itineraryCardTitle: { fontFamily: fonts.bodySemiBold, fontSize: 15.5, color: colors.ink },
  itineraryCardMeta: { fontFamily: fonts.body, fontSize: 12.5, color: colors.inkSoft, marginBottom: 4 },
  stopLine: { fontFamily: fonts.body, fontSize: 13.5, color: colors.ink },
  stopLineSuggestion: { fontFamily: fonts.body, fontSize: 13.5, color: colors.inkSoft, fontStyle: "italic" },
  viewFullLink: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.terracotta, marginTop: 6 },
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
});
