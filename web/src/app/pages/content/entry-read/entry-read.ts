import { Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { HIT_VISIBILITY_LABEL, VISIBILITY_PUBLIC_PT } from '../../../core/content/hit-effect-read';
import type { EntryRead as Read } from '../../../core/content/content-read-parts';

/**
 * An entry read in full (E10-01 state 9): the numbers in a panel of rows (dado de vida, deslocamento, habilidades...), then the
 * traits or features, then the text. A player gets this for every entry that is on; the master gets it for the kinds that
 * have no editor yet, and, as `plain`, inside an editor's preview. Read only: it draws what `readEntry` wrote and nothing
 * else (the page hands it the rows, so the preview and the player's page are one text).
 */
@Component({
  selector: 'app-entry-read',
  imports: [MatIconModule],
  template: `
    @if (read().rows.length > 0) {
      <section [class.mr-panel]="!plain()" aria-label="Dados">
        <dl class="rows">
          @for (r of read().rows; track r.label) {
            <div class="rows__row">
              <dt>{{ r.label }}</dt>
              <dd>{{ r.value }}</dd>
            </div>
          }
        </dl>
        @if (read().rowsNote) {
          <p class="note"><mat-icon aria-hidden="true">info</mat-icon>{{ read().rowsNote }}</p>
        }
      </section>
    }
    @for (s of read().sections; track s.title) {
      <section [class.mr-panel]="!plain()" [class.sec]="plain()" [attr.aria-labelledby]="s.title ? 'sec-' + $index : null" [attr.aria-label]="s.title ? null : s.label">
        @if (s.title) {
          <h2 [class.mr-panel__title]="!plain()" [class.sec__title]="plain()" [id]="'sec-' + $index">{{ s.title }}</h2>
        }
        <ul class="items">
          @for (i of s.items; track $index) {
            <li>
              <strong>{{ i.title }}.</strong>
              @if (i.tag) {
                <span class="tag"> {{ i.tag }}.</span>
              }
              {{ i.text }}
              @if (i.options) {
                <ul class="items items--nested">
                  @for (o of i.options; track $index) {
                    <li><strong>{{ o.title }}.</strong> {{ o.text }}@if (o.asks) { <span class="asks"> {{ o.asks }}.</span> }</li>
                  }
                </ul>
              }
            </li>
          }
        </ul>
        @if (s.note) {
          <p class="note note--plain">{{ s.note }}</p>
        }
      </section>
    }
    @if (read().text.length > 0 || (read().effects?.length ?? 0) > 0 || rows() || read().hit) {
      <section [class.mr-panel]="!plain()" [class.sec]="plain()" aria-label="Texto">
        @for (p of read().text; track $index) {
          <p class="text">{{ p }}</p>
        }
        @for (i of read().effects ?? []; track $index) {
          <p class="text"><strong>{{ i.title }}:</strong> {{ i.text }}</p>
        }
        @if (read().hit; as hit) {
          @if (plain()) {
            <p class="text hit-line">
              <strong>{{ hit.lead }}</strong>{{ ' ' }}<span class="chip">{{ hit.damage }}</span><span>{{ (hit.more ? ', ' + hit.more : '') + (hit.whileLasts ? ', enquanto durar' : '') + '.' }}</span>
              @for (p of hit.save; track $index) {
                <span>{{ ($first ? ' ' : '') + p.text }}</span>
              }
            </p>
          } @else {
            <section class="hit" aria-labelledby="hit-title">
              <h2 class="opts__title" id="hit-title">{{ hit.title }}</h2>
              <p class="hit__where">{{ hit.where }}</p>
              <p class="hit__chips">
                <span class="chip">{{ hit.damage }}</span>
                @if (hit.more) {
                  <span class="chip">{{ hit.more }}</span>
                }
              </p>
              @if (hit.save.length > 0) {
                <p class="text">
                  @for (p of hit.save; track $index) {
                    @if (p.strong) {
                      <strong>{{ p.text }}</strong>
                    } @else {
                      {{ p.text }}
                    }
                  }
                </p>
              }
              <dl class="hit__who">
                <dt><mat-icon aria-hidden="true">{{ visibilityIcon(hit.visibility) }}</mat-icon>{{ visibilityLabel }}</dt>
                <dd>{{ hit.visibility }}</dd>
              </dl>
            </section>
          }
        }
        @if (rows()) {
          <h2 class="opts__title" id="opts-title">Opções</h2>
          <ul class="opts" aria-labelledby="opts-title">
            @for (o of read().options ?? []; track $index) {
              <li>
                <strong>{{ o.title }}</strong>
                <span class="opts__side">{{ o.side }}</span>
              </li>
            }
          </ul>
        }
      </section>
    }
    @if (!rows() && (read().options?.length ?? 0) > 0) {
      <ul class="items items--nested" aria-label="Opções">
        @for (o of read().options ?? []; track $index) {
          <li><strong>{{ o.title }}.</strong> {{ o.text }}@if (o.asks) { <span class="asks"> {{ o.asks }}.</span> }</li>
        }
      </ul>
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-4);
      min-width: 0;
    }

    .rows {
      margin: 0;
    }

    .rows__row {
      display: flex;
      justify-content: space-between;
      gap: var(--mr-space-4);
      padding: 8px 0;
      border-top: 1px dashed var(--mr-rule);

      &:first-child {
        border-top: 0;
      }

      dt {
        flex: none;
        color: var(--mr-ink-muted);
      }

      dd {
        margin: 0;
        text-align: right;
        overflow-wrap: anywhere;
      }
    }

    // Inside a preview: no panel of its own, a rule above each part.
    .sec {
      padding-top: var(--mr-space-3);
      border-top: 1px solid var(--mr-rule);
    }

    .sec__title {
      margin: 0 0 var(--mr-space-2);
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-19);
      font-weight: 700;
    }

    .items {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      margin: 0;
      padding: 0;
      list-style: none;
      overflow-wrap: anywhere;
    }

    .note {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-2);
      margin: var(--mr-space-3) 0 0;
      font-size: var(--mr-font-15);
      color: var(--mr-ink-muted);

      .mat-icon {
        flex: none;
        width: 20px;
        height: 20px;
        font-size: var(--mr-font-20);
      }

      &--plain {
        display: block;
        margin-top: var(--mr-space-3);
      }
    }

    // "Pede Guerreiro 7 e Golpe do Corvo.": what an option asks, after its text, muted.
    .asks {
      color: var(--mr-ink-muted);
    }

    // "(de Corujeiro)": where a trait of the race comes from, smaller and muted.
    .tag {
      font-size: var(--mr-font-14);
      color: var(--mr-ink-muted);
    }

    // The options of a trait that is a choice of one: a rule on the left and no bullets.
    .items--nested {
      gap: var(--mr-space-2);
      margin-top: var(--mr-space-2);
      padding-left: var(--mr-space-3);
      border-left: 2px solid var(--mr-rule);
    }

    // A feat's options in the page: the name at the left, what it does at the right, one row each.
    .opts__title {
      margin: var(--mr-space-3) 0 var(--mr-space-2);
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-19);
      font-weight: 700;
    }

    .opts {
      margin: 0;
      padding: 0;
      list-style: none;

      li {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: var(--mr-space-4);
        padding: var(--mr-space-3) 0;
        border-top: 1px solid var(--mr-rule);
        overflow-wrap: anywhere;

        &:first-child {
          border-top: 0;
        }
      }
    }

    .opts__side {
      color: var(--mr-ink-muted);
      font-size: var(--mr-font-14);
      text-align: right;
    }

    // "Efeito no próximo golpe": the green chips of the extra damage, and who sees the effect in a grey box.
    .chip {
      display: inline-block;
      padding: 2px 10px;
      border: 1px solid var(--mr-success-ink);
      border-radius: 999px;
      background: var(--mr-success-surface);
      color: var(--mr-success-ink);
      font-size: var(--mr-font-14);
      font-weight: 700;
    }

    .hit__where {
      margin: 0 0 var(--mr-space-2);
      font-size: var(--mr-font-14);
      color: var(--mr-ink-muted);
    }

    .hit__chips {
      display: flex;
      flex-wrap: wrap;
      gap: var(--mr-space-2);
      margin: 0 0 var(--mr-space-3);
    }

    .hit__who {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--mr-space-4);
      margin: var(--mr-space-3) 0 0;
      padding: var(--mr-space-3);
      border-radius: 8px;
      background: var(--mr-ground);
      font-size: var(--mr-font-14);

      dt {
        display: flex;
        flex: none;
        white-space: nowrap;
        align-items: center;
        gap: var(--mr-space-2);
        color: var(--mr-ink-muted);
      }

      dd {
        margin: 0;
        text-align: right;
      }
    }

    .text {
      margin: 0 0 var(--mr-space-3);
      overflow-wrap: anywhere;

      &:last-child {
        margin-bottom: 0;
      }
    }
  `,
})
export class EntryRead {
  readonly read = input.required<Read>();
  /** The options as rows (name and what it does) when the page has a few words for each; the preview keeps the full texts. */
  protected readonly rows = computed(
    () =>
      !this.plain() &&
      (this.read().options?.length ?? 0) > 0 &&
      (this.read().options ?? []).every((o) => (o.side ?? '') !== ''),
  );
  /** No panels: the rows and parts one under another (an editor's preview). */
  readonly plain = input(false);
  protected readonly visibilityLabel = HIT_VISIBILITY_LABEL;
  /** An open eye when the whole table sees the effect, a crossed one when only the caster and the master do. */
  protected visibilityIcon(visibility: string): string {
    return visibility === VISIBILITY_PUBLIC_PT ? 'visibility' : 'visibility_off';
  }
}
