import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { plainName, type LevelUpSwap } from '../../../core/levelup/levelup-swap';

/**
 * "Trocar escolhas" (MR-025): one panel, in the Ability Score Improvement step or, at a level without it, in the
 * "Escolhas" step. Its groups are the table rule's "Trocar uma escolha no aumento de atributo" (at an Ability Score
 * Improvement level, only where the table plays with it) and the table features that allow a swap at the level. Optional
 * ("Não trocar" is the start): a group, the pick to give up and the pick to take in its place; the choice of each group
 * stays when another is opened. The pool may hold options the sheet does not qualify for yet; the server judges the
 * prerequisite when the level is confirmed.
 */
@Component({
  selector: 'app-swap-picker',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule],
  template: `
    <section class="mr-panel" id="pick-swap" [attr.data-missing]="swap().halfGroups().length > 0 ? '' : null" aria-labelledby="swap-title">
      <h2 class="mr-panel__title" id="swap-title">Trocar escolhas</h2>
      <p class="lead">Neste nível você pode trocar uma escolha de cada conjunto por outra do mesmo conjunto. É opcional.</p>
      <div class="opts" role="radiogroup" aria-labelledby="swap-title">
        <label class="opt opt--none" [class.opt--open]="swap().groupKey() === ''" [class.opt--done]="swap().groupKey() === ''">
          <input type="radio" name="swap-group" class="opt__input" [checked]="swap().groupKey() === ''" (change)="swap().clear()" />
          Não trocar
        </label>
        <div class="groups">
          @for (g of swap().groups(); track g.group) {
            <label
              class="opt"
              [class.opt--open]="swap().groupKey() === g.group"
              [class.opt--done]="swap().isWhole(g.group)"
              [attr.data-half]="swap().halfGroups().includes(g) ? '' : null"
            >
              <input type="radio" name="swap-group" class="opt__input" [checked]="swap().groupKey() === g.group" (change)="swap().setGroup(g.group)" />
              {{ g.namePt }}
              @if (swap().isWhole(g.group)) {
                <span class="opt__hidden"> (troca escolhida)</span>
              } @else if (swap().halfGroups().includes(g)) {
                <span class="opt__hidden"> (troca pela metade)</span>
              }
            </label>
          }
        </div>
      </div>
      @let rows = swap().rows();
      @if (rows.length > 0) {
        <h3 class="sub" id="swap-chosen">Trocas escolhidas</h3>
        <ul class="chosen" aria-labelledby="swap-chosen">
          @for (r of rows; track r.key) {
            <li class="chosen__row">
              <strong>{{ r.label }}:</strong> {{ r.before }} → {{ r.after }}
            </li>
          }
        </ul>
        <p class="chosen__note">Vale ao confirmar o nível.</p>
      }
      @if (swap().group(); as g) {
        <div class="lists">
          <div class="list">
            <h3 class="sub" id="swap-from">Abrir mão de</h3>
            <div class="opts opts--list" role="radiogroup" aria-labelledby="swap-from">
              @for (p of g.picks; track p.key) {
                <label class="opt" [class.opt--open]="swap().fromKey() === p.key" [class.opt--off]="swap().givenUpElsewhere(p.key)">
                  <input
                    type="radio"
                    name="swap-from"
                    class="opt__input"
                    [checked]="swap().fromKey() === p.key"
                    [disabled]="swap().givenUpElsewhere(p.key)"
                    (change)="swap().setFrom(p.key)"
                  />
                  <span class="opt__name">{{ plain(g.namePt, p.namePt) }}</span>
                  @if (swap().givenUpElsewhere(p.key)) {
                    <span class="opt__tag opt__tag--quiet">já sai em outro conjunto</span>
                  } @else if (swap().fromKey() === p.key) {
                    <span class="opt__tag" aria-hidden="true">sai</span>
                  }
                </label>
              }
            </div>
          </div>
          <div class="list">
            <h3 class="sub" id="swap-to">Pegar no lugar</h3>
            <div class="opts opts--list" role="radiogroup" aria-labelledby="swap-to">
              @for (p of g.pool; track p.key) {
                <label class="opt" [class.opt--open]="swap().toKey() === p.key">
                  <input type="radio" name="swap-to" class="opt__input" [checked]="swap().toKey() === p.key" (change)="swap().setTo(p.key)" />
                  <span class="opt__name">{{ plain(g.namePt, p.namePt) }}</span>
                  @if (swap().toKey() === p.key) {
                    <span class="opt__tag" aria-hidden="true">entra</span>
                  }
                </label>
              }
            </div>
          </div>
        </div>
      }
      <div class="status" role="status">
        @let half = swap().halfGroups()[0];
        @if (swap().summary(); as line) {
          <p class="line">{{ line }}</p>
        }
        @if (half) {
          <p class="hint">
            <mat-icon aria-hidden="true">warning</mat-icon>
            <span><strong>Falta escolher.</strong> {{ half.namePt }}: escolha de que abrir mão e o que pegar no lugar. Sem os dois, a troca não vale.</span>
          </p>
        } @else if (swap().group() && !swap().summary()) {
          <p class="hint">
            <mat-icon aria-hidden="true">warning</mat-icon>
            <span><strong>Falta escolher.</strong> Escolha de que abrir mão e o que pegar no lugar. Sem os dois, a troca não vale.</span>
          </p>
        }
      </div>
    </section>
  `,
  styles: `
    // The first missing choice, after a tap on the dashed "Próximo".
    :host section[data-attn] {
      border-color: var(--mr-warning-ink);
      box-shadow: 0 0 0 1px var(--mr-warning-ink);
    }

    .lead {
      margin: 0 0 var(--mr-space-3);
      font-size: var(--mr-font-15);
      line-height: 20px;
      color: var(--mr-ink-muted);
    }

    .sub {
      margin: var(--mr-space-3) 0 var(--mr-space-2);
      font-family: var(--mr-font-sans);
      font-size: var(--mr-font-15);
      font-weight: 700;
    }

    .opts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
      gap: var(--mr-space-2);

      &--list {
        grid-template-columns: 1fr;
      }
    }

    // "Não trocar" has a row of its own; the groups wrap below it, a long name taking the whole row.
    .opts:has(> .groups) {
      display: flex;
      flex-direction: column;
    }

    .groups {
      display: flex;
      flex-wrap: wrap;
      gap: var(--mr-space-2);

      .opt {
        flex: 1 1 auto;
        min-width: min(150px, 100%);
        max-width: 100%;
      }
    }

    .lists {
      display: grid;
      grid-template-columns: 1fr;
      column-gap: var(--mr-space-4);

      @media (min-width: 768px) {
        grid-template-columns: 1fr 1fr;
        align-items: start;
      }
    }

    .list {
      min-width: 0;
    }

    .opt {
      position: relative;
      display: flex;
      align-items: center;
      gap: var(--mr-space-2);
      box-sizing: border-box;
      min-height: 44px;
      padding: var(--mr-space-1) var(--mr-space-3);
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-sm);
      font-family: var(--mr-font-sans);
      font-weight: 700;
      overflow-wrap: anywhere;
      cursor: pointer;

      // The check says one thing: this group has a swap (or "Não trocar" is the choice). It is decoration:
      // the empty alternative keeps it from being read aloud.
      &--done::before {
        content: '✓' / '';
        flex: none;
      }

      // An open group, or a chosen row: the border and the fill alone.
      &--open {
        padding: calc(var(--mr-space-1) - 1px) calc(var(--mr-space-3) - 1px);
        border: 2px solid var(--mr-accent);
        background: var(--mr-accent-soft);
      }

      &--off {
        color: var(--mr-ink-muted);
        cursor: not-allowed;
      }

      &:has(:focus-visible) {
        outline: 2px solid var(--mr-focus);
        outline-offset: 2px;
      }
    }

    .opt__name {
      flex: 1 1 auto;
      min-width: 0;
    }

    .opt__tag {
      flex: none;
      padding: 0 var(--mr-space-2);
      border-radius: var(--mr-radius-sm);
      background: var(--mr-surface);
      color: var(--mr-accent-text);
      font-size: var(--mr-font-13);
      line-height: 20px;

      &--quiet {
        background: none;
        color: var(--mr-ink-muted);
        font-weight: 400;
      }
    }

    .opt__hidden {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }

    .opt__input {
      position: absolute;
      opacity: 0;
      width: 1px;
      height: 1px;
    }

    .chosen {
      margin: 0;
      padding: 0;
      list-style: none;

      &__row {
        padding: var(--mr-space-1) 0;
        font-size: var(--mr-font-17);
      }

      &__note {
        margin: var(--mr-space-1) 0 0;
        font-size: var(--mr-font-15);
        color: var(--mr-ink-muted);
      }
    }

    .line,
    .hint {
      margin: var(--mr-space-3) 0 0;
    }

    .line {
      font-weight: 700;
    }

    .hint {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-2);
      font-size: var(--mr-font-15);
      line-height: 20px;
      color: var(--mr-warning-ink);

      .mat-icon {
        flex: none;
        width: 20px;
        height: 20px;
        font-size: var(--mr-font-20);
      }
    }
  `,
})
export class SwapPicker {
  protected readonly plain = plainName;
  readonly swap = input.required<LevelUpSwap>();
}
