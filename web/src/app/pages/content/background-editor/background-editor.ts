import {
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

import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { type CatalogVm } from '../../../core/content/catalog';
import { type EntryBody, TableContentClient } from '../../../core/content/content-client';
import { EntrySaver, focusAfterRender } from '../../../core/content/entry-saver';
import { UNREADABLE_COUNT, readWhole, wholeText } from '../../../core/content/whole-number';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import {
  type BackgroundDraft,
  type FeatureDraft,
  backgroundToDraft,
  draftToBackground,
  emptyBackground,
  featurePaths,
  unreadableFeaturePaths,
} from '../../../core/content/feature-draft';
import { FeatureEditor } from '../../../shared/feature-editor/feature-editor';
import { PickList } from '../../../shared/form-fields/pick-list';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { previewRead } from '../../../core/content/preview';
import { EditorAlerts, EditorBar } from '../editor-bar/editor-bar';
import { PlayersSwitch } from '../players-switch/players-switch';
import { EntryRead } from '../entry-read/entry-read';
import type { EditorSaved } from '../spell-editor/spell-editor';

/**
 * The background editor (MR-025, RN-23; E10-01 state 7): the simplest of them. Two skills, tool proficiencies from the
 * SRD's list, how many languages the player chooses, the equipment as a text, and one feature that is a text (it may carry
 * an effect from the server's menu, as a trait does). "Salvar antecedente" sends the whole entry.
 */
@Component({
  selector: 'app-background-editor',
  imports: [
    EditorAlerts,
    EditorBar,
    PlayersSwitch,
    EntryRead,
    FeatureEditor,
    MatIconModule,
    PickList,
    SelectField,
    TextField,
  ],
  templateUrl: './background-editor.html',
  styleUrl: '../editor.scss',
})
export class BackgroundEditor {
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

  protected readonly draft = signal<BackgroundDraft>(emptyBackground());
  protected readonly saver = new EntrySaver(
    {
      aOne: 'um antecedente',
      nameOf: (key) => this.entries().find((e) => e.key === key)?.namePt ?? '',
    },
    'o antecedente',
  );
  protected readonly toolOptions = computed<SelectOption[]>(() =>
    this.menu()
      .list('tools')
      .map((t) => ({ value: t.key, label: t.namePt })),
  );
  protected readonly skillOptions = computed<SelectOption[]>(() => [...this.catalog().skills]);
  protected readonly spellOptions = computed<SelectOption[]>(() => [...this.catalog().spells]);
  /** What a player reads, written by the same function as the player's page. */
  protected readonly preview = computed(() =>
    previewRead(
      { case: 'tableBackground', value: draftToBackground(this.draft(), this.menu()) },
      this.catalog().nameOf,
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
          e?.body.case === 'tableBackground' ? backgroundToDraft(e.body.value) : emptyBackground(),
        );
        this.saver.clear();
      });
    });
  }

  protected patch(p: Partial<BackgroundDraft>): void {
    this.draft.update((d) => ({ ...d, ...p }));
  }

  protected setSkill(i: number, key: string): void {
    const skills = [...this.draft().skills];
    skills[i] = key;
    this.patch({ skills });
  }

  /** What was typed in "Idiomas à escolha" when it is not how the number is written back ("+1", "007", text no number). */
  private readonly countTyped = signal('');

  /** The text of "Idiomas à escolha": what was typed while it still reads as the number, else the number. */
  protected readonly countText = computed(() => {
    const n = this.draft().languageChoices;
    const typed = this.countTyped();
    return typed !== '' && Object.is(readWhole(typed, false), n) ? typed : wholeText(n);
  });

  protected setCount(text: string): void {
    const n = readWhole(text, false);
    this.countTyped.set(text === wholeText(n) ? '' : text);
    this.patch({ languageChoices: n });
  }

  protected setFeature(feature: FeatureDraft): void {
    this.patch({ feature });
  }

  /** The path of "Idiomas à escolha" while its text is no whole number. */
  private readonly unreadableCount = computed<readonly string[]>(() =>
    Number.isNaN(this.draft().languageChoices) ? ['table_background.language_choices'] : [],
  );

  protected readonly issuesOf = (path: string): readonly string[] => [
    ...this.saver.issues(path),
    ...(this.unreadableCount().includes(path) ? [UNREADABLE_COUNT] : []),
  ];

  private readonly known = (path: string): boolean => {
    const p = 'table_background';
    const fixed = [
      `${p}.name_pt`,
      `${p}.skills`,
      `${p}.skills[0]`,
      `${p}.skills[1]`,
      `${p}.tools`,
      `${p}.language_choices`,
      `${p}.equipment_pt`,
    ];
    return (
      fixed.includes(path) ||
      featurePaths(`${p}`, [this.draft().feature], this.menu())
        .map((x) => x.replace(`${p}[0]`, `${p}.feature`))
        .includes(path)
    );
  };

  protected async save(): Promise<void> {
    if (this.saver.saving() || this.saveBlocked()) {
      return;
    }
    const blocked = [
      ...this.unreadableCount(),
      ...unreadableFeaturePaths('table_background.feature', this.draft().feature, this.menu()),
    ];
    if (
      this.saver.blockOn(blocked, (path) =>
        focusAfterRender(this.host.nativeElement, this.injector, path),
      )
    ) {
      return;
    }
    const body: EntryBody = {
      case: 'tableBackground',
      value: draftToBackground(this.draft(), this.menu()),
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
