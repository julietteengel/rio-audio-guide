import React, { useMemo } from "react";
import { MapContainer, TileLayer, Marker } from "react-leaflet";
import { GoogleMap, MarkerF, useJsApiLoader } from "@react-google-maps/api";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import type { PlaceMapProps } from "./PlaceMap";
import { colors } from "../theme/tokens";
import { GOOGLE_MAPS_API_KEY } from "../config";
import { MAP_STYLE, PIN_ICON_SVG, ME_ICON_SVG } from "./googleMapStyle";

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

// Leaflet + OpenStreetMap tiles -- free, no API key, no billing account.
// Kept as the automatic fallback for anyone without GOOGLE_MAPS_API_KEY
// configured (a contributor's machine, or CI). See PlaceMap.tsx for the
// native counterpart (react-native-maps); same props on both.
function LeafletPlaceMap({ places, region, userLocation, youAreHereLabel, onSelectPlace }: PlaceMapProps) {
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

const GOOGLE_MAP_CONTAINER_STYLE = { height: "100%", width: "100%" };
const GOOGLE_MAP_OPTIONS = { styles: MAP_STYLE };

// Google Maps JS API, on-brand styled (see googleMapStyle.ts). Only
// rendered when GOOGLE_MAPS_API_KEY is configured -- see PlaceMap below
// for the fallback-to-Leaflet selection.
function GoogleWebPlaceMap({ places, region, userLocation, youAreHereLabel, onSelectPlace }: PlaceMapProps) {
  const { isLoaded, loadError } = useJsApiLoader({ googleMapsApiKey: GOOGLE_MAPS_API_KEY });
  const center = useMemo(
    () => ({ lat: region.latitude, lng: region.longitude }),
    [region.latitude, region.longitude],
  );

  // A script-load failure (network blocked, ad-blocker, malformed key
  // rejected at load time) leaves isLoaded false forever -- fall back to
  // Leaflet rather than showing a permanently blank map. This is distinct
  // from an invalid/unbilled key, where the script loads fine and Google
  // renders its own error overlay.
  if (loadError) return <LeafletPlaceMap places={places} region={region} userLocation={userLocation} youAreHereLabel={youAreHereLabel} onSelectPlace={onSelectPlace} />;

  // Google's Size/Point constructors only exist once the script has loaded
  // (they live on the runtime `google` global, not a static import) -- this
  // component renders nothing until then rather than risk calling them too
  // early. The map's sand-colored parent background (Map.tsx's `styles.map`)
  // already shows through during this brief gap, so no separate loading
  // state is needed.
  if (!isLoaded) return null;

  const pinIcon: google.maps.Icon = {
    url: PIN_ICON_SVG,
    scaledSize: new google.maps.Size(16, 16),
    anchor: new google.maps.Point(8, 8),
  };
  const meIcon: google.maps.Icon = {
    url: ME_ICON_SVG,
    scaledSize: new google.maps.Size(34, 34),
    anchor: new google.maps.Point(17, 17),
  };

  return (
    <GoogleMap mapContainerStyle={GOOGLE_MAP_CONTAINER_STYLE} center={center} zoom={12} options={GOOGLE_MAP_OPTIONS}>
      {places.map((p) => (
        <MarkerF
          key={p.id}
          position={{ lat: p.lat, lng: p.lon }}
          icon={pinIcon}
          title={p.name}
          onClick={() => onSelectPlace(p.id)}
        />
      ))}
      {userLocation ? (
        <MarkerF
          position={{ lat: userLocation.latitude, lng: userLocation.longitude }}
          icon={meIcon}
          title={youAreHereLabel}
          clickable={false}
        />
      ) : null}
    </GoogleMap>
  );
}

// Picks Google Maps when a key is configured, Leaflet otherwise -- see
// GOOGLE_MAPS_API_KEY's own doc comment in config.ts for why an unset key
// is a valid, expected state rather than an error.
export function PlaceMap(props: PlaceMapProps) {
  if (GOOGLE_MAPS_API_KEY) {
    return <GoogleWebPlaceMap {...props} />;
  }
  return <LeafletPlaceMap {...props} />;
}
