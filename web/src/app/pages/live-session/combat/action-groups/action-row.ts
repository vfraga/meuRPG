import { Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { MASTER_WAIT_NOTE_ID, isMasterWait } from '../../../../core/combat/reactions';
import { EconomyMark } from '../../../../shared/economy-shapes/economy-shapes';
import { SpellHelp } from '../../../../shared/spell-details/spell-help';

/** The economy a row spends, as its mark and word read: the shape of the Action, the Bonus action, the Reaction, or a bolt for what costs nothing. */
export type RowEconomy = 'action' | 'bonus' | 'reaction' | 'free';

const ECONOMY_WORD: Readonly<Record<RowEconomy, string>> = {
  action: 'Ação',
  bonus: 'Bônus',
  reaction: 'Reação',
  free: 'Sem custo',
};

let nextId = 0;

/**
 * One option of "Sua vez" (E6-06): the name, its tags on a line of their own
 * ("Truque", "1º nível", "Reação": the name column is narrow beside the "?"
 * and the button), the detail and, when the option can be used, its button. A
 * spell has the "?" (E8-02) between its text and the button, 44 px, never
 * disabled: the description of a spell that cannot be cast now is still worth
 * reading. A
 * disabled option keeps its place and its focus stop (`aria-disabled`, not
 * `disabled`): the name goes muted, the button dashed, and a block icon with
 * the reason sits on its own line, wired with `aria-describedby`. A row
 * with no `button` (Escudo, a feature waiting for 6.5c) has no button by
 * design: its detail says when it is used.
 */
@Component({
  selector: 'app-action-row',
  imports: [EconomyMark, MatButtonModule, MatIconModule, SpellHelp],
  template: `
    <div class="row__text">
      @if (name()) {
        <span class="row__head">
          <span class="row__name">{{ name() }}</span>
          @if (tags().length || economy()) {
            <span class="row__tags">
              @for (tag of tags(); track tag) {
                <span class="row__pill">{{ tag }}</span>
              }
              @if (economy(); as e) {
                <span class="row__econ"><app-economy-mark [kind]="e" [spent]="off()" />{{ word(e) }}</span>
              }
            </span>
          }
        </span>
      }
      @if (detail()) {
        <span class="row__detail">{{ detail() }}</span>
      }
      @if (note()) {
        <span class="row__detail row__note">{{ note() }}</span>
      }
      @if (rider()) {
        <span class="row__rider" lang="en">{{ rider() }}</span>
      }
      @if (off() && reason() && !masterWait()) {
        <span class="row__why" [id]="whyId"><mat-icon aria-hidden="true">block</mat-icon>{{ reason() }}</span>
      }
    </div>
    @if (helpName()) {
      <app-spell-help [name]="helpName()" (press)="help.emit()" />
    }
    @if (choosing()) {
      <span class="row__choosing">Escolhendo o alvo</span>
    }
    @if (button()) {
      <button
        matButton="outlined"
        type="button"
        class="row__btn"
        [style.display]="choosing() ? 'none' : null"
        [class.row__btn--off]="off()"
        [class.mr-button--off]="off()"
        [disabled]="off() || busy()"
        disabledInteractive
        [attr.aria-label]="buttonLabel()"
        [attr.aria-describedby]="describedBy()"
        (click)="pressed()"
      >
        @if (icon()) {
          <mat-icon aria-hidden="true">{{ icon() }}</mat-icon>
        }
        {{ button() }}
      </button>
    }
  `,
  styleUrl: './action-row.scss',
  host: {
    '[class.row--off]': 'off()',
    '[class.row--help]': '!!helpName()',
    '[class.row--choose]': 'choosing()',
  },
})
export class ActionRow {
  readonly name = input.required<string>();
  /** The small tags under the name: the circle, and "Reação" or "Ação bônus" for a spell of another economy. */
  readonly tags = input<readonly string[]>([]);
  /** The spell's name, when the row has the "?": it names the button ("Detalhes de Sono"). */
  readonly helpName = input('');
  /** The economy the row spends (a mark and its word beside the name's tags); `null` for a row that spends none, or says it elsewhere. */
  readonly economy = input<RowEconomy | null>(null);
  readonly detail = input('');
  /** A short line of why under the detail ("Golpe desarmado das Artes Marciais"). */
  readonly note = input('');
  /** The SRD's own text of a creature's attack (its rider), under the detail; English, as the book has it, and never translated here. */
  readonly rider = input('');
  /** "Atacar", "Conjurar"; empty for a row with no button. */
  readonly button = input('');
  /** The icon beside the button's word (a wand for "Conjurar", swords for "Atacar"); none for a button without one. */
  readonly icon = input('');
  /** The button's accessible name: "Atacar com Raio de Fogo". */
  readonly buttonLabel = input<string | null>(null);
  readonly off = input(false);
  readonly reason = input('');
  readonly busy = input(false);
  /** The player is choosing this action's target on the map (C7-05): the pill takes the button's place. */
  readonly choosing = input(false);

  readonly press = output<void>();
  /** The "?": open the spell's details. */
  readonly help = output<void>();

  protected readonly whyId = `row-why-${nextId++}`;
  /** The wait is the master's: the turn card says it once, and the row points to it instead of repeating it. */
  protected readonly masterWait = computed(() => this.off() && isMasterWait(this.reason()));
  protected readonly describedBy = computed(() => {
    if (!this.off() || !this.reason()) {
      return null;
    }
    return this.masterWait() ? MASTER_WAIT_NOTE_ID : this.whyId;
  });

  protected word(e: RowEconomy): string {
    return ECONOMY_WORD[e];
  }

  /** A disabled option keeps its focus stop but does nothing. */
  protected pressed(): void {
    if (!this.off() && !this.busy()) {
      this.press.emit();
    }
  }
}
