import {
  ChangeDetectionStrategy,
  Component,
  computed,
  input,
  linkedSignal,
  output,
} from '@angular/core';

import type { SummonSpellOptions } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  steedFixedLine,
  steedHowLines,
  steedSameLine,
  steedTypes,
} from '../../core/creatures/steed';
import { Fold } from '../fold/fold';
import { Segmented } from '../segmented/segmented';

/**
 * The part of the summon sheet that only Convocar Montaria has (#588, boards 588-21 and 588-22). On the first cast, under
 * the list of forms: "Tipo do corcel", three segments with the app's words, none chosen until the player picks. With a
 * bonded steed, in place of the list and of the type: one fixed row, tagged "Mesmo corcel", and the line that says what the
 * cast does and where to change the steed. Under either, "Como funciona" folds the spell's notes: closed on a phone, open
 * from the tablet up.
 */
@Component({
  selector: 'app-steed-choice',
  imports: [Fold, Segmented],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (spell().bondedSteed; as b) {
      <div class="same">
        <span class="same__head">
          <b class="same__name">{{ b.name }}</b>
          <span class="mr-tag">Mesmo corcel</span>
        </span>
        <span class="same__sub">{{ fixedLine() }}</span>
      </div>
      <p class="same__line">{{ sameLine() }}</p>
    } @else {
      <div class="types">
        <span class="types__cap" aria-hidden="true">Tipo do corcel</span>
        <app-segmented label="Tipo do corcel" [segments]="types()" [value]="type()" (choose)="pickType.emit($event)" />
      </div>
    }
    @if (how().length > 0) {
      <app-fold title="Como funciona" appearance="row" [level]="3" [(open)]="open">
        <ul class="how">
          @for (line of how(); track line) {
            <li>{{ line }}</li>
          }
        </ul>
      </app-fold>
    }
  `,
  styleUrl: './steed-choice.scss',
})
export class SteedChoice {
  readonly spell = input.required<SummonSpellOptions>();
  /** The type picked (`SummonChoice.creature_type`); `''` until the player picks. */
  readonly type = input('');
  /** A phone's bottom sheet: "Como funciona" starts closed. */
  readonly phone = input(false);
  readonly pickType = output<string>();

  protected readonly types = computed(() => steedTypes(this.spell()));
  protected readonly how = computed(() => steedHowLines(this.spell()));
  protected readonly open = linkedSignal(() => !this.phone());
  protected readonly fixedLine = computed(() => {
    const b = this.spell().bondedSteed;
    return b ? steedFixedLine(b) : '';
  });
  protected readonly sameLine = computed(() => steedSameLine(this.spell().bondedSteed?.name ?? ''));
}
