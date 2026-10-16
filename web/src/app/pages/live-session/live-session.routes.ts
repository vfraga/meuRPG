import { Routes } from '@angular/router';

import { LiveSessionSourceLive } from '../../core/play/live-session-source.live';
import { LiveSessionSource } from '../../core/play/live-session.types';

/**
 * Lazily loaded from `app.routes.ts` via `loadChildren` for
 * `/campaigns/:id/session`, so `LiveSessionSource` gets a route-scoped
 * provider without `app.routes.ts` (eager) importing the generated clients
 * behind it — see `../character-sheet/character-sheet.routes.ts`.
 */
export const LIVE_SESSION_ROUTES: Routes = [
  {
    path: '',
    providers: [{ provide: LiveSessionSource, useClass: LiveSessionSourceLive }],
    loadComponent: () => import('./live-session').then((m) => m.LiveSession),
  },
];
