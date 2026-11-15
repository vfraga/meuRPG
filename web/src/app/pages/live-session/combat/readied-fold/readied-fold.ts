import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { EncounterStatus, type Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ContestClient } from '../../../../core/combat/contest-client';
import { ContestState } from '../../../../core/combat/contest-state';
import { fireRows } from '../../../../core/combat/ready-fire';
import { Fold } from '../../../../shared/fold/fold';

/**
 * "Ações preparadas" (#582, board 582-04): the master's fold in "Turno", one row for each readied action that waits, with the trigger
 * quoted and "Disparar" (`FireReadied`: he says the trigger happened, and the readier answers). A row whose readier cannot react now keeps
 * "Disparar" off and says why in its own line, in the order the server checks (`fireRows`); the server still decides, and a refusal after
 * a race reads in the page's own band. The fold shows only while the table holds one. Who is surprised comes from the contest state.
 */
@Component({
  selector: 'app-readied-fold',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [Fold, MatButtonModule, MatIconModule],
  template: `
    @if (rows().length > 0) {
      <app-fold appearance="row" title="Ações preparadas" icon="alarm" [level]="4" [(open)]="open">
        <span foldNote class="count" [attr.aria-label]="rows().length + ' ações preparadas'">{{ rows().length }}</span>
        <p class="hint">Dispare quando o gatilho acontecer; quem preparou decide se usa.</p>
        <ul class="rows">
          @for (row of rows(); track row.id) {
            <li class="row" [attr.data-combatant]="row.combatantId">
              <div class="row__text">
                <p class="row__what">
                  <b>{{ row.who }}</b> · {{ row.held }}
                </p>
                <p class="row__quote">{{ row.quote }}</p>
                @if (row.hidden) {
                  <span class="row__chip"><mat-icon aria-hidden="true">visibility_off</mat-icon>Só você vê</span>
                }
                @if (row.why) {
                  <p class="row__why" [id]="'fire-why-' + row.id">
                    <mat-icon aria-hidden="true">block</mat-icon>{{ row.why }}
                  </p>
                }
              </div>
              <button
                type="button"
                matButton="outlined"
                class="row__fire"
                [class.row__fire--off]="!!row.why"
                [disabled]="!!row.why || busy()"
                disabledInteractive
                [attr.aria-label]="'Disparar a ação de ' + row.who"
                [attr.aria-describedby]="row.why ? 'fire-why-' + row.id : null"
                (click)="go(row.id, !!row.why)"
              >
                <mat-icon aria-hidden="true">bolt</mat-icon>Disparar
              </button>
            </li>
          }
        </ul>
      </app-fold>
    }
  `,
  styleUrl: './readied-fold.scss',
})
export class ReadiedFold {
  private readonly api = inject(ContestClient);
  private readonly contests = new ContestState();

  readonly encounter = input.required<Encounter>();
  readonly campaignId = input.required<string>();
  /** A call of the combat is on its way: nothing fires meanwhile. */
  readonly busy = input(false);
  /** "Disparar": `ReadiedAction.id`. */
  readonly fire = output<string>();

  protected readonly open = signal(true);
  protected readonly rows = computed(() => fireRows(this.encounter(), this.contests.surprised()));

  /** `disabledInteractive` keeps the button focusable, so a press on an off one is dropped here. */
  protected go(id: string, off: boolean): void {
    if (!off && !this.busy()) {
      this.fire.emit(id);
    }
  }

  constructor() {
    // The surprise marks change with the combat: read them again at each revision while a readied action waits.
    effect(() => {
      const e = this.encounter();
      const campaignId = this.campaignId();
      if (e.status !== EncounterStatus.ACTIVE || !e.combatants.some((c) => c.readied)) {
        return;
      }
      void e.revision;
      untracked(() => void this.contests.load(this.api, campaignId, e.id));
    });
  }
}
