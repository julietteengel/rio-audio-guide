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

type Props = NativeStackScreenProps<AppStackParamList, "EditProfile">;

export function EditProfileScreen({ navigation }: Props) {
  const { t } = useLocale();
  const { user, updateProfile } = useAuth();
  const [email, setEmail] = useState(user?.email ?? "");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  async function save() {
    setError(null);
    setSaved(false);
    setSubmitting(true);
    try {
      const changes: { email?: string; password?: string } = {};
      if (email.trim() && email.trim() !== user?.email) changes.email = email.trim();
      if (password) changes.password = password;
      if (Object.keys(changes).length > 0) {
        await updateProfile(changes);
      }
      setPassword("");
      setSaved(true);
    } catch (err) {
      setError(err instanceof AuthApiError ? err.message : t.auth.networkError);
    } finally {
      setSubmitting(false);
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

      <Text style={styles.h1}>{t.editProfile.title}</Text>

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
            placeholderTextColor={colors.inkFaint}
          />
        </View>
        <View style={styles.field}>
          <Text style={styles.label}>{t.editProfile.newPasswordLabel}</Text>
          <TextInput
            style={styles.input}
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            autoCapitalize="none"
            placeholder={t.editProfile.newPasswordPlaceholder}
            placeholderTextColor={colors.inkFaint}
          />
        </View>

        {error ? <Text style={styles.error}>{error}</Text> : null}
        {saved && !error ? <Text style={styles.saved}>{t.editProfile.saved}</Text> : null}

        <Pressable style={styles.btn} onPress={save} disabled={submitting}>
          {submitting ? <ActivityIndicator color={colors.cream} /> : <Text style={styles.btnText}>{t.editProfile.save}</Text>}
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
  saved: { fontFamily: fonts.body, fontSize: 13, color: colors.badgeDot, marginBottom: spacing.sm },
  btn: {
    backgroundColor: colors.terracotta,
    borderRadius: radii.sm,
    paddingVertical: 16,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  btnText: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.cream },
});
