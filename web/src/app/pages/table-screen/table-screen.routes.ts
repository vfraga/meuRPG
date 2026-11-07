import { Routes } from '@angular/router';

import { TableScreenWatcher } from '../../core/play/table-screen-watcher';

/**
 * Lazily loaded from `app.routes.ts` via `loadChildren` for `/campaigns/:id/table-screen`, so the page gets its
 * own `TableScreenWatcher` (one stream and one read loop for the tab) without the eager `app.routes.ts` importing
 * the generated client behind it — see `../session-summary/session-summary.routes.ts`.
 */
export const TABLE_SCREEN_ROUTES: Routes = [
  {
    path: '',
    providers: [TableScreenWatcher],
    loadComponent: () => import('./table-screen-page').then((m) => m.TableScreenPage),
  },
];
