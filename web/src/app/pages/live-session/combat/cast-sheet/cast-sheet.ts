import {
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
  viewChildren,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type CombatantState,
  type PendingDamage,
  type SpellCast,
  type SpellTargetResult,
  type SpellTargets,
  AttackOutcome,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  type SlotChoice,
  type SpellDetails,
  SpellRangeKind,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import {
  type Dealt,
  type SlotRow,
  castRows,
  castSubtitle,
  castTargetRows,
  dartTargets,
  dartsAt,
  dartsPlaced,
  dartsStatus,
  dealOne,
  defaultSlot,
  effectSentence,
  freeText,
  lastSlotWarning,
  rollGroups,
  slotRows,
  spellKind,
  spentLine,
  targetRule,
  toggled,
} from '../../../../core/combat/cast-flow';
import {
  type AttackDie,
  type DamageDie,
  type PoolDie,
  CombatClient,
  newKey,
} from '../../../../core/combat/combat-client';
import { ActionKey } from '../../../../core/connect/idempotency';
import { diceName, sumRange } from '../../../../core/combat/combat-dice';
import { criticalHint, criticalTypedHint, fixedParts } from '../../../../core/combat/critical';
import { poolDice, poolRollText } from '../../../../core/combat/hp-effects';
import { article } from '../../../../core/combat/combat-log';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { circleLabel } from '../../../../core/combat/combat-grid';
import { isPlayer } from '../../../../core/combat/combat-view';
import { groupFeminine, groupName, isCreature } from '../../../../core/combat/creature-names';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { openSpellDetails } from '../../../../shared/spell-details/open-spell-details';
import { spellDetailsFromGen } from '../../../../shared/spell-details/spell-details-map';
import { SpellHelp } from '../../../../shared/spell-details/spell-help';
import { RollPicker } from '../roll-picker/roll-picker';
import { injectSheet } from '../sheet-host';
import { SheetFrame } from '../sheet-frame/sheet-frame';
import { CastResult, CastSlots, type SlotAfter } from './cast-result';
import { CastTargets } from './cast-targets';
import { SlotPicker } from './slot-picker';

/** What the page hands the cast sheet. */
export interface CastSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly casterId: string;
  readonly round: number;
  readonly spellKey: string;
  readonly name: string;
  /** 0 for a cantrip. */
  readonly level: number;
  readonly concentration: boolean;
  readonly economy: ActionEconomy;
  /** The free slots the spell can be cast with (`SpellOption.slots`). */
  readonly slots: readonly SlotChoice[];
  /** The caster's slots by level and the pact slots, for "1 livre de 4". */
  readonly usage: readonly {
    readonly level: number;
    readonly total: number;
    readonly used: number;
  }[];
  readonly pact: {
    readonly slotLevel: number;
    readonly total: number;
    readonly used: number;
  } | null;
  /** Who it can target, with the distance (`GetTurnOptions.spell_targets`). */
  readonly targets: SpellTargets | undefined;
  /** How many free slots Escudo could still be cast with; `null` without Escudo. */
  readonly shieldFree: number | null;
  /** Escudo's Portuguese name ("Escudo Arcano"), as the spell list says it. */
  readonly shieldName: string;
  /** The spell attack bonus, for the typed d20's total. */
  readonly attackBonus: number;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** Where each answer's combat goes (the page's copy of the combat). */
  readonly state: CombatState;
  /** The damage of a cast whose sheet was closed before it was rolled: the
   * sheet opens at the damage with it. */
  readonly resume?: readonly PendingDamage[];
}

