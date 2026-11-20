import { ChangeDetectionStrategy, Component, computed, input, signal, output } from '@angular/core';

import { MatIconModule } from '@angular/material/icon';

import type { SummonSpellOptions } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  steedFixedLine,
  steedHowLines,
  steedSameLine,
  steedTypes,
} from '../../core/creatures/steed';
import type { Creature } from '../../../gen/meurpg/rules/v1/rules_pb';
import { CreatureArt } from '../creatures/creature-art';
import { Fold } from '../fold/fold';
import { Segmented } from '../segmented/segmented';

/**
 * The part of the summon sheet that only Convocar Montaria has. On the first cast, under
 * the list of forms (part `rest`): "Tipo do corcel", three segments with the app's words, none chosen until the player
 * picks. With a bonded steed, above the slot (part `same`): one fixed row with the silhouette, tagged "Mesmo corcel", and
 * the line that says what the cast does and where to change the steed. Under either, "Como funciona" folds the spell's
 * notes: closed until it is opened.
 */
@Component({
  selector: 'app-steed-choice',
  imports: [CreatureArt, Fold, MatIconModule, Segmented],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (part() === 'same') {
      @if (spell().bondedSteed; as b) {
        <div class="same">
          <app-creature-art class="same__art" [monsterKey]="b.monsterKey" />
          <span class="same__text">
            <span class="same__head">
              <b class="same__name">{{ b.name }}</b>
              <span class="mr-tag mr-tag--accent"><mat-icon aria-hidden="true">link</mat-icon>Mesmo corcel</span>
            </span>
            <span class="same__sub">{{ fixedLine() }}</span>
          </span>
        </div>
        <p class="same__line">{{ sameLine() }}</p>
      }
    } @else {
      @if (!spell().bondedSteed) {
        <div class="types">
          <span class="types__cap" aria-hidden="true">Tipo do corcel</span>
          <app-segmented label="Tipo do corcel" [segments]="types()" [value]="type()" (choose)="pickType.emit($event)" />
        </div>
      }
      @if (how().length > 0) {
        <app-fold title="Como funciona" appearance="flush" [level]="3" [(open)]="open">
          <ul class="how">
            @for (line of how(); track line) {
              <li>{{ line }}</li>
            }
          </ul>
        </app-fold>
      }
    }
  `,
  styleUrl: './steed-choice.scss',
})
export class SteedChoice {
  readonly spell = input.required<SummonSpellOptions>();
  /** `same`: the bonded steed's fixed row and its line, above the slot. `rest`: the type (first cast) and "Como funciona". */
  readonly part = input<'same' | 'rest'>('rest');
  /** The bonded steed's stat block, for the size and the speed of its row; `null` until it is read. */
  readonly block = input<Creature | null>(null);
  /** The type picked (`SummonChoice.creature_type`); `''` until the player picks. */
  readonly type = input('');
  readonly pickType = output<string>();

  protected readonly types = computed(() => steedTypes(this.spell()));
  protected readonly how = computed(() => steedHowLines(this.spell()));
  /** "Como funciona" starts closed, on every screen. */
  protected readonly open = signal(false);
  protected readonly fixedLine = computed(() => {
    const b = this.spell().bondedSteed;
    return b ? steedFixedLine(b, this.block()) : '';
  });
  protected readonly sameLine = computed(() => steedSameLine(this.spell().bondedSteed?.name ?? ''));
}
