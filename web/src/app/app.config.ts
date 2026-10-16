import { FullscreenOverlayContainer, OverlayContainer } from '@angular/cdk/overlay';
import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { MatIconRegistry } from '@angular/material/icon';
import { provideAnimationsAsync } from '@angular/platform-browser/animations/async';
import { TitleStrategy, provideRouter } from '@angular/router';

import { routes } from './app.routes';
import { AuthService } from './core/auth/auth.service';
import { SESSION_REFRESHER } from './core/auth/session-refresher';
import { ConfirmGuard } from './core/confirm-guard/confirm-guard';
import { PageTitleStrategy } from './core/title/page-title';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),
    // The transport asks the auth state to read `GetMe` again when a call answers `unauthenticated`; it cannot
    // import `AuthService` (which injects the transport), so it asks for this token.
    { provide: SESSION_REFRESHER, useExisting: AuthService },
    // Every page's title, in the tab and in the history (WCAG 2.4.2).
    { provide: TitleStrategy, useExisting: PageTitleStrategy },
    // Loads the animations runtime lazily, only if a component (e.g. a
    // Material overlay or ripple) actually asks for it.
    provideAnimationsAsync(),
    // Dialogs and sheets (CDK overlays) follow a map on browser full screen: the browser paints only the full-screen
    // element's subtree, and this container moves itself into it (docs/design.md).
    { provide: OverlayContainer, useClass: FullscreenOverlayContainer },
    // <mat-icon>name</mat-icon> defaults to Google's "Material Icons"
    // ligature font, which this app does not ship (no Google Fonts — see
    // docs/privacy.md). Point it at the self-hosted Material Symbols
    // font instead (index.html's <link>, copied into the build by
    // angular.json's assets entry).
    provideAppInitializer(() => {
      inject(MatIconRegistry).setDefaultFontSetClass('material-symbols-outlined');
    }),
    // A double-click never confirms a destructive inline action: every inline
    // confirmation ignores clicks for its first 500 ms (docs/design.md).
    provideAppInitializer(() => inject(ConfirmGuard).start()),
    // No root fallback for CharacterSheetSource / CharacterEditorSource /
    // CampaignCharactersSource / GameSessionSource on purpose: each is
    // provided at the route level, scoped to the pages that need it (see
    // campaign-detail.routes.ts, character-sheet.routes.ts,
    // character-editor.routes.ts) — importing their generated-client-backed
    // implementations here would pull meurpg.characters.v1 / rules.v1 /
    // play.v1's generated code into the eager bundle. A route reached
    // without its provider fails loudly (NG0201), on purpose: better a
    // clear DI error caught in review or in a test than a screen that
    // silently renders "unavailable" because a route config is wrong.
  ],
};
