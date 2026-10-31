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

import { type TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { type CatalogVm } from '../../../core/content/catalog';
import {
  type ClassFeatureDraft,
  classFeaturePaths,
  classFeatureToDraft,
  draftToClassFeature,
  emptyClassFeature,
  unreadableClassFeaturePaths,
} from '../../../core/content/class-feature-draft';
import { type EntryBody, TableContentClient } from '../../../core/content/content-client';
import { EntrySaver, focusAfterRender } from '../../../core/content/entry-saver';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import type { FeatureDraft } from '../../../core/content/feature-draft';
import { previewRead } from '../../../core/content/preview';
import { FeatureEditor } from '../../../shared/feature-editor/feature-editor';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { EditorAlerts, EditorBar } from '../editor-bar/editor-bar';
import { EntryRead } from '../entry-read/entry-read';
import { PlayersSwitch } from '../players-switch/players-switch';
import type { EditorSaved } from '../spell-editor/spell-editor';

/**
 * The table class feature editor (MR-025): a feature the master adds to a class at a class level, which may take the place of a
 * feature the class already has (an SRD one or another of the table's). A name, a text and effects from the server's menu like a
 * feat's, the class (chosen once, read-only after the first save: it is in the entry), the level and the key of the feature it
 * replaces. "Salvar característica" sends the whole entry; whether the class has the replaced feature is the server's to say.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-class-feature-editor',
  imports: [
    EditorAlerts,
    EditorBar,
    EntryRead,
    FeatureEditor,
    MatIconModule,
    PlayersSwitch,
    SelectField,
    TextField,
  ],
  templateUrl: './class-feature-editor.html',
  styleUrl: '../editor.scss',
})
export class ClassFeatureEditor {
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

  protected readonly draft = signal<ClassFeatureDraft>(emptyClassFeature());
  protected readonly saver = new EntrySaver(
    {
      aOne: 'uma característica de classe',
      nameOf: (key) => this.entries().find((e) => e.key === key)?.namePt ?? '',
    },
    'a característica',
  );
  protected readonly isNew = computed(() => this.entry() === null);
  protected readonly classOptions = computed<SelectOption[]>(() => [...this.catalog().classes]);
  protected readonly className = computed(() => this.catalog().nameOf(this.draft().classKey));
  protected readonly spellOptions = computed<SelectOption[]>(() => [...this.catalog().spells]);
  /** What a player reads, written by the same function as the player's page. */
  protected readonly preview = computed(() =>
    previewRead(
      { case: 'tableClassFeature', value: draftToClassFeature(this.draft(), this.menu()) },
      (key) => this.catalog().nameOf(key),
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
          e?.body.case === 'tableClassFeature'
            ? classFeatureToDraft(e.body.value)
            : emptyClassFeature(),
        );
        this.saver.clear();
      });
    });
  }

  protected patch(p: Partial<ClassFeatureDraft>): void {
    this.draft.update((d) => ({ ...d, ...p }));
  }

  protected setFeature(feature: FeatureDraft): void {
    this.patch({ feature });
  }

  protected readonly issuesOf = (path: string): readonly string[] => this.saver.issues(path);

  private readonly known = (path: string): boolean =>
    classFeaturePaths(this.draft(), this.menu()).includes(path);

  protected async save(): Promise<void> {
    if (this.saver.saving() || this.saveBlocked()) {
      return;
    }
    if (
      this.saver.blockOn(unreadableClassFeaturePaths(this.draft(), this.menu()), (path) =>
        focusAfterRender(this.host.nativeElement, this.injector, path),
      )
    ) {
      return;
    }
    const body: EntryBody = {
      case: 'tableClassFeature',
      value: draftToClassFeature(this.draft(), this.menu()),
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
