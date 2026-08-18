import { useTranslations, useLocale } from "next-intl";
import { Link } from "@/i18n/navigation";
import { routing } from "@/i18n/routing";

const LOCALE_LABELS: Record<string, string> = {
  fr: "FR",
  en: "EN",
  pt: "PT",
  es: "ES",
};

export function Footer() {
  const t = useTranslations("landing");
  const activeLocale = useLocale();

  return (
    <footer className="border-t border-line bg-cream py-12">
      <div className="mx-auto max-w-6xl px-8">
        <div className="flex flex-wrap items-end justify-between gap-6">
          <div>
            <p className="font-display mb-1.5 text-base font-bold uppercase tracking-wide text-ink">
              Memória Carioca
            </p>
            <p className="text-[13px] text-ink-faint">{t("footerTagline")}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {routing.locales.map((locale) => (
              <Link
                key={locale}
                href="/"
                locale={locale}
                aria-current={locale === activeLocale ? "page" : undefined}
                className={
                  "rounded-full border px-4 py-1.5 text-xs font-bold tracking-wide " +
                  (locale === activeLocale
                    ? "border-terracotta bg-terracotta text-cream"
                    : "border-line text-ink-soft hover:text-ink")
                }
              >
                {LOCALE_LABELS[locale]}
              </Link>
            ))}
          </div>
        </div>
        <div className="mt-8 flex flex-wrap justify-between gap-2 border-t border-line pt-5 text-xs text-ink-faint">
          <span>{t("copyright")}</span>
          <span>{t("footerBottomRight")}</span>
        </div>
      </div>
    </footer>
  );
}
