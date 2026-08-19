import React from "react";
import { View, Text, StyleSheet } from "react-native";
import Svg, { Path, Circle } from "react-native-svg";
import { colors, fonts, radii, spacing } from "../theme/tokens";

export function CityCard({
  city,
  meta,
  badgeLabel,
}: {
  city: string;
  meta: string;
  badgeLabel?: string;
}) {
  return (
    <View style={styles.card}>
      <View style={styles.pin}>
        <Svg width={20} height={20} viewBox="0 0 24 24" fill="none">
          <Path
            d="M12 21s-7-5.686-7-11a7 7 0 0 1 14 0c0 5.314-7 11-7 11z"
            stroke={colors.terracotta}
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
          <Circle cx={12} cy={10} r={2.5} stroke={colors.terracotta} strokeWidth={2} />
        </Svg>
      </View>
      <View style={styles.textCol}>
        <Text style={styles.city}>{city}</Text>
        <Text style={styles.meta}>{meta}</Text>
      </View>
      {badgeLabel ? (
        <View style={styles.badge}>
          <Text style={styles.badgeText}>{badgeLabel}</Text>
        </View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.md,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    padding: spacing.lg - 2,
    marginHorizontal: spacing.xl,
  },
  pin: {
    width: 40,
    height: 40,
    borderRadius: radii.sm + 2,
    backgroundColor: colors.sand,
    alignItems: "center",
    justifyContent: "center",
  },
  textCol: { flex: 1 },
  city: { fontFamily: fonts.bodyBold, fontSize: 16, color: colors.ink, marginBottom: 3 },
  meta: { fontFamily: fonts.body, fontSize: 13, color: colors.inkSoft },
  badge: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: colors.sand,
    borderRadius: radii.pill,
    paddingVertical: 5,
    paddingHorizontal: 10,
  },
  badgeText: { fontFamily: fonts.bodyBold, fontSize: 12, color: colors.terracottaDark },
});
