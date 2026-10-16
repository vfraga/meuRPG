import { Component, computed, inject, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { OpenSessions, sessionsToAnnounce } from '../../core/play/open-sessions';

/**
 * The session notice under the app bar (RN-06, artboard E5-01): "A sessão 4
 * de Mirathel começou." with "Entrar na sessão" and a close button.
 *
 * One notice per open session the person plays in, oldest first, on every
 * page except that campaign's own page and any session page
 * (`sessionsToAnnounce`): a session that starts later adds a notice below,
 * so none changes under the person's finger. Closing a notice, or following
 * its link, hides it for this tab only (in memory, no Web Storage).
 *
 * The link is stroked, not filled: the notice lands on pages that already
 * have their own filled primary action, and a screen has one fill
 * (docs/design.md). The accent frame and the icon make it stand out.
 *
 * Part of the initial bundle (the app shell), so it uses plain elements
 * styled with the tokens, not MatButton.
 */
@Component({
  selector: 'app-live-notice',
  imports: [MatIconModule, RouterLink],
  templateUrl: './live-notice.html',
  styleUrl: './live-notice.scss',
})
export class LiveNotice {
  private readonly openSessions = inject(OpenSessions);

  /** The router's current URL (the shell passes it in). */
  readonly url = input.required<string>();

  protected readonly sessions = computed(() =>
    sessionsToAnnounce(this.openSessions.sessions(), this.openSessions.dismissed(), this.url()),
  );

  protected dismiss(sessionId: string): void {
    this.openSessions.dismiss(sessionId);
  }
}
