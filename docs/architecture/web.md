# Web app

`web/` is the Angular app.
The Go server serves its build on the same origin as the API, so the browser needs no CORS and the session cookie is first-party (ADR-0006).
This page covers how the app is served and secured, how it is organised, how it talks to the server, and the screens of sign-in, campaigns, invites and claim links.
How a screen is designed and reviewed is in [Design](../design/README.md).

## How it is served

`platform/httpserver.NewStatic` serves `index.html` and the build's files from `WEB_DIR` (`/app/web` in the image).
Without a valid `WEB_DIR` the server starts API-only and logs a line; that is the normal case outside the Docker image (`make run`, for example).
A path that is not an API path falls back to `index.html`, so the Angular router takes over.
A path whose last segment has the build's file shape (`name-<8 upper-case letters or digits>.ext`) never falls back: when the file does not exist, it gets a 404 with `Cache-Control: no-store`.
A client route must not end in that shape.
The API paths (`isAPIPath`) are `/meurpg.*`, `/auth/*`, `/images/*`, `/uploads/*`, `/downloads/*`, `/healthz` and `/readyz`: an unknown path under them is a plain 404, never the page.

| Aspect | Behaviour |
| --- | --- |
| Cache | A hashed asset (`name-<8 characters>.ext`, from `outputHashing: "all"`) gets `Cache-Control: public, max-age=31536000, immutable`. `index.html` and files copied as they are (`favicon.ico`, `material-symbols/*`) get `no-cache`. |
| MIME types | The server teaches Go's `mime` package `.webmanifest` (`application/manifest+json`) and `.ico` (`image/x-icon`); the production image has no `mime.types`. |
| Compression | `.js` and `.css` files have `.br` and `.gz` copies from the build; the server sends the best one the request accepts, with `Vary: Accept-Encoding` ([Performance](../performance.md#served-compressed)). `index.html` is never encoded. |
| A stale tab | A tab open across a deploy asks for lazy files that no longer exist. The first such failure reloads the page once (`core/system/chunk-reload.ts`), for a lazy route and for a `@defer` block; a second failure on the same page shows the error and does not reload. A load that a navigation cancels (`beforeunload` fired) does not reload the page. The recovery matches a fixed phrase of each engine's message for a lazy file that cannot load. WebKit's phrase for a lazy file answered with a page instead of a script is `is not a valid JavaScript MIME type for module script`. Any other module error does not match and reaches the error handler. |
| Legal pages | `/terms`, `/privacy` and `/credits` are lazy routes of the app, public ([Design](../design/README.md#logo-icons-and-footer)). |
| Dev | `cd web && npm start` runs Angular with `proxy.conf.json`, forwarding the API paths to `localhost:8080`. |
| Codegen | `make proto` runs `buf generate`; `protoc-gen-es` (Connect-ES v2: one plugin makes messages and services) writes `web/src/gen/`, committed like the Go side. |

### Content Security Policy

The app's CSP is `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'self'; form-action 'self' <the OIDC provider's origin>; frame-ancestors 'none'; object-src 'none'` (`cspHeader`, `backend/internal/platform/httpserver/static.go`).

- **No exception in `script-src`.** The production build has no inline `<script>` or event attribute (`inlineCritical: false`).
- **`style-src` needs `'unsafe-inline'`:** Angular injects each component's CSS into a `<style>` tag at runtime.
  The `cspHeader` comment describes the alternative, a per-request nonce, and why it is not worth it yet.
- **`form-action` lists the sign-in provider's origin** (`WithFormActionOrigin`, from `OIDC_ISSUER`).
  Browsers apply `form-action` to the whole redirect chain of a form submission, and the invite and claim pages post to `/auth/login`, which answers `303` to the provider; without the origin the button does not leave the page.
- **`base-uri` stays `'self'`:** `index.html` has `<base href="/">`, which the router depends on.
- **No external requests:** images, fonts and icons are self-hosted; images never load from an external URL.

### Security headers

`securityHeaders` (`platform/httpserver/middleware.go`) sets, on every response (RPCs, `/auth/*`, images, uploads, probes, 404s):

| Header | Value |
| --- | --- |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `same-origin` (the `/auth/*` routes set `no-referrer`) |
| `Permissions-Policy` | Denies camera, microphone, geolocation, payment, USB, sensors and the rest the app does not use; the clipboard, which "copy link" uses, is not restricted. |
| `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy` | `same-origin` |
| `Content-Security-Policy` | `default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'` for what is not a page. The static handler replaces it with the app CSP. |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains`, only when the request arrived over HTTPS (direct TLS or `X-Forwarded-Proto: https`), never on `http://localhost`. On Cloud Run Google's front end terminates TLS and the container is reachable no other way, so a client cannot forge it. |

## Folders and layer rules

`web/src/app` has four layers, from the bottom up:

| Layer | Holds | May import |
| --- | --- | --- |
| `core/` | Connect clients, state, display helpers, one folder per subject (`auth`, `connect`, `campaigns`, `combat`, `maps`...) | Not `pages/` and not `shell/`. It should not import `shared/` either, but that is not enforced and some files do. |
| `shared/` | Components that several pages use (`sheet`, `map-view`, `markdown`, `retry-image`...) | `core/`, never `pages/` |
| `pages/` | One folder per route, loaded lazily (`loadComponent` or `loadChildren`) | `core/`, `shared/` and its own sub-folders, never another page |
| `shell/` | The app frame: footer, live-session notice, session notes, user menu | everything below |

What two pages share moves to `shared/` or `core/`.
Import cycles are never allowed.
CI enforces this with dependency-cruiser (`web/.dependency-cruiser.cjs`, `npm run depcruise`); `web/.dependency-cruiser-known-violations.json` is empty, so any violation fails ([CONTRIBUTING](../ci.md#web-and-e2e-linters)).
The generated code in `web/src/gen` is outside the layers.

The visual system is `--mr-*` CSS properties in `web/src/styles.scss` and shared pieces in `web/src/styles/_ui.scss` ([Design](../design/README.md)).
Fonts are Alegreya and Alegreya Sans (OFL), Latin subset in woff2, from `@fontsource`; icons are the self-hosted Material Symbols font (`@material-symbols/font-400`, copied by an `assets` entry in `angular.json`).
Both are served by Go itself, never from Google Fonts, which the CSP blocks and [Privacy](../privacy.md) forbids.
Licences are in `NOTICE` and `third_party/licenses/`.

## The transport

One Connect transport (`web/src/app/core/connect/transport.ts`) serves every generated client:

- **`baseUrl: '/'`:** the app is on the API's origin, so there is no API URL to configure.
- **`credentials: 'same-origin'`** is forced on `fetch`, so the session cookie goes on every call and never depends on a runtime default.
- **`Connect-Protocol-Version: 1`** is added to every unary call by `@connectrpc/connect` itself ([Identity](identity.md#csrf)).
- **A 60 s deadline** on every unary call that sets none (`withUnaryDeadline`), sent as `Connect-Timeout-Ms` so the server cancels the work too.
  A hung call ends in `deadline_exceeded`, which screens treat like any server failure, instead of an eternal loading indicator.
  Streams are excluded.
  60 s is above the longest legitimate long poll (`GetImageGeneration` with `wait_seconds`, at most 25 s); a call that needs more passes its own `timeoutMs`.
- **`rateLimitInterceptor`** turns a rate-limit refusal into `unavailable` with the wait in its message ([API](api.md#abuse-limits)).
- **`sessionEndedInterceptor`** reacts to `unauthenticated` ([below](#session-state)).

Connect errors become Portuguese messages through a `Code → text` map (`core/connect/connect-errors.ts`).
`unauthenticated` reads "Sua sessão acabou. Entre de novo para continuar." unless the screen has its own wording, never the "server unavailable" text, which no retry fixes.
`unknown`, which the server answers when a commit is ambiguous, reads "Não deu para confirmar se a alteração foi salva. Confira na tela antes de tentar de novo."
`AcceptInvite`'s `failed_precondition` carries the `InviteUnusable` detail, which `core/campaigns/invite-errors.ts` reads to say why (expired, revoked, used up).
`core/characters/character-errors.ts` gives every `CharacterBlockedReason` a sentence of its own, and `character-blocked-reasons.spec.ts` walks the generated enum so a reason added to `characters.proto` fails the spec until its sentence exists.
Every reason the server can send is in `proto/error-reasons.json`, and `core/errors/error-reasons.spec.ts` fails for a listed reason with no sentence (body reasons are worded in `core/errors/body-reasons.ts`); see [Error reasons](api.md#error-reasons).
A campaign the user is not a member of and one that does not exist arrive as the same `not_found` (ADR-0011) and show the same screen: the app never tries to tell them apart.

## Session state

`AuthService` (`core/auth/auth.service.ts`) calls `IdentityService.GetMe` at startup and whenever a call learns the session ended, and keeps the result in a signal:

| State | When |
| --- | --- |
| `unknown` | `GetMe` has not returned yet. |
| `signed-out` | `GetMe` answered `unauthenticated`: at startup, or after another call did (signed out in another tab, "Sair dos outros dispositivos", expiry). |
| `signed-in` | `GetMe` answered with the user and `sessionExpiresAt`. |
| `unavailable` | Any other Connect code, or a network failure. It never becomes `signed-out`: a server or database down must not look like a signed-out user. |

- **A session that ends under an open tab** is learned from any unary call: `sessionEndedInterceptor` answers `unauthenticated` from any call except `GetMe` and `SignOut` (the reader itself, which would loop) by asking `AuthService.refresh()` to read `GetMe` again; calls arriving meanwhile share it.
  The account menu then shows "Entrar", the open-sessions poll stops and guarded routes send to sign-in on the next navigation.
- **Signing in** is always a full-page navigation to `/auth/login?return_to=<path>`, never an Angular route (`AuthService.signIn`): the server drives the OIDC flow ([Identity](identity.md)).
  `signIn` drops the fragment, which may hold a secret (`/invite#t=...`), and accepts only a path of this site.
- **`authGuard`** protects the routes that need a session: `signed-in` passes; `signed-out` goes to sign-in with `return_to`; `unavailable` redirects to the "server unavailable" page with the path in `return_to`, so "Tentar de novo" returns to it; `unknown` waits for the first resolved state.
- **Signing out** calls `IdentityService.SignOut` and always returns to the home page with a full-page navigation, even if the call fails: a silently failing "Sair" is worse than a cookie a later `GetMe` corrects.
  `endSessionHere` ends the session and stays on the page, for "Não é você? Entrar com outra conta".

## Nothing in the browser besides the session cookie

Nothing in `web/` reads or writes `localStorage`, `sessionStorage` or IndexedDB, writes a cookie from JavaScript, or uses a Worker to hold a token or personal data.
The session lives only in the `__Host-meurpg_session` cookie, `HttpOnly`, which the app's JavaScript never touches.
The one-reload guard of a stale tab is the entry's own `history.state` plus a flag in memory, so it needs no storage.
The invite and claim tokens live only in a component field, in memory.
`web/src/no-web-storage.spec.ts` is the guard: it scans every `.ts` and `.html` in `web/src` (except `gen/` and specs) for those APIs and fails if one appears ([Privacy](../privacy.md)).

This is the backend-for-frontend pattern that the IETF's "OAuth 2.0 for Browser-Based Applications" ranks strongest: the OIDC tokens stay on the server, and the browser holds only a cookie that JavaScript cannot read.
The invite token is not stored either: it goes in the body of `POST /auth/login`, and the server keeps only its hash in the single-use, 10-minute login state.

## Screens: campaigns, invites, profile and claim

These screens live in `web/src/app/pages/`, all lazy, on the generated `CampaignService` client (`core/campaigns/campaigns.service.ts`).

| Route | Screen | Guard |
| --- | --- | --- |
| `/campaigns` | `ListMyCampaigns` with each one's role, or "Esperando a aprovação do mestre" for a pending member (RN-15); a "Criar campanha" form (name 1 to 80 characters, XP mode) | `authGuard` |
| `/campaigns/:id`, `/campaigns/:id/prep`, `/campaigns/:id/setup` | The campaign page: `GetCampaign` and `ListMembers`. The master's page has three lazy child routes, "Mesa" (the default), "Preparação" and "Regras", under a session strip and a nav of three links ([Design: the campaign page](../design/campaign.md)). A player's page is one page with no child route. A pending member gets only `GetCampaign`: the waiting notice and their own character. | `authGuard` |
| `/invite` | Accepts an invite by the token in the URL fragment | public (signed in or out) |
| `/invite/error` | A message by `reason`, after the invite sign-in flow | public |
| `/claim` | Takes a reserved character with the master's link `/claim#t=<token>` | public (signed in or out) |
| `/campaigns/:id/reserved/new` | The character editor in the master's mode, saving the character as reserved | `authGuard` |
| `/profile` | "Meu perfil": the display name (1 to 40 characters, `UpdateProfile`) and, under "Sessões", counts the other signed-in devices and signs them out (`CountOtherSessions`, `SignOutOtherSessions`) | `authGuard` |
| `/campaigns/:id/table-screen` | The table screen, the TV at the table ([below](#the-table-screen-page)) | `authGuard` |

Other routes (the sheet, session, maps, gallery, export and import, rules...) are in `app.routes.ts`; they belong to their modules' pages.

### The campaign page's tabs

`CampaignDetail` (`pages/campaign-detail`) reads the campaign and its members once and keeps them in `CampaignView`; the three tabs are child routes drawn by its `router-outlet`, so a tab change makes no call.
Three services are provided by the page and shared by what it draws: `CampaignSessions` (`ListGameSessions`, for the strip and "Sessões anteriores"), `CampaignRoster` (`ListCharacters`, for the players' panel, the NPCs' panel and the count on "Mesa") and `ExperienceStore`.
The tab nav marks the open tab with `aria-current="page"` (`routerLinkActive`) and keeps the focus on the link that was used, so the shell does not move it to the `h1`.
A link to `/campaigns/:id#maps` ("Abrir os mapas", in "Regras da mesa") lands on `/campaigns/:id/prep#maps` for the master.

### The invite page

`pages/invite/invite-accept.ts` handles `/invite#t=<token>`:

1. The constructor reads the token from `location.hash` and calls `history.replaceState` immediately, before anything else, so the visible URL never shows it after the first instant.
   The token stays in a private field; it never becomes a route parameter or a query string.
2. Without a token: "Link de convite inválido."
3. With a token and a session, it calls `AcceptInvite`.
   Success, `already_member` included, goes to `/campaigns/<id>`.
   Someone who just became a pending member (`awaiting_approval` and not `already_member`) goes straight to `/campaigns/<id>/characters/new`.
   An unusable invite shows the message from `invite-errors.ts`.
   A server or network failure shows its message with "Tentar de novo", which asks again with the token kept in memory (the fragment is gone, so a reload could not).
4. With a token and no session it shows "Entrar para aceitar o convite".
   The click builds and submits a hidden `<form method="post" action="/auth/login">` with `return_to=/campaigns`, `intent=campaign_invite` and `intent_payload=<token>` ([Campaigns](campaigns.md#accepting-an-invite-through-sign-in)).
   The menu's "Entrar" never carries the fragment into `return_to`.

### The claim page

`pages/claim/claim-page.ts` follows the invite page: the constructor reads `#t=<token>` and calls `replaceState` first.
Signed out, it asks the server nothing and posts `intent=character_claim`.
Signed in, it calls `PreviewClaim` with the token, or with an empty one when returning from sign-in, and only the click on "Assumir este personagem" calls `ClaimCharacter`.
"Não é você? Entrar com outra conta" ends this session (`endSessionHere`) and posts the same form with `prompt=select_account`, keeping the token in memory.
The page never shows an e-mail or a rule id ([Campaigns](campaigns.md#claim-links)).

### The table screen page

`pages/table-screen` is the TV tab ([The table screen](play/table-screen.md), [Design](../design/session.md#the-table-screen-tela-da-mesa)).
`TABLE_SCREEN_ROUTES` is loaded with `loadChildren` and provides its own `TableScreenWatcher`, so the page owns one stream and one read loop and the eager routes import nothing of the generated client behind it.

- **No shell.** `app.html` draws the bar, the footer, the live-session notice and the dice overlay only when `isTableScreenPage(url)` (`core/play/open-sessions.ts`) is false, so the tab shows the page alone.
- **Nothing else is called.** The shell calls `OpenSessions.suspend(true)` on this route, which stops the open-sessions poll (`ListOpenGameSessions`), and the page's only data services are `TableScreenWatcher` and `AuthService`.
  The tab's calls are `GetMe` (the guard), `GetTableScreen`, `WatchTableScreen` and the map tiles `?as=tv`; `table-screen.spec.ts` in `e2e/` records them ([Testing](../testing.md#end-to-end-tests-playwright)).
- **The watcher** (`core/play/table-screen-watcher.ts`) opens `WatchTableScreen` through `LiveStream` (backoff, dead-stream check, pause while the tab is hidden) and reads `GetTableScreen` after every `ready` and, gathered over 300 ms, after every `changed`.
  Reads never overlap: a `changed` that comes during a read makes exactly one more read after it.
  The stream carries no content, so the screen is only ever what a read answered.
- **The state machine** (`core/play/table-screen-state.ts`, `TvScreenState`) turns the watcher's inputs (a good read, a failed read, the connection, the end, a refusal) into one `TvPhase`: `loading`, `error` (a new read every 10 s), `ready` (with the "late" text after 3 s without a connection or with failing reads), `stale` (a minute without one: the stage is hidden), `ended`, `no-access` and `signed-out` (the page sends the viewer to `AuthService.signIn`).
  Its time comes from a `TvClock`, so a spec drives the 3 s, 10 s and 60 s without timers.
  `table-screen-view.ts` maps the response to the view models (`toTvScreen`), and `table-screen-labels.ts` holds the words of the stage, the notice and the bar button.
- **The master's side** is `TableScreenPanel` (`core/play/table-screen-panel.ts`), a plain class that `live-session` builds with its own `TableScreenClient`: it reads `GetTableScreen`, writes the choice with the revision it read, keeps the TV's presence from `table_screen_changed`, and feeds the bar button and the panel (`pages/live-session/table-screen-sheet`).
  It reads only while the session is open, and the panel stores nothing in the browser.
- **`tv-fit`** (`appTvFit`, `pages/table-screen/tv-fit`) draws only the children of a list that fit its height: it hides the first child that is cut and every one after it (the first child always stays), counts the ones with `data-count` and emits that count as `fitted` for the "+N".
  It measures again after each render and whenever the list's size changes (`ResizeObserver`); a list with no height of its own (a phone) hides nothing.
- **Layer rule.** The master's panel is in `pages/live-session` and the TV is in `pages/table-screen`; the two share only `core/play` (the client, the labels and the panel's state) and `shared/`.
  dependency-cruiser's `page-not-to-other-page` fails an import between them, so the words both need (`table-screen-labels.ts`) live in `core/play`.
- **`FogMap`** has a `tools` input (default on) that draws the zoom, fit and full-screen buttons; the TV passes `false` because nobody touches it.
  Its picture is at most `--fm-max-h` tall (78 vh by default); the TV's map card sets it to the height of its own container, so the legend stays inside the card ([Design: fog of war](../design/maps.md#fog-of-war-sharedfog-map)).

## Tests

- **Unit tests** (Vitest, `web/src/**/*.spec.ts`) cover error mapping, the fragment cut (`replaceState` called, no storage touched), the sign-in form fields, and the approval flow.
- **End to end** (Playwright, in `e2e/tests`): `campaigns.spec.ts`, `invite.spec.ts` (including the invite sign-in path), `character-approval.spec.ts`, `claim-links.spec.ts`, `login.spec.ts` and `sessions.spec.ts`, tagged by story (`@MR-001`...).
- **Accessibility:** `a11y.spec.ts` runs axe on the main screens ([CONTRIBUTING](../testing.md#end-to-end-tests-playwright)).
