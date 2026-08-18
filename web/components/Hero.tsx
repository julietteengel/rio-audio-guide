import Image from "next/image";
import { useTranslations } from "next-intl";

export function Hero() {
  const t = useTranslations("landing");

  return (
    <section className="relative flex min-h-[640px] items-center overflow-hidden bg-[linear-gradient(135deg,#7C3B20_0%,#B9552B_52%,#D2793F_100%)] text-cream">
      <div className="absolute inset-0">
        <Image
          src="/images/hero.jpg"
          alt=""
          fill
          priority
          sizes="100vw"
          className="object-cover"
        />
      </div>
      <div className="absolute inset-0 bg-[linear-gradient(100deg,rgba(28,14,7,0.78)_0%,rgba(28,14,7,0.56)_26%,rgba(28,14,7,0.22)_44%,rgba(28,14,7,0.04)_60%,rgba(28,14,7,0)_72%)]" />

      <div className="relative z-10 w-full py-18">
        <div className="mx-auto max-w-6xl px-8">
          <div className="max-w-[460px]">
            <p className="mb-6 flex items-center gap-2 text-[13px] font-bold uppercase tracking-widest text-cream/90 [text-shadow:0_1px_8px_rgba(0,0,0,0.35)]">
              <span className="h-1.5 w-1.5 flex-none rounded-full bg-cream" />
              {t("eyebrow")}
            </p>
            <h1 className="font-display text-[2.6rem] font-black leading-[1.02] [text-shadow:0_2px_16px_rgba(0,0,0,0.4)] sm:text-[3.4rem] lg:text-[4.6rem]">
              Memória Carioca
            </h1>
            <p className="mt-4 text-[15px] text-cream/85 [text-shadow:0_1px_8px_rgba(0,0,0,0.35)]">
              <b className="font-bold italic text-cream">
                {t("glossWord")}
              </b>
              {t("glossRest")}
            </p>
            <p className="mt-6 max-w-[400px] text-[17px] leading-[1.65] text-cream/95 [text-shadow:0_1px_8px_rgba(0,0,0,0.35)]">
              {t("lede")}
            </p>
            <span
              aria-disabled="true"
              className="mt-9 inline-block cursor-default select-none rounded-md bg-cream px-8 py-4 text-base font-bold text-ink"
            >
              {t("cta")}
            </span>
          </div>
        </div>
      </div>
    </section>
  );
}