/**
 * "Conjurar Mísseis Mágicos" (MR-014, E6-09): the player's cast in a bottom
 * sheet on a phone and a dialog from a tablet up. The slot (a radio group of
 * circles), the targets (one, several, or Magic Missile's darts), the last-slot
 * warning, and then one filled button, "Conjurar X": the slot and the action
 * are spent when it is pressed, not when the damage is rolled (MR-014). A spell
 * with an attack roll asks for the d20 there instead (RN-18, one for each
 * target). The result lists each target (the roll, the save, the dice) and the
 * damage still to roll comes right under it, in the same two ways of rolling as
 * an attack's. The slot count, the Escudo that lost its slot, the concentration
 * and "Sua ação foi usada" close it. The keys (one for the cast, one for each
 * damage) are made once, so a tap repeated after a lost answer never casts
 * twice. Focus: the title opens first; after the result it goes to "Voltar à
 * sua vez".
 */
@Component({
  selector: 'app-cast-sheet',
  imports: [
    CastResult,
    CastSlots,
    CastTargets,
    MatButtonModule,
    MatIconModule,
    RollPicker,
    SheetFrame,
    SlotPicker,
    SpellHelp,
  ],
  templateUrl: './cast-sheet.html',
  styleUrl: './cast-sheet.scss',
})
export class CastSheet {
  private readonly api = inject(CombatClient);
  private readonly catalog = inject(SpellCatalog);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly sheet = injectSheet<CastSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly details = signal<SpellDetails | null>(null);
  protected readonly detailsFailed = signal(false);
  protected readonly slot = signal<SlotRow | null>(null);
  protected readonly chosen = signal<string[]>([]);
  protected readonly dealt = signal<Dealt>(new Map());
  protected readonly typing = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly cast = signal<SpellCast | null>(null);
  /** Every damage and heal of the cast as it is now, by ID. */
  protected readonly pendings = signal<ReadonlyMap<string, PendingDamage>>(
    new Map((this.data.resume ?? []).map((p) => [p.id, p])),
  );
  /** Whether the cast happened (or is being resumed): the result stage. */
  protected readonly done = computed(() => this.cast() !== null || !!this.data.resume);

  /** Made again when the choice changes: a new cast, not a retry. */
  private castKey = newKey();
  /** One key per damage roll of a pending (its die): the same die again is a retry, another die a new request. */
  private readonly damageKeys = new Map<string, ActionKey>();
  private readonly pickers = viewChildren(RollPicker);
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  private readonly frame = viewChild(SheetFrame);

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  /** The damage may be typed unless the master made everybody roll in the app. */
  protected readonly canTypeDamage = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;

