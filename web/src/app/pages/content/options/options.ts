import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type OptionSwitchEntry,
  TableContentKind,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient, contentErrorText } from '../../../core/content/content-client';
import {
  CONTENT_NAV,
  type ContentNavKind,
  KIND_WORDS,
  navBySlug,
} from '../../../core/content/content-kinds';
import { ContentWatcher } from '../../../core/content/content-watcher';
import {
  OptionSwitchesState,
  counterText,
  groupRows,
  hiddenByParent,
  hiddenCounterText,
  hiddenNote,
  isMasculine,
  mainCount,
  menuCount,
  nestRows,
  searchRows,
  subraceCount,
  subraceCounterText,
  usingText,
} from '../../../core/content/option-switches';
import { spellLevelLabel } from '../../../core/characters/character-labels';
import { SpellsClient } from '../../../core/spells/spells-client';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { SwitchField } from '../../../shared/form-fields/switch-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { mediaQuery, PHONE_QUERY } from '../../../shared/map-view/media-query';
import { LiveSessionSourceLive } from '../../live-session/live-session-source.live';

type Access =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'not-master'; campaignName: string }
  | { status: 'error'; message: string }
  | { status: 'ok'; campaignName: string };

/**
 * "Opções para os jogadores" (MR-025, RN-23; E10-01 state 3): the master's one switch per class, subclass, race, sub-race,
 * background and spell, the SRD's and the table's together, by kind. What is on the players read in full and may choose; what is
 * off never reaches them, and there is no switch per player. Each row says how many characters use the option (turning it off
 * touches no sheet), the group says "Raças: 9 de 10 ligadas" and has "Ligar todas" and "Desligar todas", the search narrows by
 * name, and every change is saved at once ("Tudo salvo"). A subclass or sub-race under an off class or race says that it is
 * hidden for that reason and keeps its own switch. The page reads again when the table changes (`content_changed`).
 */
@Component({
  selector: 'app-content-options',
  imports: [
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
    SelectField,
    SwitchField,
    TextField,
  ],
  providers: [ContentWatcher, LiveSessionSourceLive],
  templateUrl: './options.html',
  styleUrl: './options.scss',
})
export class ContentOptions {
  private readonly campaigns = inject(CampaignsService);
  private readonly client = inject(TableContentClient);
  private readonly route = inject(ActivatedRoute);
  private readonly injector = inject(Injector);
  private readonly watcher = inject(ContentWatcher);
  private readonly spellsClient = inject(SpellsClient);
  private readonly title = viewChild<ElementRef<HTMLElement>>('title');

  protected readonly nav = CONTENT_NAV;
  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly campaignId = signal('');
  protected readonly access = signal<Access>({ status: 'loading' });
  protected readonly slug = signal<ContentNavKind['slug']>('classes');
  protected readonly query = signal('');
  protected readonly kindWords = KIND_WORDS;
  protected readonly Kind = TableContentKind;

  private state: OptionSwitchesState | null = null;
  /** The state is made once the campaign is known; this signal carries its options to the template. */
  private readonly stateSignal = signal<OptionSwitchesState | null>(null);

  protected readonly options = computed(() => this.stateSignal()?.options() ?? []);
  protected readonly status = computed(() => this.stateSignal()?.status() ?? 'loading');
  protected readonly loadError = computed(() => this.stateSignal()?.loadError() ?? '');
  protected readonly save = computed(() => this.stateSignal()?.save() ?? { kind: 'idle' as const });
  protected readonly announce = computed(() => this.stateSignal()?.announce() ?? '');

