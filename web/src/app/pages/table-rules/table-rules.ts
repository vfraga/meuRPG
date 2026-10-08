import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CriticalRule,
  DeathSaveVisibility,
  DiceMode,
  HitPointsRule,
  Role,
  TableStyle,
  XpMode,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import type { DiceOption } from '../../core/campaigns/dice-labels';
import {
  DICE_WORDS,
  HOUSE_RULE_MAX_COUNT,
  HOUSE_RULE_MAX_LENGTH,
  STYLE_LABELS,
  TableRulesClient,
  type RulesDraft,
  type TableRulesVm,
  applyPreset,
  changeCount,
  draftProblem,
  styleOf,
  tableRulesError,
} from '../../core/campaigns/table-rules';
import { DiceChoice } from '../../shared/dice-choice/dice-choice';
import { FogSwitch } from './fog-switch/fog-switch';
import { RulesRead } from './rules-read/rules-read';
import { MethodCards } from './method-cards/method-cards';
import { XpModePanel } from './xp-mode-panel/xp-mode-panel';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error'; message: string }
  | { status: 'ready'; vm: TableRulesVm; campaignName: string; xpMode: XpMode; canEdit: boolean };

const DICE_TITLES: Readonly<Record<number, string>> = {
  [DiceMode.APP]: 'Todos rolam no app',
  [DiceMode.PHYSICAL]: 'Todos rolam os próprios dados',
  [DiceMode.PLAYERS_CHOOSE]: 'Cada jogador escolhe',
};

/** What choosing a style did to the choices below it: said in a notice, until the master edits by hand. */
interface StyleNote {
  readonly title: string;
  readonly changed: readonly string[];
  readonly same: readonly string[];
}

const XP_OPTION_TITLES: Readonly<Record<number, string>> = {
  [XpMode.ENEMIES]: 'Por inimigos',
  [XpMode.MILESTONES]: 'Por marcos',
  [XpMode.GOLD]: 'Por ouro',
};

const STYLE_OPTIONS: readonly DiceOption<TableStyle>[] = [
  {
    value: TableStyle.TUDO_NO_APP,
    title: 'Tudo no app',
    description: 'Dados no app, combate com mapa, névoa ligada nos mapas novos.',
  },
  {
    value: TableStyle.MESA_FISICA,
    title: 'Mesa física',
    description: 'Dados físicos digitados, combate sem mapa por padrão, névoa desligada.',
  },
  {
    value: TableStyle.TEATRO_DA_MENTE,
    title: 'Teatro da mente',
    description: 'Cada jogador escolhe os dados, combate sem mapa, sem névoa.',
  },
  {
    value: TableStyle.PERSONALIZADO,
    title: 'Personalizado',
    description: 'Suas escolhas não batem com nenhum estilo.',
  },
];

const HIT_POINT_OPTIONS: readonly DiceOption<HitPointsRule>[] = [
  {
    value: HitPointsRule.PLAYER_CHOOSES,
    title: 'O jogador escolhe',
    description: 'Rola o dado ou usa a média, na hora de subir.',
  },
  {
    value: HitPointsRule.ROLL,
    title: 'Rolar o dado',
    description: 'Todos rolam; a média não é oferecida.',
  },
  {
    value: HitPointsRule.AVERAGE,
    title: 'A média',
    description: 'Todos recebem o valor médio do dado de vida.',
  },
];

const CRITICAL_OPTIONS: readonly DiceOption<CriticalRule>[] = [
  {
    value: CriticalRule.DOUBLED_DICE,
    title: 'Dados dobrados (SRD)',
    description: 'Todos os dados de dano do ataque ou da magia são rolados duas vezes.',
  },
  {
    value: CriticalRule.MAX_PLUS_ROLL,
    title: 'O máximo dos dados mais uma rolagem',
    description: 'Os dados valem o máximo; depois se rola um conjunto.',
  },
];

const DEATH_SAVE_OPTIONS: readonly DiceOption<DeathSaveVisibility>[] = [
  {
    value: DeathSaveVisibility.VISIBLE_TO_ALL,
    title: 'Todos veem',
    description: 'Os outros jogadores veem as falhas e os sucessos.',
  },
  {
    value: DeathSaveVisibility.OWNER_AND_MASTER,
    title: 'Só o dono e o mestre',
    description: 'Os outros não veem, nem na tela, nem no registro.',
  },
];

