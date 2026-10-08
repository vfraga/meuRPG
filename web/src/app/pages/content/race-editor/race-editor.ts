import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { type CatalogVm } from '../../../core/content/catalog';
import { type EntryBody, TableContentClient } from '../../../core/content/content-client';
import { EntrySaver, focusField } from '../../../core/content/entry-saver';
import {
  type EffectMenuVm,
  UNREADABLE_RANGE,
  unreadableRange,
} from '../../../core/content/effect-draft';
import { previewRead } from '../../../core/content/preview';
import {
  ABILITY_FIELDS,
  BONUS_MAX,
  BONUS_MIN,
  type FeatureDraft,
  type RaceDraft,
  type SubraceDraft,
  draftToRace,
  draftToSubrace,
  emptyFeature,
  emptyRace,
  featurePaths,
  noBonuses,
  raceToDraft,
  SIZE_OPTIONS,
  subraceToDraft,
} from '../../../core/content/feature-draft';
import { FeatureEditor } from '../../../shared/feature-editor/feature-editor';
import { NumberStepper } from '../../../shared/form-fields/number-stepper';
import { PickList } from '../../../shared/form-fields/pick-list';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { SwitchField } from '../../../shared/form-fields/switch-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { EditorAlerts, EditorBar } from '../editor-bar/editor-bar';
import { PlayersSwitch } from '../players-switch/players-switch';
import { EntryRead } from '../entry-read/entry-read';
import type { EditorSaved } from '../spell-editor/spell-editor';

/** The race and the subrace share one form: a subrace has a name, bonuses and traits, and its race never changes. */
export type RaceMode = 'race' | 'subrace';

/**
 * The race and subrace editor (MR-025, RN-23; E10-01 state 6): size, speed and darkvision in metres, the six ability
 * bonuses with "+2 e +1 à escolha" for the ones the player places, languages from the SRD's list, traits with effects chosen
 * from the server's menu (`app-effect-picker` through `app-feature-editor`), and the race's subraces. One "Salvar raça".
 */
