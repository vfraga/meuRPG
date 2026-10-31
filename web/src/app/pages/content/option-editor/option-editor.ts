import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import {
  type TableEntry,
  TableContentKind,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { type CatalogVm } from '../../../core/content/catalog';
import { type EntryBody, TableContentClient } from '../../../core/content/content-client';
import { EntrySaver, focusAfterRender } from '../../../core/content/entry-saver';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import type { FeatureDraft } from '../../../core/content/feature-draft';
import {
  OFFERERS,
  type OptionDraft,
  draftToOption,
  emptyOption,
  optionPaths,
  optionToDraft,
  unreadableOptionPaths,
} from '../../../core/content/option-draft';
import { optionSetName } from '../../../core/content/option-text';
import { previewRead } from '../../../core/content/preview';
import { FeatureEditor } from '../../../shared/feature-editor/feature-editor';
import { PickList } from '../../../shared/form-fields/pick-list';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { EditorAlerts, EditorBar } from '../editor-bar/editor-bar';
import { EntryRead } from '../entry-read/entry-read';
import { PlayersSwitch } from '../players-switch/players-switch';
import type { EditorSaved } from '../spell-editor/spell-editor';

/**
 * The table option editor (MR-025): an option for one of the SRD's option sets (an invocation, a Metamagic option, a fighting
 * style, a Pact Boon...). A name, a text and effects from the server's menu like a feat's, the set (chosen once, read-only after
 * the first save: it is in the key) and what the option asks of a character (a level in a class, other options, a Pact Boon, a
 * spell). "Salvar opção" sends the whole entry; whether a character qualifies is the server's to say.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-option-editor',
  imports: [
    EditorAlerts,
    EditorBar,
    EntryRead,
    FeatureEditor,
    MatIconModule,
    PickList,
    PlayersSwitch,
    SelectField,
    TextField,
  ],
  templateUrl: './option-editor.html',
  styleUrl: '../editor.scss',
})
export class OptionEditor {
  private readonly client = inject(TableContentClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly entry = input<TableEntry | null>(null);
  readonly menu = input.required<EffectMenuVm>();
  readonly catalog = input.required<CatalogVm>();
  readonly entries = input<readonly TableEntry[]>([]);
  readonly saveBlocked = input('');

  readonly saved = output<EditorSaved>();
  /** The entry's switch "Disponível para os jogadores" was turned (it saves at once, apart from the form). */
  readonly switched = output<TableEntry>();
  readonly reload = output<void>();
  readonly cancelled = output<void>();

  protected readonly draft = signal<OptionDraft>(emptyOption());
  protected readonly saver = new EntrySaver(
    {
      aOne: 'uma opção',
      nameOf: (key) => this.entries().find((e) => e.key === key)?.namePt ?? '',
    },
    'a opção',
  );
  /** The set is chosen on a new entry and shown, not changed, on a saved one. */
  protected readonly isNew = computed(() => this.entry() === null);
  protected readonly setOptions = computed<SelectOption[]>(() =>
    this.menu()
      .list('option_set_names')
      .map((s) => ({ value: s.key, label: s.namePt })),
  );
  protected readonly setHint = computed(
    () =>
      this.menu()
        .list('option_set_names')
        .find((s) => s.key === this.draft().set)?.hintPt ?? '',
  );
  protected readonly setName = computed(() => optionSetName(this.draft().set));
  protected readonly classOptions = computed<SelectOption[]>(() => [
    { value: '', label: 'Nenhuma' },
    ...this.catalog().classes,
  ]);
  protected readonly spellOptions = computed<SelectOption[]>(() => [...this.catalog().spells]);
  protected readonly requireSpellOptions = computed<SelectOption[]>(() => [
    { value: '', label: 'Nenhuma' },
    ...this.catalog().spells,
  ]);
  /** The SRD's options (every set's) and the table's own, but this one: what an option may require, by name. */
  protected readonly requirable = computed<SelectOption[]>(() => {
    const srd = this.srdOptions(Object.values(OFFERERS).flat());
    const own = this.entry()?.key;
    const table = this.tableOptions().filter((o) => o.key !== own);
    return [
      ...srd.map((o) => ({ value: o.key, label: o.namePt })),
      ...table.map((o) => ({ value: o.key, label: `${o.namePt} (da mesa)` })),
    ].sort((a, b) => a.label.localeCompare(b.label, 'pt-BR'));
  });
  protected readonly pactOptions = computed<SelectOption[]>(() => [
    { value: '', label: 'Nenhuma' },
    ...this.srdOptions(OFFERERS['pact_boon']).map((o) => ({ value: o.key, label: o.namePt })),
    ...this.tableOptions()
      .filter((o) => o.body.case === 'tableOption' && o.body.value.set === 'pact_boon')
      .map((o) => ({ value: o.key, label: `${o.namePt} (da mesa)` })),
  ]);
  /** What a player reads, written by the same function as the player's page. */
  protected readonly preview = computed(() =>
    previewRead({ case: 'tableOption', value: draftToOption(this.draft(), this.menu()) }, (key) =>
      this.nameOf(key),
    ),
  );

  constructor() {
    // Keyed on the entry's key and revision, never on the object: archiving hands the page a new object with the same revision,
    // and the master's unsaved draft must stay.
    const source = computed(() => {
      const e = this.entry();
      return e ? `${e.key}@${e.revision}` : 'new';
    });
    effect(() => {
      source();
      untracked(() => {
        const e = this.entry();
        this.draft.set(
          e?.body.case === 'tableOption' ? optionToDraft(e.body.value) : emptyOption(),
        );
        this.saver.clear();
      });
    });
  }

  private tableOptions(): readonly TableEntry[] {
    return this.entries().filter((e) => e.kind === TableContentKind.OPTION && !e.archived);
  }

  private srdOptions(offerers: readonly string[]): { key: string; namePt: string }[] {
    return this.menu()
      .optionSets.filter((s) => offerers.includes(s.key))
      .flatMap((s) => s.options.filter((o) => !o.key.endsWith('@mesa')))
      .map((o) => ({ key: o.key, namePt: o.namePt.replace(/^[^:]+: /, '') }));
  }

  private nameOf(key: string): string {
    const named = this.catalog().nameOf(key);
    if (named !== key) {
      return named;
    }
    const srd = this.srdOptions(Object.values(OFFERERS).flat()).find((o) => o.key === key);
    return srd?.namePt ?? key;
  }

  protected patch(p: Partial<OptionDraft>): void {
    this.draft.update((d) => ({ ...d, ...p }));
  }

  protected setFeature(feature: FeatureDraft): void {
    this.patch({ feature });
  }

  protected readonly issuesOf = (path: string): readonly string[] => this.saver.issues(path);

  private readonly known = (path: string): boolean =>
    optionPaths(this.draft(), this.menu()).includes(path);

  protected async save(): Promise<void> {
    if (this.saver.saving() || this.saveBlocked()) {
      return;
    }
    if (
      this.saver.blockOn(unreadableOptionPaths(this.draft(), this.menu()), (path) =>
        focusAfterRender(this.host.nativeElement, this.injector, path),
      )
    ) {
      return;
    }
    const body: EntryBody = {
      case: 'tableOption',
      value: draftToOption(this.draft(), this.menu()),
    };
    const res = await this.saver.run(
      () => this.client.save(this.campaignId(), this.entry(), body, this.saver.keyFor(body)),
      this.known,
    );
    if (res) {
      this.saved.emit(res);
      return;
    }
    this.saver.focusRefusal(this.host.nativeElement, this.injector);
  }
}
