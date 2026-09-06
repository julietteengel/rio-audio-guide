import React, { useEffect, useRef, useState } from "react";
import { View, StyleSheet, PanResponder, Platform, type GestureResponderEvent } from "react-native";
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
    if (Platform.OS === "web") {
      const node = trackRef.current as unknown as HTMLElement | null;
      if (node?.getBoundingClientRect) {
        const rect = node.getBoundingClientRect();
        trackLayout.current = { pageX: rect.left, width: rect.width };
        return;
      }
    }
    trackRef.current?.measure((_x, _y, width, _height, pageX) => {
      trackLayout.current = { pageX, width };
    });
  };

  const fractionFromEvent = (evt: GestureResponderEvent): number => {
    const { pageX, width } = trackLayout.current;
    if (width <= 0) return 0;
    return Math.min(1, Math.max(0, (evt.nativeEvent.pageX - pageX) / width));
  };

  const onSeekRef = useRef(onSeek);
  onSeekRef.current = onSeek;

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
        onSeekRef.current(fraction);
      },
      onPanResponderTerminate: () => setDragFraction(null),
    }),
  ).current;

  // On web, PanResponder's translation of mouse events through RN's touch
  // Responder System is not reliable for this kind of press-drag-release
  // track (clicks and drags could land wrong or do nothing at all).
  // Attaching plain DOM mouse listeners directly to the underlying element
  // sidesteps that translation layer entirely -- these are ordinary,
  // well-understood browser APIs, not a guess about how RN's responder
  // system behaves once it goes through react-native-web.
  useEffect(() => {
    if (Platform.OS !== "web") return;
    const node = trackRef.current as unknown as HTMLElement | null;
    if (!node) return;

    const fractionFromClientX = (clientX: number): number => {
      const rect = node.getBoundingClientRect();
      if (rect.width <= 0) return 0;
      return Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
    };

    let dragging = false;
    const onMouseDown = (e: MouseEvent) => {
      dragging = true;
      setDragFraction(fractionFromClientX(e.clientX));
    };
    const onMouseMove = (e: MouseEvent) => {
      if (!dragging) return;
      setDragFraction(fractionFromClientX(e.clientX));
    };
    const onMouseUp = (e: MouseEvent) => {
      if (!dragging) return;
      dragging = false;
      const fraction = fractionFromClientX(e.clientX);
      setDragFraction(null);
      onSeekRef.current(fraction);
    };

    node.addEventListener("mousedown", onMouseDown);
    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      node.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, []);

  const displayProgress = dragFraction ?? progress;

  return (
    <View
      ref={trackRef}
      style={styles.hitArea}
      onLayout={measureTrack}
      hitSlop={{ top: 12, bottom: 12 }}
      {...(Platform.OS === "web" ? {} : panResponder.panHandlers)}
    >
      <View style={styles.track}>
        <View style={[styles.fill, { width: `${displayProgress * 100}%` }]} />
        <View style={[styles.thumb, { left: `${displayProgress * 100}%` }]} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  // A taller invisible touch target around the thin visual track -- 4px is
  // too thin to reliably tap/click, on any platform.
  hitArea: { paddingVertical: 12, justifyContent: "center" },
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
