import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { type AdvantageSource, RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { sourceWord } from '../../../../core/combat/roll-mode';

/**
 * Why the app suggests the mode it does: each circumstance the server lists (`TargetInReach.sources`) tagged "Vantagem" or
 * "Desvantagem", with the server's own sentence beside it. The mode picker draws it above its radios; the window of a readied
 * action (#938) draws it alone, as the one line that says why its d20 is one die or two. Nothing for a roll with no circumstance.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-roll-mode-sources',
  imports: [MatIconModule],
  template: `
    @if (sources().length) {
      <ul class="sources" [attr.aria-label]="label()">
        @for (s of sources(); track $index) {
          <li class="source">
            <span
              class="tag"
              [class.tag--adv]="s.effect === Mode.ADVANTAGE"
              [class.tag--dis]="s.effect === Mode.DISADVANTAGE"
            >
              <mat-icon aria-hidden="true">{{
                s.effect === Mode.ADVANTAGE ? 'arrow_upward' : 'arrow_downward'
              }}</mat-icon
              >{{ sourceWord(s) }}
            </span>
            <span class="source__text">{{ s.textPt }}</span>
          </li>
        }
      </ul>
    }
  `,
  styleUrl: './roll-mode-sources.scss',
})
export class RollModeSources {
  readonly sources = input.required<readonly AdvantageSource[]>();
  /** What a screen reader hears for the list ("Por que o app sugere desvantagem"). */
  readonly label = input('');
  protected readonly Mode = RollMode;
  protected readonly sourceWord = sourceWord;
}