  protected readonly rows = computed(() =>
    slotRows(this.data.level, this.data.slots, this.data.usage, this.data.pact),
  );
  protected readonly kind = computed(() => spellKind(this.data.spellKey, this.details()));
  /** Sono and Leque Cromático roll a pool of dice: the dice at this slot. */
  protected readonly pool = computed(() =>
    this.kind() === 'pool' ? poolDice(this.details(), this.slotLevel()) : null,
  );
  /** The roll the sheet asks for before the cast: the d20 of a spell attack, or the pool when the
   * table's dice may be typed (with the app rolling every die, "Conjurar" is enough). */
  protected readonly usesPicker = computed(
    () => this.kind() === 'attack' || (this.kind() === 'pool' && this.canTypeDamage),
  );
  /** A spell that reads hit points says its area: who is in it is for the caster to say. */
  protected readonly hpArea = computed(() => this.kind() === 'pool');
  protected readonly slotLevel = computed(() => this.slot()?.level ?? this.data.level);
  protected readonly rule = computed(() =>
    targetRule(this.data.targets, this.data.casterId, this.data.level, this.slotLevel()),
  );
  protected readonly dartsTotal = computed(() =>
    dartsAt(this.data.targets?.darts ?? [], this.slotLevel()),
  );
  protected readonly reachFt = computed(() => {
    const r = this.details()?.range;
    return r?.kind === SpellRangeKind.RANGED ? r.distanceFt : null;
  });
  /** The caster is a target only of a heal or a spell the app has no effect for
   * (a blessing): a dart, a ray or a fireball at oneself is never the choice. */
  protected readonly targetRows = computed(() => {
    const self = this.kind() === 'heal' || this.kind() === 'plain';
    const list = (this.data.targets?.targets ?? []).filter(
      (t) => self || t.combatantId !== this.data.casterId,
    );
    return castTargetRows(list, this.data.casterId, this.reachFt());
  });
  protected readonly npcs = computed(() => {
    const combatants = this.data.state.encounter()?.combatants ?? [];
    return new Set(combatants.filter((c) => !isPlayer(c)).map((c) => c.id));
  });
  protected readonly creatures = computed(() => {
    const combatants = this.data.state.encounter()?.combatants ?? [];
    return new Set(combatants.filter(isCreature).map((c) => c.id));
  });
  /**
   * "Constrição encerra a concentração em Conjurar Animais.", and "Os 2 Lobos atrozes somem." when that concentration holds the
   * caster's creatures: only when this spell needs concentration and the caster holds another one, from the combat's own data.
   */
  protected readonly endsConcentration = computed(() => {
    const e = this.data.state.encounter();
    const caster = e?.combatants.find((c) => c.id === this.data.casterId);
    if (
      !e ||
      !caster ||
      !this.data.concentration ||
      !caster.concentrationSpell ||
      caster.concentrationSpell === this.data.spellKey
    ) {
      return null;
    }
    const held = e.combatants.filter(
      (c) =>
        isCreature(c) &&
        c.ownerCharacterId === caster.characterId &&
        !!c.summonGroupId &&
        !c.defeated,
    );
    let goes = '';
    if (held.length === 1) {
      goes = `${article(held[0].label) === 'a' ? 'A' : 'O'} ${held[0].label} some.`;
    } else if (held.length > 1) {
      goes = `${groupFeminine(held) ? 'As' : 'Os'} ${held.length} ${groupName(held)} somem.`;
    }
    return {
      ends: `${this.data.name} encerra a concentração em ${caster.concentrationSpellNamePt || 'a magia'}.`,
      goes,
    };
  });
  protected readonly warning = computed(() =>
    lastSlotWarning(this.slot(), this.data.shieldFree, this.data.shieldName),
  );
  /** Why "Conjurar" is not ready yet, in words; empty when it is. */
  protected readonly missing = computed(() => {
    if (!this.details() && !this.detailsFailed()) {
      return 'Lendo a magia…';
    }
    if (this.detailsFailed()) {
      return 'Não deu para ler a magia. Feche e tente de novo.';
    }
    if (this.data.level > 0 && !this.slot()) {
      return 'Escolha o espaço de magia.';
    }
    const r = this.rule();
    if (r.kind === 'darts') {
      return dartsPlaced(this.dealt()) === this.dartsTotal()
        ? ''
        : dartsStatus(this.dartsTotal(), this.dealt());
    }
    if ((r.kind === 'single' || r.kind === 'multi') && this.chosen().length < r.min) {
      return r.kind === 'single' ? 'Escolha o alvo.' : 'Escolha pelo menos um alvo.';
    }
    if (this.kind() === 'attack' && this.chosen().length > 1 && !this.canApp) {
      return 'Com dados físicos, escolha um alvo por vez.';
    }
    return '';
  });
  protected readonly ready = computed(() => this.missing() === '');
  /** Magic Missile's count, in the footer next to the button: it states what is missing, and when all are placed. */
  protected readonly dartsLine = computed(() => {
    if (this.rule().kind !== 'darts' || this.dartsTotal() === 0) {
      return '';
    }
    const n = this.dartsTotal();
    return dartsStatus(n, this.dealt());
  });
  /** The d20 of a spell attack may be typed only for a single target (the server). */
  protected readonly canType = computed(
    () =>
      this.data.diceMode !== DiceMode.APP &&
      (this.kind() !== 'attack' || this.chosen().length <= 1),
  );

