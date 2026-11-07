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

import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { type CatalogVm } from '../../../core/content/catalog';
import { type EntryBody, TableContentClient } from '../../../core/content/content-client';
import { EntrySaver, focusAfterRender } from '../../../core/content/entry-saver';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import {
  type FeatDraft,
  type Scores,
  draftToFeat,
  emptyFeat,
  featPaths,
  featToDraft,
  unreadableFeatPaths,
} from '../../../core/content/feat-draft';
import {
  LACK_GROUPS,
  lackChips,
  lackOptions,
  pickableLacks,
  pickableRaces,
  raceChips,
} from '../../../core/content/feat-lookup';
import {
  ABILITY_FIELDS,
  type AbilityField,
  type FeatureDraft,
  SIZE_OPTIONS,
} from '../../../core/content/feature-draft';
import { askedRaces } from '../../../core/content/feat-text';
import { previewRead } from '../../../core/content/preview';
import { CheckRow } from '../../../shared/form-fields/check-row';
import { FeatureEditor } from '../../../shared/feature-editor/feature-editor';
import { SearchPick } from '../../../shared/form-fields/search-pick';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { EditorAlerts, EditorBar } from '../editor-bar/editor-bar';
import { EntryRead } from '../entry-read/entry-read';
import { PlayersSwitch } from '../players-switch/players-switch';
import type { EditorSaved } from '../spell-editor/spell-editor';

const RACES = 'table_feat.prerequisite.races';
const LACKS = 'table_feat.prerequisite.lacks_feature_key';

/** The sizes a feat may ask for, as the server writes them. */
const FEAT_SIZES: SelectOption[] = [{ value: '', label: 'Qualquer' }, ...SIZE_OPTIONS];

/**
 * The feat editor: a name, a text, what the feat asks of a character, and its effects.
 * The prerequisite has ability scores, a proficiency, spellcasting, races, a size, a feature to lack and a level.
 * The effects come from the server's menu, with the types only a feat has.
 * "Salvar talento" sends the whole entry.
 * Whether a character meets the prerequisite is the server's to say.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-feat-editor',
  imports: [
    CheckRow,
    EditorAlerts,
    EditorBar,
    EntryRead,
    FeatureEditor,
    MatIconModule,
    PlayersSwitch,
    SearchPick,
    SelectField,
    TextField,
  ],
  templateUrl: './feat-editor.html',
  styleUrl: '../editor.scss',
})
export class FeatEditor {
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

  protected readonly draft = signal<FeatDraft>(emptyFeat());
  protected readonly saver = new EntrySaver(
    {
      aOne: 'um talento',
      nameOf: (key) => this.entries().find((e) => e.key === key)?.namePt ?? '',
    },
    'o talento',
  );
  protected readonly abilities = computed(() => this.catalog().abilities);
  protected readonly spellOptions = computed<SelectOption[]>(() => [...this.catalog().spells]);
  protected readonly proficiencyOptions = computed<SelectOption[]>(() => [
    { value: '', label: 'Nenhuma' },
    ...this.catalog().proficiencies,
  ]);
  protected readonly sizes = FEAT_SIZES;
  protected readonly lackGroups = LACK_GROUPS;
  private readonly lackAll = computed(() =>
    lackOptions(this.menu().list('lackable_features'), this.entries(), this.catalog().nameOf),
  );
  /** The name of any key the preview or a chip writes: the SRD's features are in the menu, not in the catalog. */
  private readonly nameOf = computed(() => {
    const srd = new Map(this.lackAll().map((o) => [o.value, o.label]));
    const catalog = this.catalog();
    return (key: string) => srd.get(key) ?? catalog.nameOf(key);
  });
  /** The races asked: a feat's single race_key stands in the list as one more chip. */
  private readonly asked = computed(() => askedRaces(this.draft().raceKey, this.draft().races));
  /** The key of each race the server refused, with the path of its refusal. */
  private readonly refusedRaces = computed(() => {
    const d = this.draft();
    const out = new Map<string, string>();
    const at = (key: string, path: string) => {
      if (key !== '' && this.saver.issues(path).length > 0) {
        out.set(key, path);
      }
    };
    at(d.raceKey, 'table_feat.prerequisite.race_key');
    d.races.forEach((key, i) => at(key, `${RACES}[${i}]`));
    return out;
  });
  protected readonly raceChipList = computed(() =>
    raceChips(this.asked(), this.entries(), this.nameOf(), this.refusedRaces()),
  );
  protected readonly raceSearch = computed(() =>
    pickableRaces(this.catalog().raceChoices, this.asked()),
  );
  /** Every refusal about the races, each text once: of the list, of one race and of the single race. */
  protected readonly raceIssues = computed(() => {
    const paths = [
      RACES,
      'table_feat.prerequisite.race_key',
      ...this.draft().races.map((_, i) => `${RACES}[${i}]`),
    ];
    return [...new Set(paths.flatMap((p) => this.saver.issues(p)))];
  });
  protected readonly lackSearch = computed(() =>
    pickableLacks(this.lackAll(), this.entry()?.key ?? '', this.draft().lacksFeatureKey),
  );
  protected readonly lackChipList = computed(() =>
    lackChips(
      this.draft().lacksFeatureKey,
      this.lackAll(),
      this.nameOf(),
      this.saver.issues(LACKS).length > 0,
    ),
  );
  /** What a player reads, written by the same function as the player's page. */
  protected readonly preview = computed(() =>
    previewRead(
      { case: 'tableFeat', value: draftToFeat(this.draft(), this.menu()) },
      this.nameOf(),
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
        this.draft.set(e?.body.case === 'tableFeat' ? featToDraft(e.body.value) : emptyFeat());
        this.saver.clear();
      });
    });
  }

  protected patch(p: Partial<FeatDraft>): void {
    this.draft.update((d) => ({ ...d, ...p }));
  }

  protected setFeature(feature: FeatureDraft): void {
    this.patch({ feature });
  }

  /** A race is added to the list; a feat's single race_key moves into it, so the server never gets both. */
  protected addRace(key: string): void {
    const d = this.draft();
    this.patch({ raceKey: '', races: [...askedRaces(d.raceKey, d.races), key] });
  }

  protected removeRace(key: string): void {
    const d = this.draft();
    this.patch(d.raceKey === key ? { raceKey: '' } : { races: d.races.filter((r) => r !== key) });
  }

  protected setScore(which: 'minimums' | 'anyOf', field: AbilityField, text: string): void {
    const scores: Scores = { ...this.draft()[which], [field]: text };
    this.patch({ [which]: scores });
  }

  protected readonly fields = ABILITY_FIELDS;
  protected readonly issuesOf = (path: string): readonly string[] => this.saver.issues(path);

  private readonly known = (path: string): boolean =>
    featPaths(this.draft(), this.menu()).includes(path);

  protected async save(): Promise<void> {
    if (this.saver.saving() || this.saveBlocked()) {
      return;
    }
    if (
      this.saver.blockOn(unreadableFeatPaths(this.draft(), this.menu()), (path) =>
        focusAfterRender(this.host.nativeElement, this.injector, path),
      )
    ) {
      return;
    }
    const body: EntryBody = {
      case: 'tableFeat',
      value: draftToFeat(this.draft(), this.menu()),
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
