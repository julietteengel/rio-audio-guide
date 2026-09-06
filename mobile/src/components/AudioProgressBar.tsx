import React, { useRef, useState } from "react";
import { View, StyleSheet, PanResponder, type GestureResponderEvent } from "react-native";
import { colors } from "../theme/tokens";

type Props = {
  /** Current playback position as a fraction of duration, 0-1. Ignored
   * while the user is actively dragging (the drag position takes over the
   * display until release). */
  progress: number;
  /** Called once, with a 0-1 fraction, when the user releases a tap or drag. */
  onSeek: (fraction: number) => void;
};

const TRACK_HEIGHT = 4;
const THUMB_SIZE = 12;

export function AudioProgressBar({ progress, onSeek }: Props) {
  const trackRef = useRef<View>(null);
  // pageX/width in screen coordinates -- refreshed on layout and again on
  // every gesture grant, since PanResponder's gestureState.moveX is a
  // screen coordinate, not one relative to this view (nativeEvent.locationX
  // is ambiguous enough across RN versions that this measures explicitly
  // rather than relying on it).
  const trackLayout = useRef({ pageX: 0, width: 0 });
  const [dragFraction, setDragFraction] = useState<number | null>(null);

  const measureTrack = () => {
    trackRef.current?.measure((_x, _y, width, _height, pageX) => {
      trackLayout.current = { pageX, width };
    });
  };

  const fractionFromEvent = (evt: GestureResponderEvent): number => {
    const { pageX, width } = trackLayout.current;
    if (width <= 0) return 0;
    return Math.min(1, Math.max(0, (evt.nativeEvent.pageX - pageX) / width));
  };

  const panResponder = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: () => true,
      onPanResponderGrant: (evt) => {
        measureTrack();
        setDragFraction(fractionFromEvent(evt));
      },
      onPanResponderMove: (evt) => {
        setDragFraction(fractionFromEvent(evt));
      },
      onPanResponderRelease: (evt) => {
        const fraction = fractionFromEvent(evt);
        setDragFraction(null);
        onSeek(fraction);
      },
      onPanResponderTerminate: () => setDragFraction(null),
    }),
  ).current;

  const displayProgress = dragFraction ?? progress;

  return (
    <View
      ref={trackRef}
      style={styles.track}
      onLayout={measureTrack}
      {...panResponder.panHandlers}
    >
      <View style={[styles.fill, { width: `${displayProgress * 100}%` }]} />
      <View style={[styles.thumb, { left: `${displayProgress * 100}%` }]} />
    </View>
  );
}

const styles = StyleSheet.create({
  track: {
    height: TRACK_HEIGHT,
    borderRadius: TRACK_HEIGHT / 2,
    backgroundColor: "rgba(250,245,238,0.2)",
    justifyContent: "center",
  },
  fill: {
    position: "absolute",
    left: 0,
    top: 0,
    height: TRACK_HEIGHT,
    borderRadius: TRACK_HEIGHT / 2,
    backgroundColor: colors.terracotta,
  },
  thumb: {
    position: "absolute",
    width: THUMB_SIZE,
    height: THUMB_SIZE,
    borderRadius: THUMB_SIZE / 2,
    backgroundColor: colors.terracotta,
    borderWidth: 2,
    borderColor: colors.cream,
    marginLeft: -(THUMB_SIZE / 2),
    top: (TRACK_HEIGHT - THUMB_SIZE) / 2,
  },
});
