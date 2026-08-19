import { useTranslations } from "next-intl";

const NUMERALS = ["I.", "II.", "III."] as const;
const KEYS = [
  { h: "f1h", p: "f1p" },
  { h: "f2h", p: "f2p" },
  { h: "f3h", p: "f3p" },
] as const;

export function Features() {
  const t = useTranslations("landing");

  return (
    <section className="bg-bg-alt py-20">
      <div className="mx-auto grid max-w-6xl grid-cols-1 gap-10 px-8 sm:grid-cols-3 sm:gap-0">
        {KEYS.map(({ h, p }, i) => (
          <div
            key={h}
            className={
              "px-0 sm:px-10 " +
              (i > 0 ? "sm:border-l sm:border-line" : "") +
              (i === 0 ? " sm:pl-0" : "") +
              (i === KEYS.length - 1 ? " sm:pr-0" : "")
            }
          >
            <p className="font-display mb-4 text-[15px] font-bold tracking-wide text-terracotta">
              {NUMERALS[i]}
            </p>
            <h3 className="font-display mb-2.5 text-xl font-bold text-ink">
              {t(h)}
            </h3>
            <p className="text-[15px] leading-[1.65] text-ink-soft">{t(p)}</p>
          </div>
        ))}
      </div>
    </section>
  );
}
