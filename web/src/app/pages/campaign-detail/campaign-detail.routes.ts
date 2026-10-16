import { Routes } from '@angular/router';

import { XpWatcher } from '../../core/play/xp-watcher';
import { LiveSessionSourceLive } from '../../core/play/live-session-source.live';
import { CampaignCharactersSource } from './characters/campaign-characters.types';
import { CampaignCharactersSourceLive } from './characters/campaign-characters-source.live';
import { GameSessionSource } from '../../core/play/game-session-card.types';
import { GameSessionSourceLive } from '../../core/play/game-session-source.live';

/**
 * Lazily loaded from `app.routes.ts` via `loadChildren` for `/campaigns/:id`
 * — see `../character-sheet/character-sheet.routes.ts`'s doc comment for
 * why. `CampaignDetail` renders `<app-campaign-characters>` and (for the
 * master) `<app-game-session-card>` directly in its template, and both
 * resolve their source through this route's environment injector, same as
 * `CampaignDetail` itself.
 */
export const CAMPAIGN_DETAIL_ROUTES: Routes = [
  {
    path: '',
    providers: [
      { provide: CampaignCharactersSource, useClass: CampaignCharactersSourceLive },
      { provide: GameSessionSource, useClass: GameSessionSourceLive },
      // The session's stream client, for the master's `xp_changed` (a level-up arrives on it, MR-040).
      LiveSessionSourceLive,
      XpWatcher,
    ],
    loadComponent: () => import('./campaign-detail').then((m) => m.CampaignDetail),
  },
];
