import { ChangeDetectionStrategy, Component, computed, input, model, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import type { InspirationDie } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  inspirationDieName,
  promptTitle,
  useLabel,
} from '../../../../core/resources/bardic-inspiration';
import type { InspirationRoll } from '../../../../core/resources/resources-client';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';

/**
 * "Usar a Inspiração de Bardo (d8)?" (PM-07c 12, right): the question a held roll asks, after the d20 and before the
 * result. The player rolls the die and adds it to the total now ("Somar o d8 (Inspiração de Orla)": rolled in the app,
 * or, with real dice, the typed face) or keeps it for later ("Guardar o dado": still inside its 10 minutes). It says
 * nothing about whether the roll hit: the master has not said yet. The die is lost once rolled.
 */
@Component({
  selector: 'app-inspiration-prompt',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, RollPicker],
  template: `
    <section class="ask" aria-labelledby="insp-t">
      <h3 class="ask__title" id="insp-t">{{ title() }}</h3>
      <p class="ask__what">
        Você rola o {{ name() }} e <b>soma</b> ao total <b>agora</b>, antes de o mestre dizer o resultado. O dado se
        perde depois de rolado.
      </p>
      <app-roll-picker
        [canApp]="canApp()"
        [canType]="canType()"
        [preferApp]="preferApp()"
        [label]="'Role 1' + name() + ' da Inspiração de Bardo'"
        [hint]="'Role o seu ' + name() + ' e digite o número que saiu (1 a ' + sides() + ').'"
        [min]="1"
        [max]="sides()"
        [appLabel]="label()"
        [hideTotal]="true"
        [busy]="busy()"
        [sticky]="false"
        [(typing)]="typing"
        (app)="use.emit({ inApp: true })"
        (typed)="use.emit({ face: $event })"
      />
      <button matButton="outlined" type="button" class="ask__keep" [disabled]="busy()" (click)="keep.emit()">
        Guardar o dado
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
export class InspirationPrompt {
  readonly die = input.required<InspirationDie | undefined>();
  readonly canApp = input(true);
  readonly canType = input(true);
  readonly preferApp = input(true);
  readonly busy = input(false);
  /** Typing mode, two-way: the sheet reads it to change its title. */
  readonly typing = model(false);
  /** The player adds the die: rolled by the app, or the face of the real die. */
  readonly use = output<InspirationRoll>();
  readonly keep = output<void>();

  protected readonly sides = computed(() => this.die()?.sides ?? 0);
  protected readonly name = computed(() => inspirationDieName(this.sides()));
  protected readonly title = computed(() => promptTitle(this.die()));
  protected readonly label = computed(() => useLabel(this.die()));
}