  protected readonly title = computed(() => {
    if (this.data.resume) {
      return `Role o dano de ${this.data.name}`;
    }
    if (this.hpDone()) {
      return `${this.data.name} conjurad${article(this.data.name) === 'a' ? 'a' : 'o'}`;
    }
    if (this.done()) {
      return `Você conjurou ${this.data.name}`;
    }
    return this.typing() ? 'Digite o resultado' : `Conjurar ${this.data.name}`;
  });
  /** "Sono conjurado", "Palavra de Poder Atordoar conjurada": the sheet of a spell that
   * reads hit points says what was done, then who it touched. */
  private readonly hpDone = computed(
    () => this.done() && (this.kind() === 'pool' || this.kind() === 'hp'),
  );
  protected readonly subtitle = computed(() => {
    if (this.done()) {
      const s = this.cast()?.slot;
      const circle = s ? circleLabel(s.level) : this.data.level === 0 ? 'Truque' : '';
      const n = this.cast()?.targets.length ?? 0;
      return this.kind() === 'pool' && n > 0
        ? `${circle} · ${n} ${n === 1 ? 'criatura' : 'criaturas'} na área`
        : circle;
    }
    if (this.typing()) {
      return `${this.data.name} · Rodada ${this.data.round}`;
    }
    return castSubtitle(
      this.data.economy,
      this.kind(),
      this.details(),
      this.slotLevel(),
      this.dartsTotal(),
    );
  });

  // ---- the result ----

  protected readonly labels = computed(() => {
    const out = new Map<string, { label: string; state: CombatantState }>();
    for (const c of this.data.state.encounter()?.combatants ?? []) {
      out.set(c.id, { label: c.label, state: c.state });
    }
    return out;
  });
  protected readonly resultRows = computed(() => {
    const cast = this.cast() ?? this.resumedCast();
    return castRows(cast, this.pendings(), this.labels());
  });
  /** The live sentence of a spell that reads hit points ("O Goblin 1 adormeceu. ..."), and the caster's
   * own roll of the pool: the server sends it only to the master and to the caster's player. */
  protected readonly sentence = computed(() => {
    const cast = this.cast();
    return cast ? effectSentence(cast, this.labels(), (id) => this.npcs().has(id)) : '';
  });
  protected readonly poolText = computed(() => {
    const roll = this.cast()?.poolRoll;
    return roll ? poolRollText(roll) : '';
  });
  protected readonly groups = computed(() => rollGroups([...this.pendings().values()]));
  protected readonly owed = computed(() => this.groups().length > 0);
  protected readonly plain = computed(() => {
    const c = this.cast();
    return (
      !!c &&
      this.kind() === 'plain' &&
      c.targets.every(
        (t) => !t.pendingDamageId && t.outcome === AttackOutcome.UNSPECIFIED && !t.save,
      )
    );
  });
  protected readonly after = computed<SlotAfter | null>(() => {
    const s = this.cast()?.slot;
    const row = s ? this.rows().find((r) => r.level === s.level && r.pact === s.pact) : undefined;
    if (!s || !row) {
      return null;
    }
    const free = Math.max(0, row.free - 1);
    return {
      level: s.level,
      total: row.total,
      used: row.total === null ? 0 : row.total - free,
      text: `Espaços de ${circleLabel(s.level)}: ${freeText(free, row.total)}`,
    };
  });
  protected readonly shieldLost = computed(() => {
    const s = this.cast()?.slot;
    return s && this.data.shieldFree === 1
      ? `${this.data.shieldName} indisponível: sem espaço de ${circleLabel(s.level)}.`
      : '';
  });
  protected readonly notes = computed(() => {
    const c = this.cast();
    if (!c) {
      return [];
    }
    const out: string[] = [];
    if (c.concentrating) {
      out.push(`Você está concentrado em ${this.data.name}.`);
    }
    if (c.concentrationEndedSpellKey) {
      out.push('A sua concentração anterior acabou.');
    }
    out.push(spentLine(this.data.economy));
    return out;
  });

  // ---- the damage still to roll ----

