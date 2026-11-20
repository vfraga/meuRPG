import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { MatIconModule } from '@angular/material/icon';

import {
  type TableEntry,
  TableFeatureSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { askKeyNamer } from '../../../core/content/ask-names';
import type { CatalogVm } from '../../../core/content/catalog';
import { featChoiceRows } from '../../../core/content/content-read-feat';
import { type EntryRead as Read, featureItem } from '../../../core/content/content-read-parts';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import type { LevelFeature } from '../../../core/content/class-draft';
import { type FeatureDraft, draftToFeature } from '../../../core/content/feature-draft';
import { featureForPlayers, hiddenFrom } from '../../../core/content/player-view';
import { SwitchesStore } from '../../../core/content/switches-store';
import { requirableOptions } from '../../../core/content/requirable-options';
import { EntryRead } from '../entry-read/entry-read';

/**
 * "Como os jogadores veem" for the open feature of a class or subclass (board 580-03): the name with "Da mesa", the choice it
 * asks ("Escolha 3 de 5 opções") and its options, each with the muted "Pede …" line. It is written by the functions of the
 * players' page (`featureItem`, `featChoiceRows`) from the feature as the save would send it, so an option with no name (not sent)
 * is not here, a class, pact, spell or option that is off or archived is not named (the players' copy drops it), and the sentence of what an option asks is the one `optionAsksPhrase` words for every reader. The master's switches (SRD ones included, and
 * what is hidden through an owner) are read from `ListOptionSwitches`; the summon line is not drawn.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-feature-preview',
  imports: [EntryRead, MatIconModule],
  template: `
    @if (feature(); as f) {
      <section class="mr-panel preview" aria-labelledby="feature-preview-t">
        <h2 class="mr-panel__title" id="feature-preview-t">Como os jogadores veem</h2>
        <p class="preview__name">
          <span>{{ f.name.trim() || 'Nova característica' }}</span>
          <span class="mr-tag"><mat-icon aria-hidden="true">menu_book</mat-icon>Da mesa</span>
        </p>
        @if (unsaved()) {
          <p class="preview__note" role="note">Ainda não salvo: a característica tem um ajuste pendente.</p>
        }
        @if (switchesUnread()) {
          <p class="preview__note" role="note">Não deu para ler quais opções do SRD estão desligadas; a prévia pode citar uma que os jogadores não recebem.</p>
        }
        <app-entry-read [read]="read()" [plain]="true" />
      </section>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .preview {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);

      .mr-panel__title {
        margin: 0;
      }

      &__note {
        margin: 0;
        font-size: var(--mr-font-15);
        line-height: 20px;
        color: var(--mr-ink-muted);
      }

      &__name {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: var(--mr-space-2);
        margin: 0;
        padding-bottom: var(--mr-space-3);
        border-bottom: 1px solid var(--mr-rule);
        font-family: var(--mr-font-display);
        font-size: var(--mr-font-24);
        font-weight: 700;
        line-height: 28px;
        overflow-wrap: anywhere;
      }
    }
  `,
})
export class FeaturePreview {
  /** The features of the class or subclass and the id of the open one: while every feature is closed nothing is drawn. */
  readonly features = input<readonly LevelFeature[]>([]);
  readonly openId = input('');
  readonly menu = input.required<EffectMenuVm>();
  readonly catalog = input<CatalogVm | null>(null);
  readonly entries = input<readonly TableEntry[]>([]);
  /** What the master's switches say the players do not receive (SRD included, and through an owner that is off); the page provides it. */
  private readonly store = inject(SwitchesStore, { optional: true });
  protected readonly switchesUnread = computed(() => this.store?.unread() ?? false);
  /** A refusal stands on the entry: what is read here is the form's, not the saved one. */
  readonly unsaved = input(false);

  protected readonly feature = computed<FeatureDraft | null>(
    () => this.features().find((f) => f.id === this.openId())?.feature ?? null,
  );
  protected readonly read = computed<Read>(() => {
    const f = this.feature();
    if (!f) {
      return { rows: [], sections: [], text: [] };
    }
    const sent = featureForPlayers(
      create(TableFeatureSchema, draftToFeature(f, this.menu())),
      hiddenFrom(this.entries(), this.store?.options() ?? []),
    );
    const nameOf = askKeyNamer(
      f.options,
      this.catalog(),
      requirableOptions(this.menu(), this.entries(), ''),
    );
    const item = featureItem(sent, nameOf);
    return {
      rows: featChoiceRows(sent),
      sections: [],
      text: item.text ? [item.text] : [],
      ...(item.options && item.options.length > 0 ? { options: item.options } : {}),
    };
  });
}
