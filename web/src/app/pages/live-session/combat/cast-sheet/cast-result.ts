import { Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { CastRow } from '../../../../core/combat/cast-flow';
import { SlotDots } from '../../../../shared/slot-dots/slot-dots';
import { D20Faces } from '../roll-mode/d20-faces';

/** What the cast left on the sheet: the slots as they are now. */
export interface SlotAfter {
  readonly level: number;
  readonly total: number | null;
  readonly used: number;
  /** "Espaços de 1º nível: 0 livres de 4". */
  readonly text: string;
}

/**
 * What the cast did (E6-09, last frame), target by target: the d20 and its
 * word ("Acertou", "Errou", "Crítico"), the target's saving throw ("Falhou",
 * "Resistiu: metade"), the dice of the damage or the heal ("Dardo 1: 1d4 (3) +
 * 1 = 4"), the total in the display type and how the target is now. Under
 * them the slots as they are now, the reminder that Escudo lost its slot, the
 * concentration and what the cast spent. A live region, so the result is read
 * out when it arrives. A target whose damage is still to roll says so.
 */
@Component({
  selector: 'app-cast-result',
  imports: [D20Faces, MatIconModule],
  template: `
    <div class="res" role="status" aria-live="polite">
      @if (sentence()) {
        <p class="sentence">{{ sentence() }}</p>
      }
      @if (pool()) {
        <p class="pool">
          <span class="pool__cap">Sua rolagem</span>
          <span class="pool__roll">{{ pool() }}</span>
        </p>
      }
      @for (r of rows(); track r.id) {
        <section class="tg" [attr.aria-label]="r.label">
          <div class="tg__head">
            <b class="tg__name">{{ r.label }}</b>
            @if (r.word) {
              <span class="pill" [class.pill--bad]="r.tone === 'bad'" [class.pill--neutral]="!!r.icon">
                <mat-icon aria-hidden="true">{{ r.icon ?? (r.tone === 'bad' ? 'close' : 'check') }}</mat-icon>{{ r.word }}{{ r.natural ? ' · ' + r.natural : '' }}
              </span>
            }
            @if (r.state && !r.icon) {
              <span class="state">{{ r.state }}</span>
            }
          </div>
          @for (line of r.lines; track $index) {
            <span class="tg__line">{{ line }}</span>
          }
          @for (d of r.d20s ?? []; track d.caption) {
            <app-d20-faces class="tg__d20" [caption]="d.caption" [roll]="d.roll" [mode]="d.mode" [sources]="d.sources" />
          }
          @if (r.summary) {
            <span class="tg__sum">{{ r.summary }}</span>
          } @else if (r.owed) {
            <span class="tg__line">Falta rolar o dano.</span>
          }
        </section>
      }
      @if (plain()) {
        <p class="plain"><mat-icon aria-hidden="true">auto_awesome</mat-icon>A magia foi conjurada: o mestre resolve o efeito.</p>
      }
    </div>
  `,
  styleUrl: './cast-result.scss',
})
export class CastResult {
  readonly rows = input.required<readonly CastRow[]>();
  /** A spell with no effect the app works out. */
  readonly plain = input(false);
  /** A spell that reads hit points, in one sentence: "O Goblin 1 adormeceu. O Capitão Goblin não foi afetado." */
  readonly sentence = input('');
  /** The caster's own roll of the pool, `5d8 (2, 4, 1, 5, 3) = 15`: never an enemy's hit points. */
  readonly pool = input('');
}

/**
 * What the cast left behind, at the end of the result (E6-09): the slots as they
 * are now, the reminder that Escudo Arcano lost its slot, the concentration and
 * what the cast spent. It comes after the targets and the damage still to roll.
 */
@Component({
  selector: 'app-cast-slots',
  imports: [MatIconModule, SlotDots],
  template: `
    @if (after() || notes().length) {
      <div class="foot" role="status">
        @if (after(); as a) {
          <b class="foot__slots">{{ a.text }}</b>
          @if (a.total !== null) {
            <app-slot-dots [total]="a.total" [used]="a.used" />
          }
        }
        @if (shieldLost()) {
          <span class="foot__note foot__note--off"><mat-icon aria-hidden="true">block</mat-icon>{{ shieldLost() }}</span>
        }
        @for (n of notes(); track n) {
          <span class="foot__note">{{ n }}</span>
        }
      </div>
    }
  `,
  styleUrl: './cast-slots.scss',
})
export class CastSlots {
  readonly after = input<SlotAfter | null>(null);
  /** "Escudo Arcano indisponível: sem espaço de 1º nível." */
  readonly shieldLost = input('');
  /** Concentration and what the cast spent, one line each. */
  readonly notes = input<readonly string[]>([]);
}