  /** The roll of the first damage group: its dice, the sum a typed roll may be. */
  protected readonly nextRoll = computed(() => {
    const g = this.groups()[0];
    if (!g) {
      return null;
    }
    const p = g[0];
    const labels = this.labels();
    const who = g.map((x) => labels.get(x.targetId)?.label ?? 'alvo');
    const name = diceName(p.diceCount, p.diceSides);
    return {
      pending: p,
      name,
      range: sumRange(p.diceCount, p.diceSides),
      label:
        p.diceCount > 1 ? `Role ${name} para o dano: some os dois` : `Role ${name} para o dano`,
      what: p.healing ? `Cura em ${who.join(', ')}` : `Dano em ${who.join(', ')}`,
      count: this.groups().length,
      // A critical spell attack follows the table's rule: what to roll, said the way it asks (RN-24).
      crit: p.critical
        ? (criticalHint(p.criticalRule, p.diceCount, p.diceSides, p.criticalMax)?.line ??
          'Acerto crítico: os dados dobram.')
        : '',
      typedHint: criticalTypedHint(
        p.criticalRule,
        name,
        sumRange(p.diceCount, p.diceSides).min,
        sumRange(p.diceCount, p.diceSides).max,
        p.criticalMax,
      ),
      // The modifier and the critical's fixed maximum, the server's numbers, added in the total before it is sent.
      modifier: p.bonus + p.criticalMax,
      fixedText: fixedParts(p.criticalMax, p.bonus),
    };
  });

  constructor() {
    void this.catalog.details(this.data.campaignId, this.data.spellKey).then((d) => {
      if (d) {
        this.details.set(d);
      } else {
        this.detailsFailed.set(true);
      }
    });
    // The lowest free slot is chosen, as the design draws it; an only target
    // and an only reachable dart target are chosen too.
    const first = defaultSlot(this.rows());
    if (first) {
      this.slot.set(first);
    }
    effect(() => {
      // When the targets list or the slot changes the choice may no longer fit.
      const r = this.rule();
      const ok = new Set(
        this.targetRows()
          .filter((t) => !t.blocked)
          .map((t) => t.id),
      );
      const kept = this.chosen()
        .filter((id) => ok.has(id))
        .slice(0, Math.max(r.max, 0));
      if (kept.length !== this.chosen().length) {
        this.chosen.set(kept);
        untracked(() => this.choiceChanged());
      }
      if (r.kind === 'darts') {
        const only = [...ok];
        if (
          only.length === 1 &&
          dartsPlaced(this.dealt()) === 0 &&
          this.dartsTotal() > 0 &&
          !this.cast()
        ) {
          this.dealt.set(new Map([[only[0], this.dartsTotal()]]));
          untracked(() => this.choiceChanged());
        }
        if (dartsPlaced(this.dealt()) > this.dartsTotal()) {
          this.dealt.set(new Map());
          untracked(() => this.choiceChanged());
        }
      }
    });
    // After a result the focus goes to the one next action, as soon as it is drawn.
    effect(() => this.back()?.nativeElement.focus());
    // The answer of a request in the air has to be shown: the sheet can't be dismissed meanwhile.
    effect(() => this.sheet.lock(this.busy()));
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
    // A new stage (the result) opens at its top, not where the last one was
    // scrolled; so does the whole result once the last damage is typed (the
    // player scrolled down to the field).
    effect(() => {
      this.done();
      this.owed();
      untracked(() => queueMicrotask(() => this.frame()?.scrollToTop()));
    });
  }

  /** A cast sheet that opens on a damage the player never rolled has no cast to
   * read: its rows are made from the damages themselves. */
  private resumedCast(): SpellCast {
    const targets = [...this.pendings().values()].map(
      (p) =>
        ({
          combatantId: p.targetId,
          darts: p.attackKey === 'spell:magic-missile' ? p.diceCount : 0,
          outcome: AttackOutcome.UNSPECIFIED,
          pendingDamageId: p.id,
        }) as unknown as SpellTargetResult,
    );
    return { targets } as unknown as SpellCast;
  }

  /** A new slot, target or dart changes what a typed roll was for (its dice, its target): the key is a new cast's and the typed text is dropped (RN-18). */
  private choiceChanged(): void {
    this.castKey = newKey();
    for (const picker of this.pickers()) {
      picker.clear();
    }
  }

