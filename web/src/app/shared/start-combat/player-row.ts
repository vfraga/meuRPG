import { Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { combatantInitial } from '../../core/combat/combat-view';
import { CheckBox } from '../check-box/check-box';
import { CombatantToken } from '../combatant-token/combatant-token';

/** A party member's row in "Iniciar combate": a checkbox, the character, and
 * how its player rolls (RN-18: "No app" or "Meus próprios dados"). */
@Component({
  selector: 'app-player-row',
  imports: [CheckBox, CombatantToken, MatIconModule],
  template: `
    <div class="row">
      <app-check-box
        [checked]="included()"
        [disabled]="!!unavailable()"
        [label]="'Incluir ' + name()"
        (toggle)="toggle.emit()"
      />
      <app-combatant-token [initial]="initial()" [size]="30" />
      <span class="text">
        <span class="name">{{ name() }}</span>
        <span class="sub">{{ sub() }}</span>
        @if (unavailable(); as why) {
          <span class="sub" data-testid="player-unavailable">{{ why }}</span>
        }
      </span>
      @if (!unavailable()) {
        <span class="mr-tag">
          <mat-icon aria-hidden="true">{{ inApp() ? 'casino' : 'person' }}</mat-icon>{{ roll() }}
        </span>
      }
    </div>
  `,
  styles: `
    :host {
      display: block;
      border-top: 1px solid var(--mr-rule);
    }

.row {
  display: grid;
  grid-template-columns: 24px 30px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  min-height: 56px;
  padding: 6px 0;
}

.text {
  display: flex;
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
  `,
})
export class PlayerRow {
  readonly name = input.required<string>();
  readonly sub = input('');
  readonly roll = input('');
  readonly inApp = input(true);
  readonly included = input(false);
  /** Why the character cannot fight ("Reservado: ainda sem jogador"), or empty. */
  readonly unavailable = input('');

  readonly toggle = output<void>();

  protected readonly initial = computed(() => combatantInitial(this.name()));
}
