import { NgTemplateOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { MatIconModule } from '@angular/material/icon';

import { AuthService } from '../../core/auth/auth.service';
import { TableScreenWatcher } from '../../core/play/table-screen-watcher';
import { fullscreenOfHost } from '../../core/ui/fullscreen';
import { TvCombat } from './tv-combat/tv-combat';
import { TvFogCard } from './tv-fog-card/tv-fog-card';
import { TvFullscreenHint } from './tv-fullscreen-hint/tv-fullscreen-hint';
import { type TvLink, TvHeader } from './tv-header/tv-header';
import { tvAnnouncement, tvHeading } from './tv-heading';
import { TvIdle } from './tv-idle/tv-idle';
import { TvImage } from './tv-image/tv-image';
import { TvMap } from './tv-map/tv-map';
import { TvMessage } from './tv-message/tv-message';
import { TvRolls } from './tv-rolls/tv-rolls';
import { TvScene } from './tv-scene/tv-scene';

/**
 * The table screen (`/campaigns/:id/table-screen`): the tab the master drags to the TV at the table. It draws what
 * `TableScreenWatcher` says (the stage the server chose, or the card of the state it is in) and calls nothing else:
 * no session client, no master's service. The shell is hidden on this route (see `app.html`).
 *
 * - One screen, no page scroll on a TV; on a phone it is one column that scrolls.
 * - A stage change is announced in a live region; a late connection is a warning notice, a lost one hides the stage.
 * - Signed out, the page sends the viewer to sign in and back, as the route's guard does.
 */
@Component({
  selector: 'app-table-screen-page',
  imports: [
    NgTemplateOutlet,
    MatIconModule,
    TvCombat,
    TvFogCard,
    TvFullscreenHint,
    TvHeader,
    TvIdle,
    TvImage,
    TvMap,
    TvMessage,
    TvRolls,
    TvScene,
  ],
  templateUrl: './table-screen-page.html',
  styleUrl: './table-screen-page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TableScreenPage {
  private readonly watcher = inject(TableScreenWatcher);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);

  protected readonly fullscreen = fullscreenOfHost();
  private readonly hintDone = signal(false);
  protected readonly phase = this.watcher.state.phase;

  /** The hint to go full screen: only where the browser can, and only until the tab is full screen or the hint is used. */
  protected readonly showHint = computed(
    () => this.fullscreen.available && !this.fullscreen.active() && !this.hintDone(),
  );

  protected readonly screen = computed(() => {
    const phase = this.phase();
    return phase.kind === 'ready' ? phase.screen : null;
  });
  protected readonly heading = computed(() => {
    const screen = this.screen();
    return screen ? tvHeading(screen) : null;
  });
  /** What a screen reader hears when the stage changes: the same text again is not announced twice. */
  protected readonly announcement = computed(() => {
    const screen = this.screen();
    return screen ? tvAnnouncement(screen) : '';
  });
  protected readonly link = computed<TvLink>(() => {
    const phase = this.phase();
    if (phase.kind === 'stale') {
      return 'lost';
    }
    if (phase.kind !== 'ready') {
      return null;
    }
    return phase.late ? 'late' : 'live';
  });
  protected readonly late = computed(() => {
    const phase = this.phase();
    return phase.kind === 'ready' ? phase.late : null;
  });

  constructor() {
    const campaignId = inject(ActivatedRoute).snapshot.paramMap.get('id') ?? '';
    this.watcher.start(campaignId);
    inject(DestroyRef).onDestroy(() => this.watcher.stop());
    // The sign-in is gone: the server's sign-in flow, and back to this page.
    effect(() => {
      if (this.phase().kind === 'signed-out') {
        untracked(() => this.auth.signIn(this.router.url));
      }
    });
  }

  protected enterFullscreen(): void {
    this.hintDone.set(true);
    this.fullscreen.toggle();
  }
}
