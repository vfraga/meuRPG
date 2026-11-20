import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, type DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AttackDie } from '../../../../core/combat/combat-client-types';
import { combatantInitial } from '../../../../core/combat/combat-view';
import { countText } from '../../../../core/combat/death-saves';
import { joinDots } from '../../../../core/format/text';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import type { CombatantInfo } from '../combat-info';
import { DeathMarks } from '../death-saves/death-marks';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';

/** What "Rolar o teste contra a morte" asks the page for: whose save, and the d20 (in the app or typed). */
export interface DeathSaveRoll {
  readonly id: string;
  readonly die: AttackDie;
}

/**
 * The turn card of a downed player character on the master's "Turno" (C8-03, RN-03, RN-18): "Toren está com 0 PV: teste contra a
 * morte", before anything else. A downed character's turn starts with its death save (SRD 5.1), so the card shows the successes and
 * failures so far, one line of the rule, "Rolar por Toren" (the app's dice) and "Digitar o resultado" (a physical die), at the same
 * size, as the table's dice mode allows. The turn does not pass until the save is made ("Próximo turno" is off, the bar says why), and
 * there is no "Agir por": the character does only the save. Once rolled, the line of the result stays (only the master and the owner
 * ever read it, RN-24).
 */
@Component({
  selector: 'app-death-save-due',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CombatantToken, DeathMarks, MatButtonModule, MatIconModule, RollPicker],
  template: `
    @for (c of due(); track c.id) {
      <section class="due mr-notice mr-notice--danger" [attr.aria-label]="'Teste contra a morte de ' + c.label" data-testid="death-save-due">
        <div class="due__body">
          <div class="due__head">
            <app-combatant-token [initial]="initial(c)" [current]="true" [size]="44" />
            <div class="due__who">
              <h3 class="due__title">{{ c.label }} está com 0 PV: teste contra a morte</h3>
              <p class="due__sub">{{ sub(c) }}</p>
            </div>
          </div>
          <div class="due__rows">
            <div class="due__row">
              <app-death-marks kind="success" [count]="c.deathSuccesses" />
              <span class="due__name"><b>Sucessos</b><small>{{ count(c.deathSuccesses) }}</small></span>
            </div>
            <div class="due__row">
              <app-death-marks kind="failure" [count]="c.deathFailures" />
              <span class="due__name"><b>Falhas</b><small>{{ count(c.deathFailures) }}</small></span>
            </div>
          </div>
          <p class="due__rule">1d20 sem modificador: 10 ou mais é sucesso; 1 conta duas falhas; 20 levanta {{ c.label }} com 1 PV.</p>
          <div class="due__btns">
            @if (canApp()) {
              <button matButton="filled" type="button" class="due__btn" [disabled]="busy()" disabledInteractive (click)="roll.emit({ id: c.id, die: { inApp: true } })">
                <mat-icon aria-hidden="true">casino</mat-icon>Rolar por {{ c.label }}
              </button>
            }
            @if (canType()) {
              <button matButton="outlined" type="button" class="due__btn" [attr.aria-expanded]="typing()" (click)="typing.set(!typing())">
                <mat-icon aria-hidden="true">edit</mat-icon>Digitar o resultado
              </button>
            }
          </div>
          @if (typing() || !canApp()) {
            <app-roll-picker
              [compact]="true"
              [canApp]="false"
              [canType]="true"
              [typing]="true"
              [label]="'Role 1d20 para o teste contra a morte de ' + c.label"
              hint="Role o dado e digite o número que saiu (1 a 20)."
              totalNote="Teste contra a morte"
              [busy]="busy()"
              (typed)="roll.emit({ id: c.id, die: { face: $event } })"
            />
          }
          <p class="due__note"><mat-icon aria-hidden="true">info</mat-icon>O turno de {{ c.label }} só passa depois do teste.</p>
        </div>
      </section>
    } @empty {
      @if (result()) {
        <p class="due__result mr-notice" role="status" aria-live="polite" data-testid="death-save-result">
          <mat-icon aria-hidden="true">casino</mat-icon>{{ result() }}
        </p>
      }
    }
  `,
  styles: `
    :host {
      display: contents;
    }

    .due {
      align-items: flex-start;
      // Under the sticky app bar when it scrolls into view.
      scroll-margin-top: 80px;
    }

    .due__body {
      display: flex;
      flex: 1;
      flex-direction: column;
      gap: 12px;
      min-width: 0;
    }

    .due__title,
    .due__text,
    .due__result {
      margin: 0;
    }

    .due__title {
      font-size: 1rem;
    }

    .due__row {
      align-items: center;
      display: flex;
      gap: 12px;
      justify-content: space-between;
    }

    .due__name {
      align-items: baseline;
      display: flex;
      gap: 8px;
    }

    .due__head {
      align-items: center;
      display: flex;
      gap: 12px;
    }

    .due__title {
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-22);
      line-height: 26px;
    }

    .due__sub,
    .due__rule,
    .due__note {
      margin: 0;
      font-size: var(--mr-font-14);
      line-height: 19px;
    }

    .due__rows {
      display: flex;
      flex-wrap: wrap;
      gap: 8px 20px;
    }

    .due__note {
      align-items: center;
      display: flex;
      gap: 6px;

      .mat-icon {
        flex: none;
        font-size: var(--mr-font-16);
        height: 16px;
        width: 16px;
      }
    }

    .due__btns {
      display: grid;
      gap: 10px;
      grid-template-columns: minmax(0, 1fr);
    }

    .due__btn {
      --mat-button-filled-container-height: 48px;
      --mat-button-outlined-container-height: 48px;
      --mat-button-outlined-label-text-color: var(--mr-ink);
      --mat-button-outlined-outline-color: var(--mr-control-line);
      width: 100%;
      white-space: nowrap;

      @media (min-width: 768px) {
        --mat-button-filled-container-height: 44px;
        --mat-button-outlined-container-height: 44px;
      }
    }
  `,
})
export class DeathSaveDue {
  /** The player characters at 0 hit points whose save is due this turn (`death_save_due`). */
  readonly due = input.required<readonly Combatant[]>();
  readonly busy = input(false);
  readonly diceMode = input.required<DiceMode>();
  readonly dicePreference = input.required<DicePreference>();
  /** The sentence of the save just rolled ("Teste contra a morte: ..."), shown once nothing is due. */
  readonly result = input('');
  /** "Guerreiro 3" by character id, and the characters' hit points, for the line under the title. */
  readonly info = input<ReadonlyMap<string, CombatantInfo>>(new Map());
  readonly vitals = input<readonly VitalsVm[]>([]);

  readonly roll = output<DeathSaveRoll>();

  protected readonly typing = signal(false);
  protected readonly canApp = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);

  protected count(n: number): string {
    return countText(n);
  }

  protected initial(c: Combatant): string {
    return combatantInitial(c.label);
  }

  /** "Guerreiro 3 · 0 de 28 PV". */
  protected sub(c: Combatant): string {
    const max = this.vitals().find((v) => v.characterId === c.characterId)?.hitPointsMax;
    return joinDots([
      this.info().get(c.characterId)?.classSummary ?? '',
      max === undefined ? '0 PV' : `0 de ${max} PV`,
    ]);
  }
}
