import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { LivePill } from '../../../shared/live-pill/live-pill';

/** The chip: live, late (reconnecting), lost or ended (`Encerrada às 23:40`); `null` draws none. */
export type TvLink = 'live' | 'late' | 'lost' | 'ended' | null;

/** The top of the TV: the icon of the screen, "Mirathel · Sessão 3", the title and the connection chip. */
@Component({
  selector: 'app-tv-header',
  imports: [LivePill, MatIconModule],
  templateUrl: './tv-header.html',
  styleUrl: './tv-header.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvHeader {
  readonly icon = input.required<string>();
  readonly kicker = input('');
  readonly title = input.required<string>();
  readonly link = input<TvLink>(null);
  /** The hour the session ended, for the `ended` chip. */
  readonly endedAt = input('');
}
