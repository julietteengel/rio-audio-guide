# Landing Page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Build the Memória Carioca marketing landing page as a static Next.js site, reproducing the
already-approved Claude Design prototype (copy, brand, hero photo) as production code with URL-based
i18n, deployed on Vercel.

**Spec:** `docs/superpowers/specs/2026-08-18-landing-page-design.md`

**Tech stack:** Next.js 15 (App Router), TypeScript, Tailwind CSS, `next-intl`, `next/font/google`.

**Testing:** light — `tsc --noEmit`, `next lint`, `next build` are the gate for every task. No
automated test suite (per spec).

---

## Task 1: Scaffold the Next.js project

**Files:** `web/` (new Next.js app), `web/app/globals.css`, `web/next.config.ts`

- [x] Step 1: `npx create-next-app@latest web --typescript --tailwind --app --eslint --src-dir=false --import-alias "@/*" --use-npm --no-turbopack` inside `.worktrees/frontend/`
- [x] Step 2 (deviation): scaffold generated Tailwind v4 (CSS-based config, no `tailwind.config.ts`)
      instead of the v3 JS-config this plan assumed — brand tokens added as `@theme inline` CSS custom
      properties in `web/app/globals.css` instead: `terracotta #C1592E`, `terracotta-dark #92401F`,
      `cream #FAF5EE`, `bg-alt #F1E6D6`, `ink #2B211B`, `ink-soft #6B5D4F`, `ink-faint #9A8B7C`,
      `line rgba(43,33,27,0.14)`
- [x] Step 3: `npm run build` passes on the untouched scaffold

## Task 2: i18n plumbing (next-intl)

**Files:** `web/i18n/request.ts`, `web/i18n/routing.ts`, `web/middleware.ts`, `web/messages/{fr,en,pt,es}.json`

- [x] Step 1: `npm install next-intl` in `web/`
- [x] Step 2: Define routing config (locales `['fr','en','pt','es']`, default `'en'`) in `web/i18n/routing.ts`
- [x] Step 3: `web/middleware.ts` using next-intl's middleware, matcher excluding `_next`/static files —
      root `/` resolves via `Accept-Language` negotiation, falls back to `en`
- [x] Step 4: Write `web/messages/fr.json`, `en.json`, `pt.json`, `es.json` — copy every string verbatim
      from the `DICT` object in
      `/private/tmp/claude-501/-Users-julietteengel-code-julietteengel-rio-audio-guide--worktrees-backend/4a2f9c64-4338-4a8f-b504-6548a1b9255c/scratchpad/memoria-landing/Main.dc.html`
      (keys: eyebrow, glossRest, lede, cta, f1h/f1p, f2h/f2p, f3h/f3p, footerTagline, footerBottomRight)
- [x] Step 5: `web/i18n/request.ts` wiring `getRequestConfig` to the routing config
- [x] Step 6: `npm run build` passes

## Task 3: Brand assets

**Files:** `web/public/images/hero.jpg`, `web/src/app/icon.png` (App Router icon convention)

- [x] Step 1: `cp` the hero photo from
      `.../scratchpad/memoria-landing/hero-illustration.jpg` to `web/public/images/hero.jpg`
- [x] Step 2: `cp` the app icon from `.../scratchpad/icon/icon-final-1024.png` to
      `web/src/app/icon.png` (Next.js auto-generates favicon sizes from this at build time) and to
      `web/public/images/icon-1024.png` (for reuse as the OG share image)

## Task 4: Root layout, fonts, metadata

**Files:** `web/app/[locale]/layout.tsx`, `web/app/layout.tsx` (redirect shell), `web/app/not-found.tsx`

- [x] Step 1: Move `app/page.tsx`/`app/layout.tsx` scaffold into `app/[locale]/` per next-intl's
      App Router convention
- [x] Step 2: `app/[locale]/layout.tsx`: `next/font/google` for Playfair Display (weights 400/700/900)
      and Inter (400/600/700), `<html lang={locale}>`, `NextIntlClientProvider`, `generateStaticParams`
      for the 4 locales, `generateMetadata` with title/description per locale plus
      `alternates.languages` (hreflang) pointing at the other 3 locale URLs, and
      `openGraph.images: ['/images/icon-1024.png']`
- [x] Step 3: `app/not-found.tsx` — plain not-found page for unknown locale segments
- [x] Step 4: `npm run build` passes, all 4 locale pages listed in the build output

## Task 5: Header component

**Files:** `web/components/Header.tsx`

- [x] Step 1: Wordmark "Memória Carioca" (Playfair, bold, small, uppercase tracked)
- [x] Step 2: Language switcher — real `<Link href="/en">` etc. (next-intl `Link`), active locale
      styled filled-pill terracotta, others outlined
- [x] Step 3: CTA button "Télécharger l'app" / per-locale copy — `<span>` or disabled-looking element,
      NOT a real link (per spec: inert until the app exists) — do not point it at `#` or `javascript:`,
      render it as a non-interactive styled element instead so it isn't a broken/dead link
- [x] Step 4: `npm run build` passes

## Task 6: Hero component

**Files:** `web/components/Hero.tsx`

- [x] Step 1: Full-bleed section, terracotta gradient background (`#7C3B20 → #B9552B → #D2793F`,
      135deg), `next/image` for `hero.jpg` absolutely positioned behind a dark-to-transparent
      left-to-right scrim (reproduce the prototype's gradient overlay so text stays legible over the
      photo's right-hand detail)
- [x] Step 2: Eyebrow (small dot + uppercase label), `<h1>` "Memória Carioca" (Playfair Black, large),
      gloss line ("carioca, adj.: ..." — italic "carioca"), lede paragraph, CTA button (same inert
      treatment as Header's, `btn-on-dark` style: cream bg, ink text)
- [x] Step 3: Responsive check at phone width — text column stays narrow enough not to run under the
      desk-scene's dense right-hand detail (max-width ~460px on desktop, full width stacked on mobile)
- [x] Step 4: `npm run build` passes

## Task 7: Features + Footer components

**Files:** `web/components/Features.tsx`, `web/components/Footer.tsx`

- [x] Step 1: `Features.tsx` — 3-column grid (1 column on mobile), each panel: Playfair roman numeral
      (I/II/III) in terracotta, `<h3>` heading, body paragraph, hairline divider between columns
      (not on mobile)
- [x] Step 2: `Footer.tsx` — wordmark + tagline left, 4 language pills right (same Link pattern as
      Header), hairline-topped copyright row: "© 2026 Memória Carioca" + locale-specific
      footerBottomRight string
- [x] Step 3: Assemble `app/[locale]/page.tsx` = Header + Hero + Features + Footer
- [x] Step 4: `npm run build` passes; manually diff against the Claude Design prototype screenshot
      description for parity (hero copy, 3 features, footer content match)

## Task 8: CI workflow

**Files:** `.github/workflows/frontend-web-ci.yml`

- [x] Step 1: On PR touching `web/**`: `npm ci`, `tsc --noEmit`, `next lint`, `next build`, working
      directory `web/`
- [x] Step 2: Commit

## Out of scope (per spec)

- Automated test suite, backend integration, real App Store link, waitlist form.