  protected readonly current = computed(() => navBySlug(this.slug()) ?? CONTENT_NAV[0]);
  protected readonly byKey = computed(() => new Map(this.options().map((o) => [o.key, o])));
  protected readonly rowsAll = computed(() => groupRows(this.options(), this.current()));
  /** The Magias group's filters: the class (its spell list, asked of the server) and the circle. */
  protected readonly classFilter = signal('');
  protected readonly levelFilter = signal('');
  /** Why the class filter was dropped, when its spell list could not be read. */
  protected readonly classError = signal('');
  private readonly classSpells = signal<ReadonlyMap<string, ReadonlySet<string>>>(new Map());
  protected readonly isSpells = computed(() => this.current().slug === 'spells');
  protected readonly classOptions = computed<SelectOption[]>(() => [
    { value: '', label: 'Todas as classes' },
    ...this.options()
      .filter((o) => o.kind === TableContentKind.CLASS)
      .map((o) => ({ value: o.key, label: o.namePt })),
  ]);
  protected readonly levelOptions: SelectOption[] = [
    { value: '', label: 'Todos os níveis' },
    ...Array.from({ length: 10 }, (_, i) => ({ value: String(i), label: spellLevelLabel(i) })),
  ];
  protected readonly rows = computed(() => {
    let rows = searchRows(this.rowsAll(), this.query());
    if (this.isSpells()) {
      const level = this.levelFilter();
      if (level !== '') {
        rows = rows.filter((o) => o.level === Number(level));
      }
      const klass = this.classSpells().get(this.classFilter());
      if (this.classFilter() !== '' && klass) {
        rows = rows.filter((o) => klass.has(o.key));
      }
    }
    return rows;
  });
  /** What the list draws: the races with their sub-races under them, the rest as they come. */
  protected readonly shown = computed(() =>
    this.current().slug === 'races'
      ? nestRows(this.rows())
      : this.rows().map((row) => ({ row, nested: false })),
  );
  protected readonly counter = computed(
    () =>
      counterText(this.current(), mainCount(this.options(), this.current())) +
      (['subclasses', 'races'].includes(this.current().slug)
        ? hiddenCounterText(this.current(), hiddenByParent(this.rowsAll()))
        : ''),
  );
  protected readonly subraces = computed(() => {
    const c = subraceCount(this.options());
    return this.current().slug === 'races' && c.total > 0 ? subraceCounterText(c) : '';
  });
  protected readonly subraceTotal = computed(() => subraceCount(this.options()).total);
  protected readonly menuCounts = computed(
    () => new Map(CONTENT_NAV.map((n) => [n.slug, menuCount(this.options(), n)])),
  );
  protected readonly kindOptions = computed<SelectOption[]>(() =>
    CONTENT_NAV.map((n) => ({
      value: n.slug,
      label: `${n.plural} · ${this.menuCounts().get(n.slug)}`,
    })),
  );
  protected readonly searching = computed(() => this.query().trim() !== '');
  /** "Ligar todas" acts on what the list shows: with a search on, only on the rows found, and the buttons say so. */
  protected readonly narrowed = computed(
    () => this.searching() || this.levelFilter() !== '' || this.classFilter() !== '',
  );
  protected readonly bulkSuffix = computed(() =>
    this.narrowed() ? ` (${this.rows().length})` : '',
  );
  /** The class filter is picked but the class's spell list has not come yet: the rows in view are not narrowed yet. */
  protected readonly classPending = computed(
    () => this.classFilter() !== '' && !this.classSpells().has(this.classFilter()),
  );
  /** "Ligar todas" and "Desligar todas" are off when they would change nothing in the rows in view, and while the class filter loads. */
  protected readonly turnOnDisabled = computed(
    () => this.classPending() || this.rows().every((o) => !o.off),
  );
  protected readonly turnOffDisabled = computed(
    () => this.classPending() || this.rows().every((o) => o.off),
  );
  protected readonly allWord = computed(() =>
    this.current().slug === 'backgrounds' ? 'todos' : 'todas',
  );
  protected readonly onWord = computed(() =>
    this.current().slug === 'backgrounds' ? 'Ligado' : 'Ligada',
  );
  protected readonly offWord = computed(() =>
    this.current().slug === 'backgrounds' ? 'Desligado' : 'Desligada',
  );

  constructor() {
    this.route.paramMap.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((params) => {
      const id = params.get('id');
      if (id) {
        this.campaignId.set(id);
        void this.start(id);
      }
    });
    this.route.queryParamMap.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((params) => {
      const n = navBySlug(params.get('kind'));
      if (n) {
        this.slug.set(n.slug);
        this.query.set('');
      }
    });
    // The table changed (an entry written, a switch turned): the list is read again, without a spinner.
    this.watcher.whileLive(this.campaignId, () => void this.state?.load(true));
  }

