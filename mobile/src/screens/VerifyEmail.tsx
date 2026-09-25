// mobile/src/screens/VerifyEmail.tsx
import React, { useEffect, useState } from "react";
import { View, Text, TextInput, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { AuthApiError } from "../data/AuthRepository";
import { colors, fonts, spacing, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "VerifyEmail">;

export function VerifyEmailScreen({ route, navigation }: Props) {
  const { email, password, codeAlreadySent } = route.params;
  const { t, locale } = useLocale();
  const { verifyEmail, resendVerificationCode, login } = useAuth();
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const [resendMessage, setResendMessage] = useState<string | null>(null);
  // codeAlreadySent is false when this screen was reached via /login's 403
  // (unverified account) rather than a fresh register -- there's no way to
  // know from here whether a code was ever actually sent to this address
  // (it may be a pre-existing account from before this feature existed), so
  // a fresh one is requested up front rather than telling the user to check
  // an inbox that may have nothing in it.
  const [initializing, setInitializing] = useState(!codeAlreadySent);

  useEffect(() => {
    if (codeAlreadySent) return;
    let cancelled = false;
    resendVerificationCode(email, locale)
      .catch(() => {
        if (!cancelled) setError(t.auth.networkError);
      })
      .finally(() => {
        if (!cancelled) setInitializing(false);
      });
    return () => {
      cancelled = true;
    };
  }, [codeAlreadySent, email, locale, resendVerificationCode, t]);

  async function submit() {
    setError(null);
    setSubmitting(true);
    try {
      await verifyEmail(email, code.trim());
      // Verification succeeded -- complete the login the user originally
      // started with (password was carried through via route params
      // specifically so they don't have to type it twice).
      await login(email, password);
      // VerifyEmail and Auth are both plain screens on AppNavigator's own
      // stack (siblings of Map/Settings/etc, not nested navigators), so
      // popping 2 dismisses both and lands back on whatever screen opened
      // Auth -- getParent() would instead walk up to RootNavigator, whose
      // stack has nothing to pop once onboarding is done, and silently no-op.
      navigation.pop(2);
    } catch (err) {
      setError(err instanceof AuthApiError ? t.verifyEmail.invalidCode : t.auth.networkError);
    } finally {
      setSubmitting(false);
    }
  }

  async function resend() {
    setResendMessage(null);
    setResending(true);
    try {
      await resendVerificationCode(email, locale);
      setResendMessage(t.verifyEmail.resendSent);
    } catch {
      setError(t.auth.networkError);
    } finally {
      setResending(false);
    }
  }

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      {initializing ? (
        <View style={styles.initializing}>
          <ActivityIndicator color={colors.terracotta} />
        </View>
      ) : (
        <View style={styles.form}>
          <Text style={styles.title}>{t.verifyEmail.title}</Text>
          <Text style={styles.subtitle}>{t.verifyEmail.subtitle.replace("{email}", email)}</Text>

          <TextInput
            style={styles.input}
            value={code}
            onChangeText={setCode}
            keyboardType="number-pad"
            maxLength={6}
            placeholder={t.verifyEmail.codePlaceholder}
            placeholderTextColor={colors.inkFaint}
            editable={!submitting && !resending}
          />

          {error ? <Text style={styles.error}>{error}</Text> : null}
          {resendMessage ? <Text style={styles.resendMessage}>{resendMessage}</Text> : null}

          <Pressable
            style={[styles.btn, (!code.trim() || submitting || resending) && styles.btnDisabled]}
            onPress={submit}
            disabled={!code.trim() || submitting || resending}
          >
            {submitting ? <ActivityIndicator color={colors.cream} /> : <Text style={styles.btnText}>{t.verifyEmail.submitCta}</Text>}
          </Pressable>

          <Pressable style={styles.resendLinkWrap} onPress={resend} disabled={resending || submitting}>
            <Text style={styles.resendLinkText}>{t.verifyEmail.resendLink}</Text>
          </Pressable>
        </View>
      )}
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
  initializing: { flex: 1, alignItems: "center", justifyContent: "center" },
  form: { marginHorizontal: spacing.xl, marginTop: spacing.xl },
  title: { fontFamily: fonts.display, fontSize: 26, color: colors.ink, marginBottom: 10 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, marginBottom: spacing.lg },
  input: {
    fontFamily: fonts.body,
    fontSize: 20,
    letterSpacing: 4,
    textAlign: "center",
    color: colors.ink,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 14,
    marginBottom: spacing.md,
  },
  error: { fontFamily: fonts.body, fontSize: 13, color: colors.terracottaDark, marginBottom: spacing.sm },
  resendMessage: { fontFamily: fonts.body, fontSize: 13, color: colors.inkSoft, marginBottom: spacing.sm },
  btn: {
    backgroundColor: colors.terracotta,
    borderRadius: radii.sm,
    paddingVertical: 16,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  btnDisabled: { opacity: 0.5 },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
  resendLinkWrap: { paddingVertical: 16, alignItems: "center" },
  resendLinkText: { fontFamily: fonts.bodySemiBold, fontSize: 14, color: colors.terracotta },
});
