import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import type { AttackDie } from '../../../../core/combat/combat-client';
import { countText } from '../../../../core/combat/death-saves';
import { DeathMarks } from '../death-saves/death-marks';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';

/** What "Rolar o teste contra a morte" asks the page for: whose save, and the d20 (in the app or typed). */
export interface DeathSaveRoll {
  readonly id: string;
  readonly die: AttackDie;
}

/**
 * The master's death save for a player who is not there (RN-03, RN-18): on the
 * turn of a downed character whose save is still due, the card "Teste contra a
 * morte de Brisa" with "Rolar teste contra a morte de Brisa" ("Rolar no app" or
 * "Digitar o resultado", as the table's dice mode and the master's preference
 * say, the same picker the player's own card uses). Without it the character
 * would never stabilise or die when its player is away, because the master's
 * "Próximo turno" does not wait for the save. Once rolled, the card stays with
 * the result line (only the master and the owner ever read it, RN-24). When the
 * master taps "Próximo turno" with the save still due, `asking` swaps in the
 * question in place: "Rolar agora" (safe, with the focus) or "Passar sem rolar".
 */
@Component({
  selector: 'app-death-save-due',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DeathMarks, MatButtonModule, MatIconModule, RollPicker],
  template: `
    @for (c of due(); track c.id) {
      <section #box class="due mr-notice mr-notice--warning" [attr.aria-label]="'Teste contra a morte de ' + c.label" data-testid="death-save-due">
        <mat-icon aria-hidden="true">casino</mat-icon>
        <div class="due__body">
          <h3 class="due__title">Teste contra a morte de {{ c.label }}</h3>
          <div class="due__row">
            <span class="due__name"><b>Sucessos</b><small>{{ count(c.deathSuccesses) }}</small></span>
            <app-death-marks kind="success" [count]="c.deathSuccesses" />
          </div>
          <div class="due__row">
            <span class="due__name"><b>Falhas</b><small>{{ count(c.deathFailures) }}</small></span>
            <app-death-marks kind="failure" [count]="c.deathFailures" />
          </div>
          @if (asking()) {
            <div class="due__ask" role="alertdialog" [attr.aria-label]="'O teste contra a morte de ' + c.label + ' ainda não foi rolado'">
              <p class="due__text"><strong>O teste contra a morte de {{ c.label }} ainda não foi rolado.</strong></p>
              <div class="due__btns">
                <button #safe mat-stroked-button type="button" class="due__btn" (click)="rollNow.emit()">Rolar agora</button>
                <button mat-stroked-button type="button" class="due__btn" [disabled]="busy()" (click)="pass.emit()">Passar sem rolar</button>
              </div>
            </div>
          }
          <app-roll-picker
            [compact]="true"
            [canApp]="canApp()"
            [canType]="canType()"
            [preferApp]="preferApp()"
            [label]="'Role 1d20 para o teste contra a morte de ' + c.label"
            hint="Role o dado e digite o número que saiu (1 a 20)."
            totalNote="Teste contra a morte"
            [busy]="busy()"
            [appLabel]="'Rolar o teste contra a morte de ' + c.label"
            (app)="roll.emit({ id: c.id, die: { inApp: true } })"
            (typed)="roll.emit({ id: c.id, die: { face: $event } })"
          />
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

    .due__btns {
      display: grid;
      gap: 10px;
      grid-template-columns: minmax(0, 1fr);

      @media (min-width: 768px) {
        grid-template-columns: repeat(2, 200px);
      }
    }

    .due__btn {
      --mat-button-outlined-container-height: 48px;
      --mat-button-outlined-label-text-color: var(--mr-ink);
      --mat-button-outlined-outline-color: var(--mr-control-line);
      width: 100%;
      white-space: nowrap;

      @media (min-width: 768px) {
        --mat-button-outlined-container-height: 44px;
      }
    }
  `,
})
export class DeathSaveDue {
  private readonly injector = inject(Injector);

  /** The player characters at 0 hit points whose save is due this turn (`death_save_due`). */
  readonly due = input.required<readonly Combatant[]>();
  readonly busy = input(false);
  readonly diceMode = input.required<DiceMode>();
  readonly dicePreference = input.required<DicePreference>();
  /** The sentence of the save just rolled ("Teste contra a morte: ..."), shown once nothing is due. */
  readonly result = input('');
  /** "Próximo turno" was tapped with a save due: the question takes the card. */
  readonly asking = input(false);

  readonly roll = output<DeathSaveRoll>();
  /** "Rolar agora": the question goes away and the roll is next. */
  readonly rollNow = output<void>();
  /** "Passar sem rolar": the turn ends with the save unrolled. */
  readonly pass = output<void>();

  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });
  private readonly box = viewChild('box', { read: ElementRef<HTMLElement> });

  protected readonly canApp = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);
  protected readonly preferApp = computed(
    () => effectivePreference(this.diceMode(), this.dicePreference()) === DicePreference.APP,
  );

  constructor() {
    // The question opens with the focus on the safe answer, "Rolar agora" (a stray Enter must not skip a death save).
    effect(() => {
      if (this.asking()) {
        afterNextRender(
          () => {
            this.box()?.nativeElement.scrollIntoView({ block: 'start' });
            this.safe()?.nativeElement.focus({ preventScroll: true });
          },
          { injector: this.injector },
        );
      }
    });
  }

  protected count(n: number): string {
    return countText(n);
  }
}
