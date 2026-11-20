import { Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import {
  type CastTargetRow,
  type Dealt,
  type TargetRule,
  dartsPlaced,
} from '../../../../core/combat/cast-flow';
import { combatantInitial } from '../../../../core/combat/combat-view';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';

let nextId = 0;

/**
 * The target step of the cast sheet (E6-09): who the spell touches. One
 * creature is a radio group, several (an area) a group of checkboxes with the
 * limit said, and Magic Missile hands out its darts, and Raio Ardente its rays
 * (several to one target if the caster wants), with 44px steppers and a live
 * counter ("3 de 3 dardos distribuídos"). A target the spell cannot reach is
 * dashed and disabled, with the reason on its own line.
 */
@Component({
  selector: 'app-cast-targets',
  imports: [CombatantToken, MatIconModule],
  template: `
    <span class="cap" [id]="id + '-l'">{{ caption() }}</span>
    @if (rule().kind === 'darts') {
      <ul class="darts" [attr.aria-labelledby]="id + '-l'">
        @for (t of rows(); track t.id) {
          <li class="dart" [class.dart--off]="t.blocked">
            <app-combatant-token [initial]="initial(t.label)" [npc]="isNpc(t.id) && !isCreature(t.id)" [creature]="isCreature(t.id)" [size]="30" />
            <span class="dart__text">
              <b class="dart__name">{{ t.label }}</b>
              <span class="dart__sub">{{ t.sub }}</span>
              @if (t.blocked) {
                <span class="why"><mat-icon aria-hidden="true">block</mat-icon>{{ t.blocked }}</span>
              }
            </span>
            <span class="step" role="group" [attr.aria-label]="(rays() ? 'Raios em ' : 'Dardos em ') + t.label">
              <button
                type="button"
                class="step__btn"
                [attr.aria-label]="(rays() ? 'Tirar um raio de ' : 'Tirar um dardo de ') + t.label"
                [disabled]="!!t.blocked"
                [attr.aria-disabled]="!t.blocked && (dealt().get(t.id) ?? 0) === 0 ? 'true' : null"
                (click)="step(t.id, -1)"
              >
                <mat-icon aria-hidden="true">remove</mat-icon>
              </button>
              <span class="step__n" aria-hidden="true">{{ dealt().get(t.id) ?? 0 }}</span>
              <button
                type="button"
                class="step__btn"
                [attr.aria-label]="(rays() ? 'Pôr um raio em ' : 'Pôr um dardo em ') + t.label"
                [disabled]="!!t.blocked"
                [attr.aria-disabled]="!t.blocked && placed() >= total() ? 'true' : null"
                (click)="step(t.id, 1)"
              >
                <mat-icon aria-hidden="true">add</mat-icon>
              </button>
            </span>
          </li>
        } @empty {
          <li class="note">Não há ninguém ao alcance.</li>
        }
      </ul>
    } @else {
      <div class="targets" [attr.role]="rule().kind === 'single' ? 'radiogroup' : 'group'" [attr.aria-labelledby]="id + '-l'">
        @for (t of rows(); track t.id) {
          <label class="target" [class.target--off]="t.blocked || (!chosen().includes(t.id) && full())">
            <input
              [type]="rule().kind === 'single' ? 'radio' : 'checkbox'"
              class="mr-visually-hidden"
              [name]="id"
              [checked]="chosen().includes(t.id)"
              [disabled]="!!t.blocked || (!chosen().includes(t.id) && full())"
              (change)="toggle.emit(t.id)"
            />
            <app-combatant-token [initial]="initial(t.label)" [npc]="isNpc(t.id) && !isCreature(t.id)" [creature]="isCreature(t.id)" [size]="30" />
            <span class="target__text">
              <b class="target__name">{{ t.label }}</b>
              <span class="target__sub">{{ t.sub }}</span>
              @if (t.cover) {
                <span class="cover">
                  @if (t.coverMark) {
                    <span class="mr-swatch mr-swatch--sm" [class.mr-swatch--half]="t.coverMark === 'half'" [class.mr-swatch--three]="t.coverMark === 'three'" aria-hidden="true"></span>
                  }
                  {{ t.cover }}
                </span>
              }
              @if (t.blocked) {
                <span class="why"><mat-icon aria-hidden="true">block</mat-icon>{{ t.blocked }}</span>
              }
            </span>
            <!-- The same mark as the slot cards: a check circle when chosen, an empty one when not. -->
            @if (!t.blocked) {
              <mat-icon class="mark" [class.mark--on]="chosen().includes(t.id)" aria-hidden="true">{{ chosen().includes(t.id) ? 'check_circle' : 'radio_button_unchecked' }}</mat-icon>
            }
          </label>
        } @empty {
          <p class="note">Não há ninguém ao alcance.</p>
        }
      </div>
      @if (area()) {
        <p class="note">O mestre confere quem está na área. Você não vê os pontos de vida dos inimigos.</p>
      }
    }
  `,
  styleUrl: './cast-targets.scss',
})
export class CastTargets {
  readonly rows = input.required<readonly CastTargetRow[]>();
  readonly rule = input.required<TargetRule>();
  readonly chosen = input<readonly string[]>([]);
  readonly dealt = input<Dealt>(new Map());
  /** Magic Missile: how many darts the slot makes. */
  readonly total = input(0);
  /** The ids of the NPCs, drawn as squares. */
  readonly npcs = input<ReadonlySet<string>>(new Set());
  /** The targets that are a player's creatures: the round dashed token, not the NPC's rounded square. */
  readonly creatures = input<ReadonlySet<string>>(new Set());
  /** A spell that reads hit points over an area (Sono): the caster says who is in it, by name, and
   * is told why there is no hit point on the list. */
  readonly area = input(false);

  readonly toggle = output<string>();
  readonly deal = output<{ id: string; delta: 1 | -1 }>();

  protected readonly id = `cast-targets-${nextId++}`;
  protected readonly rays = computed(() => this.rule().rays === true);
  protected readonly placed = computed(() => dartsPlaced(this.dealt()));
  protected readonly full = computed(
    () => this.rule().kind === 'multi' && this.chosen().length >= this.rule().max,
  );
  protected readonly caption = computed(() => {
    const r = this.rule();
    switch (r.kind) {
      case 'darts':
        return `Divida os ${this.total()} ${r.rays ? 'raios' : 'dardos'} entre os alvos`;
      case 'single':
        return 'Escolha o alvo (só quem você vê)';
      default:
        if (this.area()) {
          return 'Quem está na área da magia';
        }
        return r.min === 0
          ? 'Quem a magia atinge (pode ser ninguém)'
          : `Escolha até ${r.max} alvos (só quem você vê)`;
    }
  });

  /**
   * A tap on "+" or "−". At a limit (the last dart is placed, none is left to take) the button is `aria-disabled` and does nothing,
   * as the other steppers do: a button that turned natively `disabled` under the finger would drop the keyboard focus to the page.
   * A creature the spell cannot reach has its buttons natively disabled from the start; they never change under the focus.
   */
  protected step(id: string, delta: 1 | -1): void {
    const now = this.dealt().get(id) ?? 0;
    if (delta > 0 ? this.placed() >= this.total() : now === 0) {
      return;
    }
    this.deal.emit({ id, delta });
  }

  protected initial(label: string): string {
    return combatantInitial(label.replace(/ \(você\)$/, ''));
  }

  protected isNpc(id: string): boolean {
    return this.npcs().has(id);
  }

  protected isCreature(id: string): boolean {
    return this.creatures().has(id);
  }
}
