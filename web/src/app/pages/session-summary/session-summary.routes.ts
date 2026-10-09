import { Routes } from '@angular/router';

import { GameSessionSource } from '../campaign-detail/game-session/game-session-card.types';
import { GameSessionSourceLive } from '../campaign-detail/game-session/game-session-source.live';

/**
 * Lazily loaded from `app.routes.ts` via `loadChildren` for
 * `/campaigns/:id/sessions/:number`, so the page gets its route-scoped
 * `GameSessionSource` (which lists the campaign's sessions) without the eager
 * `app.routes.ts` importing the generated client behind it — see
 * `../character-sheet/character-sheet.routes.ts`.
 */
export const SESSION_SUMMARY_ROUTES: Routes = [
  {
    path: '',
    providers: [{ provide: GameSessionSource, useClass: GameSessionSourceLive }],
    loadComponent: () => import('./session-summary-page').then((m) => m.SessionSummaryPage),
  },
];
