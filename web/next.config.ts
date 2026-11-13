import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin("./i18n/request.ts");

const nextConfig: NextConfig = {
  // The Docker image needs the standalone server; Vercel builds its own output and breaks on it.
  output: process.env.VERCEL ? undefined : "standalone",
};

export default withNextIntl(nextConfig);
