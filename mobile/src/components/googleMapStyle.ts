import { colors } from "../theme/tokens";

// Muted warm-adjacent blue for water -- same family as colors.groundBg
// (#EBF2FF, used for the "sources verified" badge elsewhere in the app),
// slightly deeper so it reads as water rather than another UI surface.
// Not one of the shared theme tokens: water is the one map feature with no
// equivalent elsewhere in the app's palette, so it's declared locally here
// rather than added to theme/tokens.ts for a single call site.
const WATER = "#D9E6F5";

// Google Maps JS API custom style JSON -- warm palette matching the rest of
// the app instead of Google's default blue/green. POI icons/labels are
// hidden entirely: the app already renders its own place markers, and
// Google's default restaurant/shop icons would visually compete with them
// and open Google's own (non-app) info windows on tap.
export const MAP_STYLE: google.maps.MapTypeStyle[] = [
  { featureType: "water", elementType: "geometry", stylers: [{ color: WATER }] },
  { featureType: "landscape", elementType: "geometry", stylers: [{ color: colors.sand }] },
  { featureType: "road", elementType: "geometry", stylers: [{ color: colors.cream }] },
  { featureType: "road", elementType: "labels.text.fill", stylers: [{ color: colors.inkSoft }] },
  { featureType: "poi", stylers: [{ visibility: "off" }] },
];

// btoa is available in every web/browser context this file runs in (it's
// web-only, resolved by Metro instead of PlaceMap.tsx on native) -- the SVG
// markup below is plain ASCII (hex colors, simple shapes), so base64
// encoding it is always safe.
function toDataUri(svg: string): string {
  return `data:image/svg+xml;base64,${btoa(svg)}`;
}

// Matches today's Leaflet divIcon pin exactly (12px dot, 2px cream border)
// in a 16x16 box -- see PlaceMap.web.tsx's existing pinIcon(). The original
// divIcon also has a subtle drop shadow (box-shadow); deliberately dropped
// here rather than reproduced with an SVG filter -- low visual impact, not
// worth the extra markup.
const PIN_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="${colors.terracottaDark}" stroke="${colors.cream}" stroke-width="2"/></svg>`;

// Matches today's Leaflet "you are here" marker exactly: a 34x34 translucent
// blue halo behind a solid 15px blue dot with a 2.5px cream border -- see
// PlaceMap.web.tsx's existing meIcon(). Blue (colors.groundText), not
// terracotta, so the user's own position is never confused with a place pin.
const ME_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="34" height="34" viewBox="0 0 34 34"><circle cx="17" cy="17" r="17" fill="rgba(46,90,172,0.22)"/><circle cx="17" cy="17" r="7.5" fill="${colors.groundText}" stroke="${colors.cream}" stroke-width="2.5"/></svg>`;

export const PIN_ICON_SVG = toDataUri(PIN_SVG);
export const ME_ICON_SVG = toDataUri(ME_SVG);
