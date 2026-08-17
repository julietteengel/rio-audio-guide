# Landing page — design spec

**Date:** 2026-08-18
**Branch/worktree:** `frontend`, `.worktrees/frontend/`
**Status:** approved, ready for implementation plan

## Context

Memória Carioca is an offline cultural audio guide for Rio de Janeiro — not a tourist guide. It
tells the verified, sourced history of places (Wikipedia/Wikidata and other credible sources, with
anti-hallucination review), never inventing facts to fill a gap. The brand, copy, and full visual
design for this landing page were already brainstormed, written, and approved with the user across
an earlier session using Higgsfield (image generation) and Claude Design (a multi-artboard prototype
canvas). That prototype is the visual reference for this implementation — it is not being redesigned,
only rebuilt as production code.

Brand tokens already decided and validated:
- Primary color: terracotta `#C1592E`. Neutrals: ivory `#FAF5EE`, ink `#2B211B`, soft ink `#6B5D4F`.
- Display font: Playfair Display (Bold/Black). Body/UI font: Inter.
- Visual direction: antique copper-plate etching / pen-and-ink illustration style, never a colorful
  tourist-postcard look. The hero image is a photorealistic flat-lay of an aged wooden desk holding
  an etched postcard illustration (Corcovado/Christ the Redeemer, geographically accurate — Pão de
  Açúcar correctly separated across the bay), a quill pen, an inkwell, and a corner of old paper —
  generated via Higgsfield, left third of the frame deliberately empty for text overlay.
- App icon: a terracotta medallion with a cream "M" monogram in Playfair Display Black, on an azulejo
  tile texture border.

Existing page copy (hero eyebrow, "carioca" gloss line, lede paragraph, 3 numbered features, footer)
was already written and approved in French, English, Portuguese, and Spanish in the Claude Design
prototype and is reused verbatim, not rewritten.

This is a decomposed sub-project. The React Native mobile app (the second deliverable discussed with
the user) is out of scope here and gets its own spec and plan once this one ships.

## Decisions

- **No backend.** Purely static marketing content — no form, no API call, no dynamic data. The
  "Télécharger l'app" button is visually present but inert (no destination) until the app exists on
  the App Store.
- **Hosting: Vercel.** Deliberately decoupled from the backend's AWS infrastructure — the two systems
  never talk to each other at runtime, so there is no technical reason to host them together. Vercel
  is Next.js's zero-config target (CDN, PR previews, free at this traffic level); reproducing that on
  AWS (S3+CloudFront or Amplify) would add setup for no benefit here.
- **i18n: `next-intl`, URL-based locales** (`/fr`, `/en`, `/pt`, `/es`), not a client-side toggle. This
  makes each language independently indexable by Google and independently shareable as a link — the
  reason URL-based routing was chosen over the client-state toggle used in the prototype.
- **Root path (`/`) redirects based on browser language** (`Accept-Language`), falling back to `/en`
  if none of the four match.
- **Testing: light.** TypeScript (strict) + `next build` as the correctness gate, plus ESLint. No
  automated test suite for this iteration — the content is presentational, not business logic, and a
  human visual review is the right check for a marketing page of this size.

## Architecture

### Stack

Next.js 15, App Router, TypeScript, deployed on Vercel. Tailwind CSS, configured with the brand tokens
above as a custom theme, replacing the hand-written CSS custom properties used in the prototype. Fonts
via `next/font/google` (Playfair Display, Inter) — self-hosted by Next.js at build time, replacing the
manual base64 `@font-face` embedding used in the prototype (that technique existed only to satisfy the
Artifact sandbox's no-network-egress rule, which does not apply here).

### Routing & content

```
middleware.ts                 browser-language redirect at "/", unknown locale -> not-found
i18n/request.ts                next-intl config: supported locales, default/fallback locale (en)
messages/fr.json               all page copy in French (copied verbatim from the prototype)
messages/en.json
messages/pt.json
messages/es.json
app/[locale]/layout.tsx        <html lang={locale}>, font setup, next-intl provider, metadata incl.
                                alternates.languages (hreflang) so Google treats the 4 URLs as one
                                page's translations, not duplicate content
app/[locale]/page.tsx          assembles Header, Hero, Features, Footer
app/[locale]/not-found.tsx     invalid locale segment
```

All four locale pages are fully static (`generateStaticParams`), no runtime data fetching.

### Components

```
components/Header.tsx      wordmark, language switcher (real links: /en, /pt, /es, /fr), CTA button
components/Hero.tsx        eyebrow, "Memória Carioca" title, "carioca" gloss line, lede paragraph,
                            CTA, hero photo (next/image)
components/Features.tsx    the 3 numbered panels (I/II/III) — offline map, 4 languages, sourced-not-
                            invented
components/Footer.tsx      wordmark, tagline, language pills, copyright
```

The language switcher renders real `<Link>`s to the same page under each locale prefix, replacing the
prototype's client-state toggle.

### Assets

- Hero photo (desk flat-lay with the etched postcard, quill, and inkwell) and the app icon (terracotta
  azulejo medallion) are saved into this branch as production assets (`public/images/`,
  `app/icon.png`) rather than living only in the ephemeral design-prototype scratch files.
- The same icon file backs both the site favicon and the Open Graph share image
  (`metadata.openGraph.images`) — one asset, two Next.js file-convention usages.

### Error handling

The only failure surface is an invalid locale segment in the URL, handled by the standard
`not-found.tsx`. No forms, no client-side data fetching, so no other error states exist.

### Tooling

TypeScript strict mode, ESLint (Next.js default config), `next build` as the CI gate. A GitHub Actions
workflow on the `frontend` branch (typecheck + lint + build per PR) mirrors the pattern already in use
for the backend's CI.

## Out of scope

- The React Native mobile app (separate spec/plan, decomposed out of this brainstorm).
- Any backend integration, waitlist/email capture, or dynamic App Store link — all deferred until the
  app itself exists.
- An automated test suite — revisit if the page grows real interactivity or logic.
