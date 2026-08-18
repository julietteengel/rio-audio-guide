import React, { createContext, useContext, useEffect, useMemo, useState } from "react";
import * as Localization from "expo-localization";
import { dictionary, DEFAULT_LOCALE, SUPPORTED_LOCALES, Locale, Dictionary } from "./dictionary";
import { setPlacesRepositoryLocale } from "../data/PlacesRepository";

function detectInitialLocale(): Locale {
  const deviceLocales = Localization.getLocales();
  for (const l of deviceLocales) {
    const code = l.languageCode?.toLowerCase();
    if (code && (SUPPORTED_LOCALES as string[]).includes(code)) {
      return code as Locale;
    }
  }
  return DEFAULT_LOCALE;
}

type LocaleContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: Dictionary;
};

const LocaleContext = createContext<LocaleContextValue | null>(null);

export function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocale] = useState<Locale>(() => detectInitialLocale());

  // HttpPlacesRepository.getById needs the current language for every
  // request but isn't a React component — kept in sync via this module-level
  // setter rather than threading locale through the repository interface.
  useEffect(() => {
    setPlacesRepositoryLocale(locale);
  }, [locale]);

  const value = useMemo<LocaleContextValue>(
    () => ({ locale, setLocale, t: dictionary[locale] }),
    [locale],
  );

  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

export function useLocale() {
  const ctx = useContext(LocaleContext);
  if (!ctx) {
    throw new Error("useLocale must be used within a LocaleProvider");
  }
  return ctx;
}
