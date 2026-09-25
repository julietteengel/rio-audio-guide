// mobile/src/screens/ForgotPassword.tsx
import React, { useState } from "react";
import { View, Text, TextInput, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { colors, fonts, spacing, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ForgotPassword">;

export function ForgotPasswordScreen({ navigation }: Props) {
  const { t, locale } = useLocale();
  const { forgotPassword } = useAuth();
  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit() {
    setSubmitting(true);
    try {
      // Always "succeeds" from this screen's point of view -- the backend
      // never reveals whether the email belongs to a real account
      // (anti-enumeration), so there is nothing to branch on here.
      await forgotPassword(email.trim(), locale);
    } catch {
      // A network failure still moves on to ResetPassword: the "resend"
      // link there covers retrying the actual send.
    } finally {
      setSubmitting(false);
      navigation.navigate("ResetPassword", { email: email.trim() });
    }
  }

  const canSubmit = email.trim().length > 0 && !submitting;

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
        <Text style={styles.title}>{t.forgotPassword.title}</Text>
        <Text style={styles.subtitle}>{t.forgotPassword.subtitle}</Text>

        <TextInput
          style={styles.input}
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="email-address"
          placeholder="vous@exemple.com"
          placeholderTextColor={colors.inkFaint}
          editable={!submitting}
        />

        <Pressable style={[styles.btn, !canSubmit && styles.btnDisabled]} onPress={submit} disabled={!canSubmit}>
          {submitting ? <ActivityIndicator color={colors.cream} /> : <Text style={styles.btnText}>{t.forgotPassword.submitCta}</Text>}
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
  btn: {
    backgroundColor: colors.terracotta,
    borderRadius: radii.sm,
    paddingVertical: 16,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  btnDisabled: { opacity: 0.5 },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
});