  protected pickSlot(row: SlotRow): void {
    this.slot.set(row);
    this.choiceChanged();
    this.error.set('');
  }

  protected toggle(id: string): void {
    this.chosen.set(toggled(this.rule(), this.chosen(), id));
    this.choiceChanged();
    this.error.set('');
  }

  protected deal(change: { id: string; delta: 1 | -1 }): void {
    this.dealt.set(dealOne(this.dealt(), change.id, change.delta, this.dartsTotal()));
    this.choiceChanged();
    this.error.set('');
  }

  /** "Conjurar X" (a spell with no attack roll). */
  protected castNow(): Promise<void> {
    return this.doCast(null);
  }

  protected castRolled(die: AttackDie): Promise<void> {
    return this.doCast(die);
  }

  /** A pool spell with the dice the table's mode allows: rolled in the app, or the typed sum. */
  protected castPooled(die: PoolDie): Promise<void> {
    return this.doCast(die);
  }

  private async doCast(die: AttackDie | PoolDie | null): Promise<void> {
    if (this.busy() || !this.ready()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const slot = this.slot();
      const targets =
        this.rule().kind === 'darts'
          ? dartTargets(this.dealt())
          : this.rule().kind === 'none'
            ? []
            : this.chosen().map((combatantId) => ({ combatantId, darts: 0 }));
      const res = await this.api.castSpell(
        this.data.campaignId,
        this.data.encounterId,
        this.data.casterId,
        this.data.spellKey,
        this.data.level > 0 && slot ? { level: slot.level, pact: slot.pact } : null,
        targets,
        // A pool is always rolled by someone: the server does it when the app rolls every die.
        die ?? (this.kind() === 'pool' ? { inApp: true } : null),
        this.castKey,
      );
      this.data.state.apply(res.encounter);
      this.cast.set(res.cast);
      this.pendings.set(new Map(res.cast.pendingDamages.map((p) => [p.id, p])));
      this.typing.set(false);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'conjurar a magia'));
    } finally {
      this.busy.set(false);
    }
  }

  /** "Rolar no app": every damage still owed is rolled, one call each. */
  protected async rollAll(): Promise<void> {
    for (const group of this.groups()) {
      if (!(await this.rollGroup(group, { inApp: true }))) {
        return;
      }
    }
  }

  protected async rollTyped(sum: number): Promise<void> {
    const group = this.groups()[0];
    if (group) {
      if (await this.rollGroup(group, { sum })) {
        this.typing.set(false);
      }
    }
  }

  private async rollGroup(group: readonly PendingDamage[], die: DamageDie): Promise<boolean> {
    const p = group[0];
    if (this.busy()) {
      return false;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      let keys = this.damageKeys.get(p.id);
      if (!keys) {
        keys = new ActionKey();
        this.damageKeys.set(p.id, keys);
      }
      const key = keys.keyFor(die);
      const res = await this.api.rollDamage(
        this.data.campaignId,
        this.data.encounterId,
        p.id,
        die,
        key,
      );
      this.data.state.apply(res.encounter);
      const next = new Map(this.pendings());
      for (const settled of [res.pending, ...res.cast]) {
        next.set(settled.id, settled);
      }
      this.pendings.set(next);
      return true;
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'rolar o dano'));
      return false;
    } finally {
      this.busy.set(false);
    }
  }

  /** The "?" in the header: the spell's description over this sheet, so the slot and the targets chosen stay. */
  protected describe(): void {
    const { campaignId, spellKey, name } = this.data;
    openSpellDetails(
      this.dialog,
      this.bottomSheet,
      {
        namePt: name,
        load: async () => {
          const details = await this.catalog.details(campaignId, spellKey);
          if (!details) {
            throw new Error('spell details unavailable');
          }
          return spellDetailsFromGen(details);
        },
      },
      true,
    );
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.done() && !this.owed());
  }
}
