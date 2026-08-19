import { setRequestLocale } from "next-intl/server";
import { Header } from "@/components/Header";
import { Hero } from "@/components/Hero";
import { Features } from "@/components/Features";
import { Footer } from "@/components/Footer";

type Props = {
  params: Promise<{ locale: string }>;
};

export default async function LandingPage({ params }: Props) {
  const { locale } = await params;
  setRequestLocale(locale);

  return (
    <>
      <Header />
      <Hero />
      <Features />
      <Footer />
    </>
  );
}
