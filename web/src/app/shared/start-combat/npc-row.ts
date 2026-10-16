import { Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { combatantInitial } from '../../core/combat/combat-view';
import { CombatantToken } from '../combatant-token/combatant-token';

/** An NPC's row in "Iniciar combate": how many copies fight (0 is "Fora do
 * combate") and whether they start hidden (question 31: yes, by default). */
@Component({
  selector: 'app-npc-row',
  imports: [CombatantToken, MatIconModule],
  template: `
    <app-combatant-token
      class="tk"
      [initial]="initial()"
      [npc]="true"
      [hidden]="count() > 0 && hidden()"
      [size]="30"
    />
    <span class="text">
      <span class="name">{{ name() }}</span>
      <span class="sub">{{ sub() }}</span>
      @if (count() > 0 && !hidden()) {
        <span class="sub">Você escolheu mostrá-lo aos jogadores desde o início.</span>
      }
    </span>
    <span class="step" role="group" [attr.aria-label]="'Quantos ' + name()">
      <button
        type="button"
        class="btn"
        [class.btn--off]="count() === 0"
        [attr.aria-disabled]="count() === 0"
        [attr.aria-label]="'Menos um ' + name()"
        (click)="count() > 0 && countChange.emit(count() - 1)"
      >
        <mat-icon aria-hidden="true">remove</mat-icon>
      </button>
      <span class="n" role="status" [attr.aria-label]="count() + ' de ' + name()">{{ count() }}</span>
      <button
        type="button"
        class="btn"
        [class.btn--off]="count() >= 10"
        [attr.aria-disabled]="count() >= 10"
        [attr.aria-label]="'Mais um ' + name()"
        (click)="count() < 10 && countChange.emit(count() + 1)"
      >
        <mat-icon aria-hidden="true">add</mat-icon>
      </button>
    </span>
    <span class="hide">
      <button
        type="button"
        role="switch"
        class="track"
        [class.track--on]="hidden() && count() > 0"
        [attr.aria-checked]="hidden()"
        [attr.aria-label]="'Escondido no início: ' + name()"
        [disabled]="count() === 0"
        (click)="hiddenChange.emit(!hidden())"
      >
        <span class="handle">
          @if (hidden() && count() > 0) {
            <mat-icon aria-hidden="true">check</mat-icon>
          }
        </span>
      </button>
      <span class="word">{{ word() }}</span>
    </span>
  `,
  styles: `
    :host {
      display: grid;
      grid-template-columns: 30px minmax(0, 1fr) auto;
      grid-template-areas:
        'tk text step'
        '. hide hide';
      align-items: center;
      gap: 4px 12px;
      min-height: 64px;
      padding: 8px 0;
      border-top: 1px solid var(--mr-rule);

      @media (min-width: 768px) {
        grid-template-columns: 30px minmax(0, 1fr) auto 200px;
        grid-template-areas: 'tk text step hide';
        gap: 12px;
      }
    }

    .tk {
      grid-area: tk;
    }

    .text {
      display: flex;
      grid-area: text;
      flex-direction: column;
      min-width: 0;
    }

    .name {
      font-size: 16px;
      font-weight: 700;
      line-height: 21px;
    }

    .sub {
      font-size: 14px;
      line-height: 19px;
      color: var(--mr-ink-muted);
    }

    .step {
      display: flex;
      grid-area: step;
      align-items: center;
      gap: 8px;
    }

    .btn {
      display: flex;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      width: 44px;
      height: 44px;
      padding: 0;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-sm);
      background: var(--mr-surface);
      color: var(--mr-ink);
      cursor: pointer;
    }

    // At the limit the button is a dashed outline that does nothing.
    .btn--off {
      border-style: dashed;
      color: var(--mr-ink-muted);
      cursor: default;
    }

    .n {
      box-sizing: border-box;
      width: 56px;
      height: 44px;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-sm);
      font-family: var(--mr-font-display);
      font-size: 22px;
      font-weight: 700;
      line-height: 42px;
      text-align: center;
    }

    .hide {
      display: flex;
      grid-area: hide;
      align-items: center;
      gap: 10px;
    }

    // 52 x 28 track with a 44px-tall target around it; ink when on, as in
    // the artboard (the check says it too, so it is not colour alone).
    .track {
      position: relative;
      flex: none;
      box-sizing: border-box;
      width: 52px;
      height: 28px;
      padding: 0;
      border: 2px solid var(--mr-control-line);
      border-radius: var(--mr-radius-pill);
      background: none;
      cursor: pointer;

      &::before {
        content: '';
        position: absolute;
        inset: -8px -2px;
      }

      &:disabled {
        cursor: default;
        opacity: 0.6;
      }
    }

    .track--on {
      border-color: var(--mr-ink);
      background: var(--mr-ink);
    }

    .handle {
      position: absolute;
      top: 50%;
      left: 3px;
      display: flex;
      align-items: center;
      justify-content: center;
      width: 16px;
      height: 16px;
      margin-top: -8px;
      border-radius: var(--mr-radius-pill);
      background: var(--mr-control-line);

      .track--on & {
        left: 24px;
        width: 22px;
        height: 22px;
        margin-top: -11px;
        background: var(--mr-surface);
        color: var(--mr-ink);
      }

      .mat-icon {
        width: 16px;
        height: 16px;
        font-size: 16px;
      }
    }

    .word {
      font-size: 15px;
    }
  `,
})
export class NpcRow {
  readonly name = input.required<string>();
  readonly sub = input('');
  readonly count = input(0);
  readonly hidden = input(true);

  readonly countChange = output<number>();
  readonly hiddenChange = output<boolean>();

  protected readonly initial = computed(() => combatantInitial(this.name()));
  protected readonly word = computed(() =>
    this.count() === 0
      ? 'Fora do combate'
      : this.hidden()
        ? 'Escondido no início'
        : 'À vista no início',
  );
}
