import { useTranslations, useLocale } from "next-intl";
import { Link } from "@/i18n/navigation";
import { routing } from "@/i18n/routing";

const LOCALE_LABELS: Record<string, string> = {
  fr: "FR",
  en: "EN",
  pt: "PT",
  es: "ES",
};

export function Header() {
  const t = useTranslations("landing");
  const activeLocale = useLocale();

  return (
    <header className="border-b border-line bg-cream">
      <div className="mx-auto flex max-w-6xl items-center justify-between gap-5 px-8 py-5">
        <span className="font-display text-base font-bold uppercase tracking-wide text-ink">
          Memória Carioca
        </span>
        <div className="flex items-center gap-4">
          <nav className="flex gap-1 rounded-full border border-line p-1">
            {routing.locales.map((locale) => (
              <Link
                key={locale}
                href="/"
                locale={locale}
                aria-current={locale === activeLocale ? "page" : undefined}
                className={
                  "rounded-full px-3 py-1.5 text-xs font-bold tracking-wide " +
                  (locale === activeLocale
                    ? "bg-terracotta text-cream"
                    : "text-ink-soft hover:text-ink")
                }
              >
                {LOCALE_LABELS[locale]}
              </Link>
            ))}
          </nav>
          <span
            aria-disabled="true"
            className="cursor-default select-none rounded-md bg-terracotta px-5 py-2.5 text-sm font-bold text-cream opacity-90"
          >
            {t("cta")}
          </span>
        </div>
      </div>
    </header>
  );
}
