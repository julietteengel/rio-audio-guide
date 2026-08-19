import React from "react";
import { View, StyleSheet } from "react-native";
import { colors } from "../theme/tokens";

export function Dots({
  total,
  activeIndex,
  activeColor = colors.terracotta,
  inactiveColor = colors.line,
}: {
  total: number;
  activeIndex: number;
  activeColor?: string;
  inactiveColor?: string;
}) {
  return (
    <View style={styles.row}>
      {Array.from({ length: total }).map((_, i) => (
        <View
          key={i}
          style={[
            styles.dot,
            {
              backgroundColor: i === activeIndex ? activeColor : inactiveColor,
              width: i === activeIndex ? 16 : 6,
              borderRadius: i === activeIndex ? 3 : 3,
            },
          ]}
        />
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", justifyContent: "center", gap: 6, marginBottom: 20 },
  dot: { height: 6 },
});