  protected async start(id: string): Promise<void> {
    this.access.set({ status: 'loading' });
    try {
      const { campaign } = await this.campaigns.getCampaign(id);
      if (!campaign || campaign.awaitingApproval) {
        this.access.set({ status: 'not-found' });
        return;
      }
      if (campaign.myRole !== Role.MASTER) {
        this.access.set({ status: 'not-master', campaignName: campaign.name });
        return;
      }
      this.state = new OptionSwitchesState(this.client, id, (err) =>
        contentErrorText(err, 'salvar'),
      );
      this.stateSignal.set(this.state);
      this.access.set({ status: 'ok', campaignName: campaign.name });
      await this.state.load();
      afterNextRender(() => this.title()?.nativeElement.focus({ preventScroll: true }), {
        injector: this.injector,
      });
    } catch (err) {
      this.access.set(
        ConnectError.from(err, Code.Unavailable).code === Code.NotFound
          ? { status: 'not-found' }
          : { status: 'error', message: contentErrorText(err, 'abrir as opções') },
      );
    }
  }

  protected retry(): void {
    const id = this.campaignId();
    if (this.access().status === 'ok') {
      void this.state?.load();
    } else {
      void this.start(id);
    }
  }

  protected back(): string[] {
    return ['/campaigns', this.campaignId(), 'content'];
  }

  protected setQuery(text: string): void {
    this.query.set(text);
  }

  protected pickKind(slug: string): void {
    const n = navBySlug(slug);
    if (n) {
      this.slug.set(n.slug);
      this.query.set('');
      this.classFilter.set('');
      this.classError.set('');
      this.levelFilter.set('');
    }
  }

  protected toggle(o: OptionSwitchEntry, on: boolean): void {
    void this.state?.toggle(o.key, !on);
  }

  protected setAll(off: boolean): void {
    if (this.classPending()) {
      return;
    }
    void this.state?.setAll(this.rows(), off, this.current().plural);
  }

  protected using(o: OptionSwitchEntry): string {
    return usingText(o.charactersUsing, o.off);
  }

  protected hidden(o: OptionSwitchEntry): boolean {
    return hiddenNote(o, this.byKey()) !== '';
  }

  protected setClass(key: string): void {
    this.classFilter.set(key);
    this.classError.set('');
    if (key && !this.classSpells().has(key)) {
      // The class's own spell list, as the server serves it to the master (the SRD's and the table's spells alike).
      void this.spellsClient
        .list({ campaignId: this.campaignId(), classKey: key, pageSize: 400 })
        .then(
          (res) =>
            this.classSpells.update((m) =>
              new Map(m).set(key, new Set(res.spells.map((s) => s.key))),
            ),
          (err) => {
            if (this.classFilter() === key) {
              this.classFilter.set('');
            }
            this.classError.set(contentErrorText(err, 'abrir as magias da classe'));
          },
        );
    }
  }

  protected setLevel(value: string): void {
    this.levelFilter.set(value);
  }

  protected dismissError(): void {
    this.state?.dismiss();
  }

  protected note(o: OptionSwitchEntry): string {
    const hidden = hiddenNote(o, this.byKey());
    if (hidden) {
      return hidden;
    }
    if (o.off && o.charactersUsing > 0) {
      const p = isMasculine(o.kind) ? 'o' : 'a';
      return o.charactersUsing === 1
        ? `A ficha que ${p} usa continua funcionando.`
        : `As fichas que ${p} usam continuam funcionando.`;
    }
    if (o.archived) {
      return 'Arquivada: os jogadores não a recebem.';
    }
    return '';
  }

  protected kindLabel(o: OptionSwitchEntry): string {
    if (o.kind === TableContentKind.SPELL) {
      return spellLevelLabel(o.level);
    }
    return o.kind === TableContentKind.SUBRACE || o.kind === TableContentKind.SUBCLASS
      ? this.parentName(o)
      : '';
  }

  private parentName(o: OptionSwitchEntry): string {
    const parent = o.parentKey ? this.byKey().get(o.parentKey) : undefined;
    if (!parent) {
      return '';
    }
    return o.kind === TableContentKind.SUBRACE
      ? `Sub-raça de ${parent.namePt}`
      : `Subclasse de ${parent.namePt}`;
  }
}
