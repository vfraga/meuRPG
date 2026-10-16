import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { type RollNote, RollModeKind } from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import type { CheckDie } from '../../../../core/combat/contest-client';
import { modeLabel, noteLines } from '../../../../core/combat/contest-view';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';

/** The two d20 of a roll with advantage or disadvantage, in the order they are rolled. */
const PAIR_FIELDS: readonly RollField[] = [
  { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
  { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
];

/**
 * The two ways to roll the d20 of a contest, of Hide or of a group check (W7-X): "Rolar no app", "Digitar o resultado" and,
 * where the board has it, "Deixar o mestre rolar por mim". A roll with advantage or disadvantage (the server says so with the
 * mode of the check) asks for the two dice when they are typed. The campaign's dice mode and the player's preference choose
 * which way is the filled button, as everywhere else. It sends the roll as `CheckDie`; the sheet makes the call and its key.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-check-roll-form',
  imports: [MultiRoll, RollPicker],
  template: `
    @if (why().length) {
      <ul class="why" aria-label="Como o d20 é rolado">
        @for (line of why(); track $index) {
          <li>{{ line }}</li>
        }
      </ul>
    }
    @if (pair()) {
      <app-multi-roll
        [fields]="fields"
        [canApp]="canApp()"
        [canType]="canType()"
        [preferApp]="preferApp()"
        [combine]="combine()"
        [modifier]="modifier()"
        [totalNote]="totalNote()"
        [appLabel]="appLabel() || 'Rolar 2d20 no app'"
        [hint]="hint()"
        [busy]="busy()"
        [(typing)]="typing"
        (app)="roll.emit({ inApp: true })"
        (typed)="roll.emit({ faces: $event })"
      />
    } @else {
      <app-roll-picker
        [canApp]="canApp()"
        [canType]="canType()"
        [preferApp]="preferApp()"
        [label]="label()"
        hint="Role o seu dado e digite o número que saiu (1 a 20)."
        [totalNote]="totalNote()"
        [modifier]="modifier()"
        [busy]="busy()"
        [sticky]="false"
        [appLabel]="appLabel() || 'Rolar no app'"
        [(typing)]="typing"
        (app)="roll.emit({ inApp: true })"
        (typed)="roll.emit({ faces: [$event] })"
      />
    }
    @if (deferLabel() && !typing()) {
      <button type="button" class="defer" [disabled]="busy()" (click)="defer.emit()">
        {{ deferLabel() }}
      </button>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .why {
      margin: 0 0 var(--mr-space-3);
      padding: 0;
      list-style: none;
      font-size: 14px;
      line-height: 20px;
      color: var(--mr-ink-muted);
    }

    .why li:first-child:not(:only-child) {
      font-weight: 700;
      color: var(--mr-ink);
    }

    .defer {
      display: block;
      width: 100%;
      min-height: 44px;
      margin-top: 4px;
      padding: 0 12px;
      border: 0;
      background: none;
      color: var(--mr-accent-text);
      font: inherit;
      font-weight: 700;
      text-align: center;
      cursor: pointer;
    }

    .defer:focus-visible {
      outline: 3px solid var(--mr-focus);
      outline-offset: 2px;
      border-radius: 8px;
    }

    .defer:disabled {
      color: var(--mr-ink-muted);
      cursor: default;
    }
  `,
})
export class CheckRollForm {
  /** How the d20 rolls: a pair for advantage or disadvantage. */
  readonly mode = input<RollModeKind>(RollModeKind.NORMAL);
  /** What the check adds to the d20. */
  readonly modifier = input(0);
  /** The name of the check, for the typed field's label: "Força (Atletismo)". */
  readonly checkName = input.required<string>();
  /** The circumstances behind the mode ("Ajuda de Orla", "Envenenado"), said above the buttons one sentence each; none: nothing is said. */
  readonly notes = input<readonly RollNote[]>([]);
  readonly totalNote = input('Total do teste');
  readonly diceMode = input.required<DiceMode>();
  readonly preference = input.required<DicePreference>();
  readonly busy = input(false);
  /** The label of the app button when it is not "Rolar no app" (the master's: "Rolar pelo Hobgoblin"). */
  readonly appLabel = input('');
  /** "Deixar o mestre rolar por mim": the link under the buttons; empty for none. */
  readonly deferLabel = input('');

  readonly roll = output<CheckDie>();
  readonly defer = output<void>();

  protected readonly fields = PAIR_FIELDS;
  protected readonly typing = signal(false);
  protected readonly canApp = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);
  protected readonly preferApp = computed(
    () => effectivePreference(this.diceMode(), this.preference()) === DicePreference.APP,
  );
  protected readonly pair = computed(
    () => this.mode() === RollModeKind.ADVANTAGE || this.mode() === RollModeKind.DISADVANTAGE,
  );
  /** "Vantagem" and why, so the player knows the dice they are about to roll are two. */
  protected readonly why = computed(() => {
    const lines = noteLines({ notes: [...this.notes()] });
    const word = modeLabel(this.mode());
    return word || lines.length ? [word, ...lines].filter(Boolean) : [];
  });
  protected readonly combine = computed(() =>
    this.mode() === RollModeKind.DISADVANTAGE ? 'lower' : 'higher',
  );
  protected readonly label = computed(
    () => `Role 1d20 para ${this.checkName()} (${signedText(this.modifier())})`,
  );
  protected readonly hint = computed(
    () =>
      `Role os dois d20 e digite os dois números, na ordem em que saíram (1 a 20 cada). Conta o ${this.combine() === 'lower' ? 'menor' : 'maior'}.`,
  );

  /** Back to the buttons (the sheet changed what is rolled). */
  reset(): void {
    this.typing.set(false);
  }
}

function signedText(n: number): string {
  return n < 0 ? `−${Math.abs(n)}` : `+${n}`;
}
