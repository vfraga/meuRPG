import { ChangeDetectionStrategy, Component, input } from '@angular/core';

/**
 * The four numbers of an ended session, as the master reads them: how long it
 * ran, how many combats and scenes there were, and the checks passed outside
 * combat. Two tiles to a line on a phone, one row on a desktop. The ending
 * screen's card and the archived summary's "Em números" draw the same tiles.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-session-stats',
  template: `
    <dl class="stats">
      <div class="stat"><dt>Duração</dt><dd>{{ duration() }}</dd></div>
      <div class="stat"><dt>Combates</dt><dd>{{ combats() }}</dd></div>
      <div class="stat"><dt>Cenas abertas</dt><dd>{{ scenes() }}</dd></div>
      <div class="stat"><dt>Testes passados fora do combate</dt><dd>{{ checks() }}</dd></div>
    </dl>
  `,
  styleUrl: './session-stats.scss',
})
export class SessionStats {
  readonly duration = input.required<string>();
  readonly combats = input.required<number>();
  readonly scenes = input.required<number>();
  /** "9 de 12". */
  readonly checks = input.required<string>();
}
