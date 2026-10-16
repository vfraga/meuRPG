import { ChangeDetectionStrategy, Component, computed, input, model, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import type { LuckyOffer } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { LUCKY_TITLE, luckyWhat } from '../../../../core/combat/lucky';
import type { LuckyRoll } from '../../../../core/resources/resources-client';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';

/** Each prompt's own heading id: a sheet and the session panel's Lucky question may both be on the page. */
let nextId = 0;

/**
 * "Tirou 1: rolar de novo?": the question a held roll asks when a halfling's d20 shows a 1 (Sortudo, SRD 5.1), after
 * the d20 and before the result, in the same place and shape as the Bardic Inspiration question (`InspirationPrompt`).
 * The player rolls that d20 again and keeps the new result, whatever it is ("Rolar de novo": rolled in the app, or, with
 * real dice, the typed face), or keeps the 1 ("Ficar com o 1"). With advantage or disadvantage only the die that showed
 * the 1 is rolled again. It says nothing about whether the roll hit: the master has not said yet.
 */
@Component({
  selector: 'app-lucky-prompt',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, RollPicker],
  template: `
    <section class="ask" [attr.aria-labelledby]="titleId">
      <h3 class="ask__title" [id]="titleId">{{ title }}</h3>
      <p class="ask__what">{{ what() }}</p>
      <app-roll-picker
        [canApp]="canApp()"
        [canType]="canType()"
        [preferApp]="preferApp()"
        label="Role 1d20 de novo"
        hint="Role o d20 de novo e digite o número que saiu (1 a 20)."
        [min]="1"
        [max]="20"
        appLabel="Rolar de novo"
        [hideTotal]="true"
        [busy]="busy()"
        [sticky]="false"
        [(typing)]="typing"
        (app)="reroll.emit({ inApp: true })"
        (typed)="reroll.emit({ face: $event })"
      />
      <button matButton="outlined" type="button" class="ask__keep" [disabled]="busy()" (click)="keep.emit()">
        Ficar com o 1
      </button>
    </section>
  `,
  styles: `
    :host {
      display: block;
    }

    .ask {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      padding: var(--mr-space-3);
      border: 2px solid var(--mr-accent);
      border-radius: var(--mr-radius-md);
    }

    .ask__title {
      margin: 0;
      font-size: 17px;
      line-height: 23px;
      font-weight: 800;
    }

    .ask__what {
      margin: 0;
      font-size: 15px;
      line-height: 20px;
    }

    .ask__keep {
      --mat-button-outlined-container-height: 48px;
      --mat-button-outlined-label-text-color: var(--mr-ink);
      width: 100%;
    }
  `,
})
export class LuckyPrompt {
  /** The question: the d20 with the die that shows the 1. */
  readonly offer = input.required<LuckyOffer | null | undefined>();
  readonly canApp = input(true);
  readonly canType = input(true);
  readonly preferApp = input(true);
  readonly busy = input(false);
  /** Typing mode, two-way: the sheet reads it to change its title. */
  readonly typing = model(false);
  /** The player rolls the d20 again: rolled by the app, or the face of the real die. */
  readonly reroll = output<LuckyRoll>();
  /** The player keeps the 1. */
  readonly keep = output<void>();

  protected readonly title = LUCKY_TITLE;
  protected readonly titleId = `lucky-t-${nextId++}`;
  protected readonly what = computed(() => luckyWhat(this.offer()));
}
