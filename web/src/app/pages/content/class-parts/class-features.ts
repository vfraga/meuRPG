import { Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import {
  type LevelFeature,
  newLevelFeatureId,
  sortedFeatures,
} from '../../../core/content/class-draft';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import { type FeatureDraft, emptyFeature } from '../../../core/content/feature-draft';
import { FeatureEditor } from '../../../shared/feature-editor/feature-editor';
import { FieldNote } from '../../../shared/form-fields/field-note';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';

/**
 * The features of a class or a subclass (E10-02 state 3): one row per feature (level, name, the kind of effect in a tag), and the
 * open one is the feature editor of the shared pieces, with its level beside it. One is open at a time. The counter says
 * "5 de 60 características" and turns to the danger ink with an icon past the limit (the server refuses a save over it; nothing
 * here stops the typing). A refusal opens the feature it is about (`openId` is the parent's).
 */
@Component({
  selector: 'app-class-features',
  imports: [FeatureEditor, FieldNote, MatButtonModule, MatIconModule, SelectField],
  template: `
    <div class="head" [attr.data-field]="path() || null" tabindex="-1">
      <h2 class="mr-panel__title" [id]="headingId()">Características</h2>
      <p class="count" [class.count--over]="over()">
        @if (over()) {
          <mat-icon aria-hidden="true">warning</mat-icon>
        }
        {{ features().length }} de {{ max() }} características
      </p>
    </div>
    <app-field-note [issues]="issues()" />
    @if (features().length === 0) {
      <p class="hint">Nenhuma característica ainda. Cada uma tem um efeito escolhido de um menu, ou só o texto.</p>
    }
    <ul class="list">
      @for (f of features(); track f.id; let i = $index) {
        <li class="item" [class.item--open]="openId() === f.id">
          <button
            type="button"
            class="row"
            [attr.aria-expanded]="openId() === f.id"
            [attr.aria-controls]="'feat-' + f.id"
            (click)="openIdChange.emit(openId() === f.id ? '' : f.id)"
          >
            <span class="row__level">Nível {{ f.level }}</span>
            <span class="row__name">{{ f.feature.name.trim() || 'Sem nome' }}</span>
            <span class="mr-tag">{{ effectWord(f.feature) }}</span>
            <mat-icon aria-hidden="true">{{ openId() === f.id ? 'expand_less' : 'expand_more' }}</mat-icon>
          </button>
          @if (openId() === f.id) {
            <div class="open" [id]="'feat-' + f.id">
              <app-feature-editor
                [flat]="true"
                [withLead]="true"
                [allowOptions]="true"
                [allowSummon]="true"
                [campaignId]="campaignId()"
                [entries]="entries()"
                [feature]="f.feature"
                [menu]="menu()"
                [basePath]="baseOf()(i)"
                [issuesOf]="issuesOf()"
                [spellOptions]="spellOptions()"
                nameLabel="Nome da característica"
                removeLabel="Remover característica"
                (featureChange)="setFeature(f.id, $event)"
                (removed)="remove(f.id)"
              >
                <app-select-field lead label="Nível" [options]="levelOptions()" [value]="f.level" (valueChange)="setLevel(f.id, $event)" />
              </app-feature-editor>
            </div>
          }
        </li>
      }
    </ul>
    <button matButton="outlined" type="button" class="add" (click)="add()"><mat-icon aria-hidden="true">add</mat-icon>Adicionar característica</button>
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      min-width: 0;
    }

    .head {
      display: flex;
      flex-wrap: wrap;
      align-items: baseline;
      justify-content: space-between;
      gap: var(--mr-space-2);
    }

    .head .mr-panel__title {
      margin: 0;
    }

    .count {
      display: flex;
      align-items: center;
      gap: 6px;
      margin: 0;
      font-size: var(--mr-font-15);
      color: var(--mr-ink-muted);

      &--over {
        color: var(--mr-danger-ink);
        font-weight: 700;
      }

      .mat-icon {
        width: 18px;
        height: 18px;
        font-size: var(--mr-font-18);
      }
    }

    .hint {
      margin: 0;
      font-size: var(--mr-font-15);
      color: var(--mr-ink-muted);
    }

    .list {
      display: flex;
      flex-direction: column;
      margin: 0;
      padding: 0;
      list-style: none;
    }

    .item + .item {
      border-top: 1px solid var(--mr-rule);
    }

    // The open feature is one frame (the accent contour); its editor inside has no card of its own.
    .item--open {
      margin: var(--mr-space-2) 0;
      padding: var(--mr-space-3);
      border: 2px solid var(--mr-accent);
      border-radius: var(--mr-radius-md);
    }

    .row {
      display: flex;
      align-items: center;
      gap: var(--mr-space-3);
      width: 100%;
      min-height: 56px;
      padding: 0 var(--mr-space-2);
      border: 0;
      background: transparent;
      color: var(--mr-ink);
      font: inherit;
      text-align: left;
      cursor: pointer;

      &:focus-visible {
        outline: 2px solid var(--mr-focus);
        outline-offset: -2px;
      }
    }

    .row__level {
      flex: none;
      min-width: 64px;
      font-size: var(--mr-font-15);
      color: var(--mr-ink-muted);
    }

    .row__name {
      flex: 1 1 auto;
      min-width: 0;
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-19);
      font-weight: 700;
      overflow-wrap: anywhere;
    }

    .open {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      padding-top: var(--mr-space-2);
    }

    .add {
      align-self: flex-start;
      min-height: 44px;
    }
  `,
})
export class ClassFeatures {
  readonly features = input.required<readonly LevelFeature[]>();
  readonly menu = input.required<EffectMenuVm>();
  /** The levels a feature may be at. */
  readonly levels = input.required<readonly number[]>();
  /** The level a new feature starts at. */
  readonly newLevel = input(1);
  readonly max = input(60);
  /** Where feature `i` (its place in `features`) is in the request: "table_class.levels[4].features[0]". */
  readonly baseOf = input.required<(index: number) => string>();
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);
  /** What was refused about the list as a whole (the limit). */
  readonly issues = input<readonly string[]>([]);
  readonly spellOptions = input<readonly SelectOption[]>([]);
  /** The campaign and its entries: the open feature's "Invoca criaturas" searches the bestiary and names the table's creatures. */
  readonly campaignId = input('');
  readonly entries = input<readonly TableEntry[]>([]);
  readonly openId = input('');
  readonly headingId = input('features-t');
  /** The path of the panel head: where "the class has too many features" lands. */
  readonly path = input('');

  readonly featuresChange = output<LevelFeature[]>();
  readonly openIdChange = output<string>();

  protected readonly over = computed(() => this.features().length > this.max());
  protected readonly levelOptions = computed<SelectOption<number>[]>(() =>
    this.levels().map((n) => ({ value: n, label: String(n) })),
  );

  /** "Recurso", "Só texto": what the feature does, in the menu's own word. */
  protected effectWord(f: FeatureDraft): string {
    const first = f.effects[0];
    if (!first) {
      return 'Só texto';
    }
    const word = this.menu().typeOf(first.type)?.namePt ?? first.type;
    return f.effects.length > 1 ? `${word} +${f.effects.length - 1}` : word;
  }

  protected add(): void {
    const id = newLevelFeatureId();
    this.featuresChange.emit(
      sortedFeatures([...this.features(), { id, level: this.newLevel(), feature: emptyFeature() }]),
    );
    this.openIdChange.emit(id);
  }

  protected setFeature(id: string, feature: FeatureDraft): void {
    this.featuresChange.emit(this.features().map((f) => (f.id === id ? { ...f, feature } : f)));
  }

  protected setLevel(id: string, level: number): void {
    this.featuresChange.emit(
      sortedFeatures(this.features().map((f) => (f.id === id ? { ...f, level } : f))),
    );
  }

  protected remove(id: string): void {
    this.featuresChange.emit(this.features().filter((f) => f.id !== id));
    this.openIdChange.emit('');
  }
}
