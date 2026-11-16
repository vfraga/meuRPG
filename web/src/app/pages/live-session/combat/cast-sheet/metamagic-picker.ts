import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { CastTargetRow } from '../../../../core/combat/cast-flow';
import {
  CAREFUL_KEY,
  HEIGHTENED_KEY,
  type MetamagicPicks,
  type MetamagicRow,
  TWINNED_KEY,
} from '../../../../core/resources/metamagic';
import { type Pool } from '../../../../core/resources/pools';
import { type PickRow, ResourcePick } from '../../../../shared/resource-pick/resource-pick';

let nextId = 0;

/**
 * "Metamagia" on the cast sheet (PM-07c 11; SRD 5.1, Sorcerer): only the options the sorcerer knows, each with its cost
 * in sorcery points and its effect in one sentence. An option the spell does not take stays grey and dashed with the
 * server's reason written ("O Raio de Gelo não pede teste de resistência."), and so do the others once one is marked
 * (only one option on a spell, except that Empowered Spell may join another). The options that need creatures ask for
 * them under the list: the second target of Twinned Spell, the creatures Careful Spell protects, the target of
 * Heightened Spell. Real checkboxes: Space marks.
 */
@Component({
  selector: 'app-metamagic-picker',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule, ResourcePick],
  template: `
    <section class="mm" [attr.aria-labelledby]="id + '-t'">
      <h3 class="mm__title" [id]="id + '-t'">
        Metamagia
        <span class="mm__meta">
          · {{ rows().length }} {{ rows().length === 1 ? 'opção conhecida' : 'opções conhecidas' }}
          @if (points(); as p) {
            · Pontos de Feitiçaria: {{ p.left }} de {{ p.total }}
          }
        </span>
      </h3>
      @for (r of rows(); track r.key) {
        <label class="opt" [class.opt--on]="r.checked" [class.opt--off]="!!r.blocked">
          <input
            type="checkbox"
            class="mr-visually-hidden"
            [checked]="r.checked"
            [disabled]="!!r.blocked"
            (change)="flip.emit(r.key)"
          />
          <span class="opt__box" aria-hidden="true">
            @if (r.checked) {
              <mat-icon>check</mat-icon>
            }
          </span>
          <span class="opt__text">
            <span class="opt__head">
              <b>{{ r.name }}</b>
              <span class="opt__cost">{{ r.cost }}</span>
            </span>
            <span class="opt__sub">{{ r.summary }}</span>
            @if (r.blocked) {
              <span class="opt__why"><mat-icon aria-hidden="true">block</mat-icon>{{ r.blocked }}</span>
            }
          </span>
        </label>
      }
      @if (has(twinnedKey)) {
        <app-resource-pick
          label="Segundo alvo da Magia Duplicada"
          [rows]="secondTargets()"
          [chosen]="picks().twinned || null"
          empty="Não há outro alvo no alcance."
          (pick)="picked.emit({ ...picks(), twinned: $event })"
        />
      }
      @if (has(carefulKey)) {
        <fieldset class="careful">
          <legend class="careful__legend">Criaturas que passam sozinhas (Magia Cuidadosa)</legend>
          @for (t of candidates(); track t.id) {
            <label class="check">
              <input
                type="checkbox"
                [checked]="picks().careful.includes(t.id)"
                (change)="picked.emit({ ...picks(), careful: carefulToggled(t.id) })"
              />
              <span>{{ t.label }}</span>
            </label>
          }
        </fieldset>
      }
      @if (has(heightenedKey)) {
        <app-resource-pick
          label="Alvo com desvantagem (Magia Aumentada)"
          [rows]="targetPicks()"
          [chosen]="picks().heightened || null"
          empty="Escolha antes os alvos da magia."
          (pick)="picked.emit({ ...picks(), heightened: $event })"
        />
      }
      @if (line()) {
        <p class="mm__line" role="status" aria-live="polite">{{ line() }}</p>
      }
    </section>
  `,
  styles: `
    :host {
      display: block;
    }

    .mm {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-2);
    }

    .mm__title {
      margin: 0;
      font-size: var(--mr-font-15);
      line-height: 20px;
      font-weight: 700;
    }

    .mm__meta {
      font-weight: 400;
      color: var(--mr-ink-muted);
    }

    .mm__line {
      margin: 0;
      font-size: var(--mr-font-15);
      line-height: 20px;
    }

    .opt {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-3);
      box-sizing: border-box;
      min-height: 56px;
      padding: 8px 14px;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-md);
      cursor: pointer;

      &:has(input:focus-visible) {
        outline: 2px solid var(--mr-focus);
        outline-offset: 2px;
      }

      &--on {
        padding: 7px 13px;
        border: 2px solid var(--mr-accent);
        background: var(--mr-accent-soft);
      }

      &--off {
        border-style: dashed;
        color: var(--mr-ink-muted);
        cursor: default;
      }
    }

    .opt__box {
      flex: none;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      width: 22px;
      height: 22px;
      margin-top: 2px;
      border: 2px solid var(--mr-control-line);
      border-radius: 4px;

      .opt--on & {
        border-color: var(--mr-accent);
        background: var(--mr-accent);
        color: var(--mr-on-accent);
      }

      .mat-icon {
        width: 16px;
        height: 16px;
        font-size: var(--mr-font-16);
      }
    }

    .opt__text {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-width: 0;
    }

    .opt__head {
      display: flex;
      justify-content: space-between;
      gap: var(--mr-space-2);
    }

    .opt__cost {
      flex: none;
      font-size: var(--mr-font-14);
      font-weight: 700;
    }

    .opt__sub {
      font-size: var(--mr-font-14);
      line-height: 19px;
      color: var(--mr-ink-muted);
    }

    .opt__why {
      display: flex;
      align-items: flex-start;
      gap: 4px;
      font-size: var(--mr-font-14);
      line-height: 19px;

      .mat-icon {
        flex: none;
        width: 16px;
        height: 16px;
        margin-top: 1px;
        font-size: var(--mr-font-16);
      }
    }

    .careful {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-1);
      margin: 0;
      padding: 0;
      border: 0;
    }

    .careful__legend {
      margin: 0 0 var(--mr-space-1);
      padding: 0;
      font-size: var(--mr-font-15);
      font-weight: 700;
    }

    .check {
      display: flex;
      align-items: center;
      gap: var(--mr-space-2);
      min-height: 44px;
    }
  `,
})
export class MetamagicPicker {
  readonly rows = input.required<readonly MetamagicRow[]>();
  readonly points = input<Pool | null>(null);
  readonly picks = input.required<MetamagicPicks>();
  /** The creatures of the cast that can be picked (not blocked). */
  readonly candidates = input.required<readonly CastTargetRow[]>();
  /** The first target of the cast: the second one of Twinned Spell is another creature. */
  readonly firstTarget = input<string | null>(null);
  /** The targets already chosen for the spell, for Heightened Spell's target. */
  readonly targetIds = input<readonly string[]>([]);
  readonly line = input('');

  readonly flip = output<string>();
  readonly picked = output<MetamagicPicks>();

  protected readonly id = `metamagic-${nextId++}`;
  protected readonly twinnedKey = TWINNED_KEY;
  protected readonly carefulKey = CAREFUL_KEY;
  protected readonly heightenedKey = HEIGHTENED_KEY;

  protected has(key: string): boolean {
    return this.rows().some((r) => r.key === key && r.checked);
  }

  private pickRow(t: CastTargetRow): PickRow {
    return { id: t.id, title: t.label, sub: t.sub, blocked: t.blocked };
  }

  protected readonly secondTargets = computed<PickRow[]>(() =>
    this.candidates()
      .filter((t) => t.id !== this.firstTarget())
      .map((t) => this.pickRow(t)),
  );
  protected readonly targetPicks = computed<PickRow[]>(() =>
    this.candidates()
      .filter((t) => this.targetIds().includes(t.id))
      .map((t) => this.pickRow(t)),
  );

  protected carefulToggled(id: string): string[] {
    const now = this.picks().careful;
    return now.includes(id) ? now.filter((x) => x !== id) : [...now, id];
  }
}