const DICE_OPTIONS: readonly DiceOption<DiceMode>[] = [
  {
    value: DiceMode.APP,
    title: 'Todos rolam no app',
    description: 'O app rola e registra cada dado.',
  },
  {
    value: DiceMode.PHYSICAL,
    title: 'Todos rolam os próprios dados',
    description: 'Cada jogador digita o que tirou.',
  },
  {
    value: DiceMode.PLAYERS_CHOOSE,
    title: 'Cada jogador escolhe',
    description: 'Rolar no app ou com os próprios dados.',
  },
];

const COMBAT_OPTIONS: readonly DiceOption<number>[] = [
  { value: 1, title: 'Começar com mapa', description: 'O combate usa o mapa e a grade.' },
  {
    value: 0,
    title: 'Começar sem mapa (teatro da mente)',
    description: 'O mestre julga o alcance; o movimento é por número.',
  },
];

/**
 * "Regras da mesa" (MR-025, RN-24, RN-09; E10-03 states 1 to 3), master only: the "Estilo da mesa" on top, which fills the
 * three settings it owns (dice, combat with or without a map, fog on new maps) and leaves each editable; then the
 * table's choices (hit points at level-up, the ways of making ability scores, the critical, who sees death saves, the
 * house reminders), the XP mode with its own question, and the links to the table's content and the maps. One "Salvar
 * regras" saves all of it at once with the dice mode (`SetTableRules`); the XP mode has its own flow (`XpModePanel`).
 * The style a draft makes is worked out here from the server's three presets before it is saved, and nothing is
 * calculated that the server does not already say (ADR-0008).
 */
@Component({
  selector: 'app-table-rules',
  imports: [
    DiceChoice,
    FogSwitch,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressSpinnerModule,
    MethodCards,
    RouterLink,
    RulesRead,
    XpModePanel,
  ],
  templateUrl: './table-rules.html',
  styleUrl: './table-rules.scss',
})
export class TableRulesPage {
  private readonly campaigns = inject(CampaignsService);
  private readonly client = inject(TableRulesClient);
  private readonly route = inject(ActivatedRoute);
  private readonly injector = inject(Injector);

  protected readonly campaignId = signal('');
  protected readonly state = signal<PageState>({ status: 'loading' });
  protected readonly draft = signal<RulesDraft | null>(null);
  /** What the server has: the draft's baseline for "Tudo salvo" and the "Mudou" tags. */
  protected readonly saved = signal<RulesDraft | null>(null);
  protected readonly styleNote = signal<StyleNote | null>(null);
  protected readonly saving = signal(false);
  protected readonly justSaved = signal(false);
  protected readonly error = signal('');

  protected readonly styleOptions = STYLE_OPTIONS;
  protected readonly hitPointOptions = HIT_POINT_OPTIONS;
  protected readonly criticalOptions = CRITICAL_OPTIONS;
  protected readonly deathSaveOptions = DEATH_SAVE_OPTIONS;
  protected readonly diceOptions = DICE_OPTIONS;
  protected readonly combatOptions = COMBAT_OPTIONS;
  protected readonly inertStyle: readonly TableStyle[] = [TableStyle.PERSONALIZADO];
  protected readonly maxLength = HOUSE_RULE_MAX_LENGTH;
  protected readonly maxCount = HOUSE_RULE_MAX_COUNT;

