import { defineRouting } from "next-intl/routing";

export const routing = defineRouting({
  locales: ["fr", "en", "pt", "es"],
  defaultLocale: "en",
});

export type Locale = (typeof routing.locales)[number];
