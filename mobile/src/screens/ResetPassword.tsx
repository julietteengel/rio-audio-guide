// mobile/src/screens/ResetPassword.tsx
import React, { useState } from "react";
import { View, Text, TextInput, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { AuthApiError } from "../data/AuthRepository";
import { colors, fonts, spacing, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ResetPassword">;

export function ResetPasswordScreen({ route, navigation }: Props) {
  const { email } = route.params;
  const { t, locale } = useLocale();
  const { resetPassword, forgotPassword, login } = useAuth();
  const [code, setCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const [resendMessage, setResendMessage] = useState<string | null>(null);

  async function submit() {
    setError(null);
    setSubmitting(true);
    try {
      await resetPassword(email, code.trim(), newPassword);
      // Complete the login with the password the user just set, same
      // "finish what they came here to do" pattern VerifyEmail.tsx uses.
      await login(email, newPassword);
      // ResetPassword -> ForgotPassword -> Auth is 3 screens deep on
      // AppNavigator's own stack (all three are plain sibling screens
      // there, not nested navigators -- see VerifyEmail.tsx's comment on
      // the same stack for why getParent() would be wrong here too), so
      // popping 3 lands back on whatever screen originally opened Auth.
      navigation.pop(3);
    } catch (err) {
      setError(err instanceof AuthApiError ? t.resetPassword.invalidCode : t.auth.networkError);
    } finally {
      setSubmitting(false);
    }
  }

  async function resend() {
    setResendMessage(null);
    setResending(true);
    try {
      await forgotPassword(email, locale);
      setResendMessage(t.resetPassword.resendSent);
    } catch {
      setError(t.auth.networkError);
    } finally {
      setResending(false);
    }
  }

  const canSubmit = code.trim().length > 0 && newPassword.length > 0 && !submitting && !resending;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      <View style={styles.form}>
        <Text style={styles.title}>{t.resetPassword.title}</Text>
        <Text style={styles.subtitle}>{t.resetPassword.subtitle.replace("{email}", email)}</Text>

        <TextInput
          style={styles.codeInput}
          value={code}
          onChangeText={setCode}
          keyboardType="number-pad"
          maxLength={6}
          placeholder={t.resetPassword.codePlaceholder}
          placeholderTextColor={colors.inkFaint}
          editable={!submitting && !resending}
        />

        <TextInput
          style={styles.input}
          value={newPassword}
          onChangeText={setNewPassword}
          secureTextEntry
          autoCapitalize="none"
          placeholder={t.resetPassword.newPasswordPlaceholder}
          placeholderTextColor={colors.inkFaint}
          editable={!submitting && !resending}
        />

        {error ? <Text style={styles.error}>{error}</Text> : null}
        {resendMessage ? <Text style={styles.resendMessage}>{resendMessage}</Text> : null}

        <Pressable style={[styles.btn, !canSubmit && styles.btnDisabled]} onPress={submit} disabled={!canSubmit}>
          {submitting ? <ActivityIndicator color={colors.cream} /> : <Text style={styles.btnText}>{t.resetPassword.submitCta}</Text>}
        </Pressable>

        <Pressable style={styles.resendLinkWrap} onPress={resend} disabled={resending || submitting}>
          <Text style={styles.resendLinkText}>{t.resetPassword.resendLink}</Text>
        </Pressable>
      </View>
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
  form: { marginHorizontal: spacing.xl, marginTop: spacing.xl },
  title: { fontFamily: fonts.display, fontSize: 26, color: colors.ink, marginBottom: 10 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, marginBottom: spacing.lg },
  codeInput: {
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
  input: {
    fontFamily: fonts.body,
    fontSize: 15,
    color: colors.ink,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 12,
    paddingHorizontal: 14,
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