@Component({
  selector: 'app-race-editor',
  imports: [
    EditorAlerts,
    EditorBar,
    PlayersSwitch,
    EntryRead,
    FeatureEditor,
    MatButtonModule,
    MatIconModule,
    NumberStepper,
    PickList,
    RouterLink,
    SelectField,
    SwitchField,
    TextField,
  ],
  templateUrl: './race-editor.html',
  styleUrl: '../editor.scss',
})
export class RaceEditor {
  private readonly client = inject(TableContentClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly mode = input<RaceMode>('race');
  readonly entry = input<TableEntry | null>(null);
  /** A new subrace: the race it belongs to. */
  readonly parentKey = input('');
  readonly menu = input.required<EffectMenuVm>();
  readonly catalog = input.required<CatalogVm>();
  readonly entries = input<readonly TableEntry[]>([]);
  readonly saveBlocked = input('');

  readonly saved = output<EditorSaved>();
  /** The entry's switch "Disponível para os jogadores" was turned (it saves at once, apart from the form). */
  readonly switched = output<TableEntry>();
  readonly reload = output<void>();
  readonly cancelled = output<void>();

  protected readonly race = signal<RaceDraft>(emptyRace());
  protected readonly sub = signal<SubraceDraft>({
    name: '',
    raceKey: '',
    bonuses: noBonuses(),
    traits: [],
  });
  protected readonly saver = new EntrySaver(
    { aOne: 'uma raça', nameOf: (key) => this.entries().find((e) => e.key === key)?.namePt ?? '' },
    'a raça',
  );

  protected readonly isRace = computed(() => this.mode() === 'race');
  protected readonly prefix = computed(() => (this.isRace() ? 'table_race' : 'table_subrace'));
  protected readonly sizes: SelectOption[] = [...SIZE_OPTIONS];
  /** The six abilities, named as the catalog names them. */
  protected readonly abilities = computed(() => this.catalog().abilities);
  protected readonly bonusRange = { min: BONUS_MIN, max: BONUS_MAX };
  /** A new sub-race picks its race from the catalog's (the SRD's and the table's); an existing one keeps it. */
  protected readonly raceOptions = computed<SelectOption[]>(() => [...this.catalog().races]);
  /** The race is fixed: the entry exists, or the page was opened from a race. A new one chooses it in the select, and the select stays once chosen. */
  protected readonly raceKnown = computed(() => this.entry() !== null || this.parentKey() !== '');
  protected readonly languageOptions = computed<SelectOption[]>(() =>
    this.menu()
      .list('languages')
      .map((l) => ({ value: l.key, label: l.namePt })),
  );
  protected readonly spellOptions = computed<SelectOption[]>(() => [...this.catalog().spells]);

  protected readonly bonuses = computed(() =>
    this.isRace() ? this.race().bonuses : this.sub().bonuses,
  );
  protected readonly traits = computed(() =>
    this.isRace() ? this.race().traits : this.sub().traits,
  );
  protected readonly name = computed(() => (this.isRace() ? this.race().name : this.sub().name));
  protected readonly parentName = computed(() => this.catalog().nameOf(this.sub().raceKey));
  protected readonly subraces = computed(() => {
    const e = this.entry();
    return e
      ? this.entries().filter(
          (x) => x.body.case === 'tableSubrace' && x.body.value.raceKey === e.key,
        )
      : [];
  });

  /** "Como os jogadores veem": what a player reads, written by the same function as the player's page. */
  protected readonly preview = computed(() => {
    const body = this.isRace()
      ? ({ case: 'tableRace', value: draftToRace(this.race(), this.menu()) } as const)
      : ({ case: 'tableSubrace', value: draftToSubrace(this.sub(), this.menu()) } as const);
    return previewRead(body, this.catalog().nameOf);
  });

  constructor() {
    // Keyed on the entry's key and revision (and the parent race of a new sub-race), never on the object: archiving hands the page
    // a new object with the same revision, and the master's unsaved draft must stay.
    const source = computed(() => {
      const e = this.entry();
      return `${this.mode()}:${e ? `${e.key}@${e.revision}` : `new:${this.parentKey()}`}`;
    });
    effect(() => {
      source();
      untracked(() => this.start());
    });
  }

  private start(): void {
    {
      const e = this.entry();
      if (this.mode() === 'race') {
        this.race.set(e?.body.case === 'tableRace' ? raceToDraft(e.body.value) : emptyRace());
      } else {
        const base: SubraceDraft =
          e?.body.case === 'tableSubrace'
            ? subraceToDraft(e.body.value)
            : { name: '', raceKey: this.parentKey(), bonuses: noBonuses(), traits: [] };
        this.sub.set(base);
      }
      this.saver.clear();
    }
  }

  protected patchRace(p: Partial<RaceDraft>): void {
    this.race.update((d) => ({ ...d, ...p }));
  }

  protected setName(name: string): void {
    if (this.isRace()) {
      this.patchRace({ name });
    } else {
      this.sub.update((d) => ({ ...d, name }));
    }
  }

  protected setBonus(key: (typeof ABILITY_FIELDS)[number], value: number): void {
    if (this.isRace()) {
      this.race.update((d) => ({ ...d, bonuses: { ...d.bonuses, [key]: value } }));
    } else {
      this.sub.update((d) => ({ ...d, bonuses: { ...d.bonuses, [key]: value } }));
    }
  }

  protected setTraits(traits: FeatureDraft[]): void {
    if (this.isRace()) {
      this.patchRace({ traits });
    } else {
      this.sub.update((d) => ({ ...d, traits }));
    }
  }

  protected setTrait(i: number, t: FeatureDraft): void {
    this.setTraits(this.traits().map((x, k) => (k === i ? t : x)));
  }

  protected removeTrait(i: number): void {
    this.setTraits(this.traits().filter((_, k) => k !== i));
  }

  protected moveTrait(i: number, by: -1 | 1): void {
    const list = [...this.traits()];
    const j = i + by;
    if (j < 0 || j >= list.length) {
      return;
    }
    [list[i], list[j]] = [list[j], list[i]];
    this.setTraits(list);
  }

  protected addTrait(): void {
    this.setTraits([...this.traits(), emptyFeature()]);
    afterNextRender(
      () =>
        focusField(
          this.host.nativeElement,
          `${this.prefix()}.traits[${this.traits().length - 1}].name_pt`,
        ),
      { injector: this.injector },
    );
  }

  protected setCount(text: string): void {
    const n = Number(text.trim());
    this.patchRace({ languageChoices: Number.isInteger(n) && n >= 0 ? n : 0 });
  }

  protected newSubraceLink(): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'new', 'subrace'];
  }

  protected subraceLink(e: TableEntry): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'entries', e.key];
  }

  protected setParent(raceKey: string): void {
    this.sub.update((d) => ({ ...d, raceKey }));
  }

  protected abilityIssues(): readonly string[] {
    return [
      ...this.issuesOf(this.prefix() + '.ability_bonuses'),
      ...ABILITY_FIELDS.flatMap((a) => this.issuesOf(`${this.prefix()}.ability_bonuses.${a}`)),
    ];
  }

  /** The speed and darkvision fields whose text is not a number of metres, by path. */
  private readonly unreadable = computed<readonly string[]>(() => {
    const r = this.race();
    const p = this.prefix();
    return this.isRace()
      ? [
          ...(unreadableRange(r.speedM) ? [`${p}.speed_ft`] : []),
          ...(unreadableRange(r.darkvisionM) ? [`${p}.darkvision_ft`] : []),
        ]
      : [];
  });

  protected readonly issuesOf = (path: string): readonly string[] => [
    ...this.saver.issues(path),
    ...(this.unreadable().includes(path) ? [UNREADABLE_RANGE] : []),
  ];

  private readonly known = (path: string): boolean => {
    const p = this.prefix();
    const fixed = [`${p}.name_pt`, `${p}.traits`];
    for (const a of ABILITY_FIELDS) fixed.push(`${p}.ability_bonuses.${a}`);
    fixed.push(`${p}.ability_bonuses`);
    if (this.isRace()) {
      fixed.push(
        `${p}.size`,
        `${p}.speed_ft`,
        `${p}.darkvision_ft`,
        `${p}.languages`,
        `${p}.language_choices`,
        `${p}.choice_bonuses`,
      );
    } else {
      fixed.push(`${p}.race_key`);
    }
    return (
      fixed.includes(path) || featurePaths(`${p}.traits`, this.traits(), this.menu()).includes(path)
    );
  };

  protected async save(): Promise<void> {
    if (this.saver.saving() || this.saveBlocked() || this.unreadable().length > 0) {
      return;
    }
    const menu = this.menu();
    const body: EntryBody = this.isRace()
      ? { case: 'tableRace', value: draftToRace(this.race(), menu) }
      : { case: 'tableSubrace', value: draftToSubrace(this.sub(), menu) };
    const res = await this.saver.run(
      () => this.client.save(this.campaignId(), this.entry(), body, this.saver.keyFor(body)),
      this.known,
    );
    if (res) {
      this.saved.emit(res);
      return;
    }
    afterNextRender(
      () => {
        const first = this.saver.placement().fields[0];
        if (first) {
          focusField(this.host.nativeElement, first);
        }
      },
      { injector: this.injector },
    );
  }
}