  private readonly vm = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? s.vm : null;
  });
  protected readonly style = computed<TableStyle>(() => {
    const d = this.draft();
    const vm = this.vm();
    return d && vm ? styleOf(d, vm.presets) : TableStyle.PERSONALIZADO;
  });
  protected readonly changes = computed(() => {
    const d = this.draft();
    const s = this.saved();
    return d && s ? changeCount(d, s) : 0;
  });
  protected readonly problem = computed(() => {
    const d = this.draft();
    return d ? draftProblem(d) : '';
  });
  protected readonly canSave = computed(
    () => this.changes() > 0 && this.problem() === '' && !this.saving(),
  );
  protected readonly saveLine = computed(() => {
    const n = this.changes();
    if (n === 0) {
      return 'Tudo salvo. Nenhuma mudança para salvar.';
    }
    return n === 1 ? '1 mudança não salva' : `${n} mudanças não salvas`;
  });
  /** "Personalizado. Suas escolhas não batem mais com nenhum estilo.", once the master edited a preset by hand. */
  protected readonly wentCustom = signal(false);
  protected readonly diceWord = (m: DiceMode): string => DICE_WORDS[m] ?? '';

  /** The master's question is open (the XP mode): the sticky bar's button is muted, so one filled button shows. */
  protected readonly questionOpen = signal(false);
  private readonly bar = viewChild<ElementRef<HTMLElement>>('bar');

  /** What a player reads: each rule in words (the page is read-only for them; only the master changes it). */
  protected readonly readRows = computed<readonly { label: string; value: string }[]>(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return [];
    }
    const d = s.vm.saved;
    const methods = [
      d.standardArray ? 'Conjunto padrão' : '',
      d.pointBuy ? 'Compra por pontos' : '',
      d.rolled4d6 ? '4d6, descartando o menor' : '',
      d.typed ? 'Digitar os valores' : '',
    ].filter((m) => m !== '');
    return [
      { label: 'Dados', value: DICE_TITLES[d.diceMode] },
      {
        label: 'Combate',
        value: d.combatStartsWithMap ? 'Começa com mapa' : 'Começa sem mapa (teatro da mente)',
      },
      { label: 'Névoa de guerra nos mapas novos', value: d.fogOnNewMaps ? 'Ligada' : 'Desligada' },
      {
        label: 'Pontos de vida ao subir de nível',
        value: HIT_POINT_OPTIONS.find((o) => o.value === d.hitPoints)?.title ?? '',
      },
      { label: 'Habilidades de uma ficha nova', value: methods.join(', ') },
      {
        label: 'Acertos críticos',
        value: CRITICAL_OPTIONS.find((o) => o.value === d.critical)?.title ?? '',
      },
      {
        label: 'Testes contra a morte',
        value: DEATH_SAVE_OPTIONS.find((o) => o.value === d.deathSaves)?.title ?? '',
      },
      { label: 'Experiência', value: XP_OPTION_TITLES[s.xpMode] ?? '' },
    ];
  });
  protected readonly styleLabel = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? STYLE_LABELS[s.vm.style] : '';
  });
  protected readonly readReminders = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? s.vm.saved.houseRules : [];
  });

  constructor() {
    // The sticky save bar must never cover a focused field or an open question: the page keeps its height in the
    // viewport's `scroll-padding-bottom` (WCAG 2.4.11), so anything scrolled or focused into view stops above it.
    effect((onCleanup) => {
      const el = this.bar()?.nativeElement;
      if (!el || typeof ResizeObserver === 'undefined') {
        return;
      }
      const root = document.documentElement;
      const set = () => (root.style.scrollPaddingBottom = `${el.offsetHeight + 16}px`);
      set();
      const observer = new ResizeObserver(set);
      observer.observe(el);
      onCleanup(() => {
        observer.disconnect();
        root.style.scrollPaddingBottom = '';
      });
    });
    this.route.paramMap.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((params) => {
      const id = params.get('id');
      if (id) {
        this.campaignId.set(id);
        void this.load(id);
      }
    });
  }

  protected async load(id = this.campaignId()): Promise<void> {
    this.state.set({ status: 'loading' });
    try {
      const res = await this.campaigns.getCampaign(id);
      const campaign = res.campaign;
      if (!campaign || campaign.awaitingApproval) {
        this.state.set({ status: 'not-found' });
        return;
      }
      const canEdit = campaign.myRole === Role.MASTER;
      const vm = await this.client.get(id);
      this.saved.set(vm.saved);
      this.draft.set(vm.saved);
      this.styleNote.set(null);
      this.wentCustom.set(false);
      this.state.set({
        status: 'ready',
        vm,
        campaignName: campaign.name,
        xpMode: campaign.xpMode,
        canEdit,
      });
    } catch (err) {
      const code = ConnectError.from(err, Code.Unavailable).code;
      this.state.set(
        code === Code.NotFound
          ? { status: 'not-found' }
          : { status: 'error', message: tableRulesError(err, 'abrir as regras') },
      );
    }
  }

  /** An edit by hand: the style follows the three settings, and a note about a preset goes away. */
  protected edit(patch: Partial<RulesDraft>): void {
    const d = this.draft();
    const vm = this.vm();
    if (!d || !vm) {
      return;
    }
    const before = styleOf(d, vm.presets);
    const next = { ...d, ...patch };
    this.draft.set(next);
    this.justSaved.set(false);
    this.error.set('');
    this.styleNote.set(null);
    this.wentCustom.set(
      before !== TableStyle.PERSONALIZADO && styleOf(next, vm.presets) === TableStyle.PERSONALIZADO,
    );
  }

  protected chooseStyle(style: TableStyle): void {
    const d = this.draft();
    const vm = this.vm();
    const preset = vm?.presets.find((p) => p.style === style);
    if (!d || !vm || !preset) {
      return;
    }
    const next = applyPreset(d, preset);
    const changed: string[] = [];
    const same: string[] = [];
    (next.combatStartsWithMap !== d.combatStartsWithMap ? changed : same).push(
      `Combate: ${next.combatStartsWithMap ? 'começar com mapa' : 'começar sem mapa'}.`,
    );
    (next.fogOnNewMaps !== d.fogOnNewMaps ? changed : same).push(
      `Névoa de guerra nos mapas novos: ${next.fogOnNewMaps ? 'ligada' : 'desligada'}.`,
    );
    if (next.diceMode !== d.diceMode) {
      changed.push(`Dados: ${DICE_WORDS[next.diceMode]}.`);
    } else {
      same.push(`Dados: já era “${DICE_TITLES[next.diceMode]}”.`);
    }
    this.draft.set(next);
    this.justSaved.set(false);
    this.error.set('');
    this.wentCustom.set(false);
    this.styleNote.set({
      title: STYLE_LABELS[style],
      changed,
      same,
    });
  }

  /** "Mudou" and what it was: the three settings the style owns, against what the server has. */
  protected readonly was = computed<{ dice: string; combat: string; fog: string } | null>(() => {
    const d = this.draft();
    const s = this.saved();
    if (!d || !s) {
      return null;
    }
    return {
      dice: d.diceMode !== s.diceMode ? DICE_WORDS[s.diceMode] : '',
      combat:
        d.combatStartsWithMap !== s.combatStartsWithMap
          ? s.combatStartsWithMap
            ? 'com mapa'
            : 'sem mapa'
          : '',
      fog: d.fogOnNewMaps !== s.fogOnNewMaps ? (s.fogOnNewMaps ? 'ligada' : 'desligada') : '',
    };
  });

  protected setHouseRule(index: number, text: string): void {
    const d = this.draft();
    if (d) {
      this.edit({ houseRules: d.houseRules.map((r, i) => (i === index ? text : r)) });
    }
  }

  protected addHouseRule(): void {
    const d = this.draft();
    if (d && d.houseRules.length < HOUSE_RULE_MAX_COUNT) {
      this.edit({ houseRules: [...d.houseRules, ''] });
      afterNextRender(
        () => {
          const fields = document.querySelectorAll<HTMLInputElement>('.reminder__field input');
          fields[fields.length - 1]?.focus();
        },
        { injector: this.injector },
      );
    }
  }

  protected removeHouseRule(index: number): void {
    const d = this.draft();
    if (d) {
      this.edit({ houseRules: d.houseRules.filter((_, i) => i !== index) });
    }
  }

  protected async save(): Promise<void> {
    const d = this.draft();
    if (!d || !this.canSave()) {
      return;
    }
    this.saving.set(true);
    this.error.set('');
    try {
      const res = await this.client.set(this.campaignId(), d);
      this.saved.set(res.saved);
      // What the server kept replaces the draft only when nothing was edited while the call ran: a later edit stays and
      // is still counted as unsaved.
      if (this.draft() === d) {
        this.draft.set(res.saved);
      }
      this.styleNote.set(null);
      this.wentCustom.set(false);
      this.justSaved.set(true);
    } catch (err) {
      this.error.set(tableRulesError(err, 'salvar as regras'));
    } finally {
      this.saving.set(false);
    }
  }
}
