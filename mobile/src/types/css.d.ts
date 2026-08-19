// Metro's web bundler already knows how to handle a side-effect CSS import
// (leaflet/dist/leaflet.css in PlaceMap.web.tsx) -- this only tells
// TypeScript's checker the same thing, it has no runtime effect.
declare module "*.css";
