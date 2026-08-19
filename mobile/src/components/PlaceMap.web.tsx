import React, { useMemo } from "react";
import { MapContainer, TileLayer, Marker } from "react-leaflet";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import type { PlaceMapProps } from "./PlaceMap";
import { colors } from "../theme/tokens";

// Custom divIcon HTML instead of Leaflet's default marker image -- the
// default relies on image URLs (marker-icon.png etc.) that break under
// Metro's web bundler without extra asset-path configuration. A styled div
// sidesteps that entirely and matches the native pin's look (small dot,
// cream ring) instead of Leaflet's default blue teardrop.
function pinIcon(): L.DivIcon {
  return L.divIcon({
    className: "",
    html: `<div style="width:12px;height:12px;border-radius:6px;background:${colors.terracottaDark};border:2px solid ${colors.cream};box-shadow:0 1px 3px rgba(0,0,0,0.35);"></div>`,
    iconSize: [16, 16],
    iconAnchor: [8, 8],
  });
}

function meIcon(): L.DivIcon {
  return L.divIcon({
    className: "",
    html: `<div style="width:34px;height:34px;border-radius:17px;background:rgba(193,89,46,0.22);display:flex;align-items:center;justify-content:center;"><div style="width:15px;height:15px;border-radius:7.5px;background:${colors.terracotta};border:2.5px solid ${colors.cream};"></div></div>`,
    iconSize: [34, 34],
    iconAnchor: [17, 17],
  });
}

// Leaflet + OpenStreetMap tiles -- free, no API key, no billing account,
// unlike a Google Maps JS API web equivalent. See PlaceMap.tsx for the
// native counterpart (react-native-maps); same props on both.
export function PlaceMap({ places, region, userLocation, youAreHereLabel, onSelectPlace }: PlaceMapProps) {
  const pin = useMemo(() => pinIcon(), []);
  const me = useMemo(() => meIcon(), []);

  return (
    <MapContainer
      center={[region.latitude, region.longitude]}
      zoom={12}
      style={{ height: "100%", width: "100%" }}
      // react-leaflet doesn't ship its own attribution UI beyond what
      // TileLayer renders -- OpenStreetMap's terms require crediting it.
      attributionControl={true}
    >
      <TileLayer
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
      />
      {places.map((p) => (
        <Marker
          key={p.id}
          position={[p.lat, p.lon]}
          icon={pin}
          eventHandlers={{ click: () => onSelectPlace(p.id) }}
          title={p.name}
        />
      ))}
      {userLocation ? (
        <Marker
          position={[userLocation.latitude, userLocation.longitude]}
          icon={me}
          title={youAreHereLabel}
          interactive={false}
        />
      ) : null}
    </MapContainer>
  );
}
