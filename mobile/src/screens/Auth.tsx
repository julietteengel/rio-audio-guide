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

type Props = NativeStackScreenProps<AppStackParamList, "Auth">;

type Mode = "login" | "register";

export function AuthScreen({ navigation }: Props) {
  const { t } = useLocale();
  const { login, register } = useAuth();
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function submit() {
    setError(null);
    setSubmitting(true);
    try {
      if (mode === "login") {
        await login(email.trim(), password);
      } else {
        await register(email.trim(), password);
      }
      navigation.goBack();
    } catch (err) {
      // /login always returns the same generic error whether the email or
      // the password was wrong (see application.ErrInvalidCredentials on the
      // backend) — the UI shows that same generic message rather than
      // guessing which field was the problem.
      setError(
        err instanceof AuthApiError ? t.auth.genericError : t.auth.networkError,
      );
    } finally {
      setSubmitting(false);
    }
  }

  const canSubmit = email.trim().length > 0 && password.length > 0 && !submitting;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      <Text style={styles.h1}>{mode === "login" ? t.auth.loginTitle : t.auth.registerTitle}</Text>

      <View style={styles.form}>
        <View style={styles.field}>
          <Text style={styles.label}>{t.auth.emailLabel}</Text>
          <TextInput
            style={styles.input}
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="email-address"
            placeholder="vous@exemple.com"
            placeholderTextColor={colors.inkFaint}
          />
        </View>
        <View style={styles.field}>
          <Text style={styles.label}>{t.auth.passwordLabel}</Text>
          <TextInput
            style={styles.input}
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            autoCapitalize="none"
            placeholder="••••••••"
            placeholderTextColor={colors.inkFaint}
          />
        </View>

        {error ? <Text style={styles.error}>{error}</Text> : null}

        <Pressable style={[styles.btn, !canSubmit && styles.btnDisabled]} onPress={submit} disabled={!canSubmit}>
          {submitting ? (
            <ActivityIndicator color={colors.cream} />
          ) : (
            <Text style={styles.btnText}>{mode === "login" ? t.auth.loginCta : t.auth.registerCta}</Text>
          )}
        </Pressable>

        <Pressable
          style={styles.switchLink}
          onPress={() => {
            setError(null);
            setMode((m) => (m === "login" ? "register" : "login"));
          }}
        >
          <Text style={styles.switchLinkText}>
            {mode === "login" ? t.auth.switchToRegister : t.auth.switchToLogin}
          </Text>
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
  h1: { fontFamily: fonts.bodyBold, fontSize: 28, color: colors.ink, marginHorizontal: 28, marginTop: 20, marginBottom: 28 },
  form: { marginHorizontal: spacing.xl },
  field: { marginBottom: spacing.md },
  label: { fontFamily: fonts.bodySemiBold, fontSize: 13, color: colors.inkSoft, marginBottom: 6 },
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
  },
  error: { fontFamily: fonts.body, fontSize: 13, color: colors.terracottaDark, marginBottom: spacing.sm },
  btn: {
    backgroundColor: colors.terracotta,
    borderRadius: radii.sm,
    paddingVertical: 16,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  btnDisabled: { opacity: 0.5 },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
  switchLink: { paddingVertical: 16, alignItems: "center" },
  switchLinkText: { fontFamily: fonts.bodySemiBold, fontSize: 14, color: colors.terracotta },
});
