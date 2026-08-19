import type { Metadata } from "next";
import { Playfair_Display, Inter } from "next/font/google";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { notFound } from "next/navigation";
import { routing } from "@/i18n/routing";
import "../globals.css";

const playfair = Playfair_Display({
  variable: "--font-playfair",
  subsets: ["latin"],
  weight: ["400", "700", "900"],
});

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
  weight: ["400", "600", "700"],
});

export function generateStaticParams() {
  return routing.locales.map((locale) => ({ locale }));
}

type Props = {
  children: React.ReactNode;
  params: Promise<{ locale: string }>;
};

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const meta = await getTranslations({ locale, namespace: "meta" });

  const languages: Record<string, string> = {};
  for (const l of routing.locales) {
    languages[l] = `/${l}`;
  }

  return {
    // NEXT_PUBLIC_SITE_URL must be set in Vercel once the real domain is chosen;
    // memoriacarioca.example is a placeholder (.example is IANA-reserved, never a real domain).
    metadataBase: new URL(
      process.env.NEXT_PUBLIC_SITE_URL ?? "https://memoriacarioca.example",
    ),
    title: meta("title"),
    description: meta("description"),
    alternates: {
      languages,
    },
    openGraph: {
      title: meta("title"),
      description: meta("description"),
      images: ["/images/icon-1024.png"],
    },
  };
}

export default async function LocaleLayout({ children, params }: Props) {
  const { locale } = await params;

  if (!hasLocale(routing.locales, locale)) {
    notFound();
  }

  setRequestLocale(locale);

  return (
    <html lang={locale} className={`${playfair.variable} ${inter.variable}`}>
      <body className="font-sans antialiased">
        <NextIntlClientProvider>{children}</NextIntlClientProvider>
      </body>
    </html>
  );
}
