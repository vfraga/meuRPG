import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
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
  AreaPlacement,
  type CombatantState,
  type PendingDamage,
  type SpellCast,
  type SpellTargetResult,
  type SpellTargets,
  AttackOutcome,
  type DiceRoll,
  type Encounter,
  type LuckyOffer,
  type OutsideInspirationOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  type MetamagicOption,
  type SlotChoice,
  type SpellDetails,
  SpellAreaShape,
  SpellDamageChoice,
  SpellRangeKind,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
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
  saveAbility,
  slotRows,
  spellKind,
  spentLine,
  targetRule,
  toggled,
} from '../../../../core/combat/cast-flow';
import {
  type AttackDie,
  type DamageDie,
  type PairDie,
  type PoolDie,
  CombatClient,
  newKey,
} from '../../../../core/combat/combat-client';
import { d20Count, modeStatus, orNormal } from '../../../../core/combat/roll-mode';
import { ActionKey } from '../../../../core/connect/idempotency';
import { AreaFlow } from '../../../../core/combat/area-flow';
import {
  allyWarning,
  areaRows,
  confirmLabel,
  countText,
  nobodyText,
  notPlacedReason,
  shapeText,
  stepOneTitle,
} from '../../../../core/combat/area-text';
import type { AreaShape } from '../../../../core/combat/spell-area';
import type { MapLayers } from '../../../../core/maps/layers';
import type { Vision } from '../../../../core/maps/vision';
import { AreaList } from '../../../../shared/area-picker/area-list';
import { AreaMap } from '../../../../shared/area-picker/area-map';
import { AreaStep } from '../../../../shared/area-picker/area-step';
import type { CombatMapImage } from '../../../../shared/combat-map/combat-map';
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
import { abilityMissing, needsAbility } from '../../../../core/effects/ability-choice';
import {
  type MetamagicPicks,
  NO_PICKS,
  castLabel,
  chosenLine,
  metamagicChoices,
  metamagicCost,
  metamagicMissing,
  metamagicRows,
  metamagicSpentLine,
  toggledOption,
} from '../../../../core/resources/metamagic';
import { isHit, outcomeWord } from '../../../../core/combat/attack-flow';
import {
  RollAnimator,
  type RollShow,
  showOfDice,
  showOfDie,
} from '../../../../shared/roll-overlay/roll-animator';
import { InspirationQuestion } from '../../../../core/resources/bardic-inspiration';
import { LuckyQuestion, luckyHeldLine, luckyReroll } from '../../../../core/combat/lucky-question';
import type { Pool } from '../../../../core/resources/pools';
import {
  ResourceClient,
  type InspirationRoll,
  type LuckyRoll,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { InspirationOffer } from '../../inspiration/inspiration-offer';
import { LuckyPrompt } from '../lucky-prompt/lucky-prompt';
import { openSpellDetails } from '../../../../shared/spell-details/open-spell-details';
import { spellDetailsFromGen } from '../../../../shared/spell-details/spell-details-map';
import { SpellHelp } from '../../../../shared/spell-details/spell-help';
import { AbilityPicker } from '../../effects/ability-picker/ability-picker';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollModePicker } from '../roll-mode/roll-mode-picker';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { CastResult, CastSlots, type SlotAfter } from './cast-result';
import { CastTargets } from './cast-targets';
import { SculptPicker, type SculptRow } from './sculpt-picker';
import { MetamagicPicker } from './metamagic-picker';
import { DamageTypePicker } from './damage-type-picker';
import { SlotPicker } from '../../../../shared/slot-picker/slot-picker';

/** The battle map an area spell is placed on (a combat with a grid): what the page already draws. */
export interface CastMapData {
  readonly image: CombatMapImage;
  readonly mapName: string;
  readonly columns: number;
  readonly rows: number;
  readonly layers: MapLayers | null;
  /** A player's map with the fog on: what they see. */
  readonly fog: Vision | null;
}

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
  /** A damage cantrip's dice at the caster's level (`Attack.spellDice`); empty otherwise. */
  readonly cantripDice: string;
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
  /** The master casts it for an NPC: any roll mode, with no need to ask. */
  readonly master?: boolean;
  /** The damage of a cast whose sheet was closed before it was rolled: the
   * sheet opens at the damage with it. */
  readonly resume?: readonly PendingDamage[];
  /** The Metamagic options the sorcerer knows, for this spell (`SpellOption.metamagic_options`); empty for none. */
  readonly metamagic?: readonly MetamagicOption[];
  /** The sorcery points left and their maximum, for "Pontos de Feitiçaria: 5 de 5". */
  readonly sorceryPoints?: Pool | null;
  /** The caster has Sculpt Spells and the spell is of evocation (`SpellOption.sculpt_spells`). */
  readonly sculptSpells?: boolean;
  /** The battle map, in a combat with a grid: an area spell is placed on it. Absent without a map (theatre of the mind),
   * where every area spell keeps the list of who it hits. */
  readonly map?: CastMapData | null;
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
    AreaList,
    AreaMap,
    AreaStep,
    CastResult,
    CastSlots,
    CastTargets,
    AbilityPicker,
    DamageTypePicker,
    InspirationOffer,
    LuckyPrompt,
    MatButtonModule,
    MetamagicPicker,
    MatIconModule,
    MultiRoll,
    RollModePicker,
    RollPicker,
    SculptPicker,
    SheetFrame,
    SlotPicker,
    SpellHelp,
  ],
  templateUrl: './cast-sheet.html',
  styleUrl: './cast-sheet.scss',
})
export class CastSheet {
  private readonly api = inject(CombatClient);
  /** Read when the player answers about the die: a sheet that never asks does not build the client. */
  private readonly lazyInjector = inject(Injector);
  private get resources(): ResourceClient {
    return this.lazyInjector.get(ResourceClient);
  }
  private readonly animator = inject(RollAnimator);
  private readonly catalog = inject(SpellCatalog);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly sheet = injectSheet<CastSheetData, boolean>();
  private readonly injector = inject(Injector);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly details = signal<SpellDetails | null>(null);
  protected readonly detailsFailed = signal(false);
  protected readonly slot = signal<SlotRow | null>(null);
  protected readonly chosen = signal<string[]>([]);
  protected readonly dealt = signal<Dealt>(new Map());
  protected readonly typing = signal(false);
  /** The mode of a spell attack's d20 the caster picked; null leaves it to the app's suggestion for each target. */
  protected readonly picked = signal<RollMode | null>(null);
  protected readonly reason = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly cast = signal<SpellCast | null>(null);
  /** The spell attack's d20 was rolled and kept: the character holds a Bardic Inspiration die, and the player decides
   * before the result is told. Nothing was spent yet. */
  protected readonly offer = signal<OutsideInspirationOffer | null>(null);
  private readonly answerKey = new ActionKey();
  /** The spell attack's d20 was rolled and kept because it shows a 1 and the caster is a halfling: the Lucky question
   * ("Tirou 1: rolar de novo?") comes first, before the die's. Nothing was spent yet. */
  protected readonly luckyOffer = signal<LuckyOffer | null>(null);
  /** The held d20, above the Lucky question: the sheet has no result to show it in yet. */
  protected readonly luckyHeld = computed(() => luckyHeldLine(this.luckyOffer()));
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKey = new ActionKey();
  /** Every damage and heal of the cast as it is now, by ID. */
  protected readonly pendings = signal<ReadonlyMap<string, PendingDamage>>(
    new Map((this.data.resume ?? []).map((p) => [p.id, p])),
  );
  /** The Metamagic options the sorcerer knows for this spell, and the ones marked: at most one, or Empowered Spell
   * with one other; each may ask for creatures (`picks`). */
  protected readonly metaOptions = this.data.metamagic ?? [];
  protected readonly sorceryPoints = this.data.sorceryPoints ?? null;
  protected readonly chosenMeta = signal<string[]>([]);
  protected readonly picks = signal<MetamagicPicks>(NO_PICKS);
  protected readonly metaRows = computed(() => metamagicRows(this.metaOptions, this.chosenMeta()));
  protected readonly metaCost = computed(() => metamagicCost(this.metaOptions, this.chosenMeta()));
  protected readonly metaLine = computed(() => chosenLine(this.metaOptions, this.chosenMeta()));
  /** "Conjurar e gastar 1 ponto" once an option is marked, else "Conjurar X". */
  protected readonly castText = computed(() => castLabel(this.data.name, this.metaCost()));
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

  /** The damage types a spell lets the caster pick among; empty when it offers no choice. */
  protected readonly damageChoices = computed(() => {
    const d = this.details();
    return d && d.damageChoice !== SpellDamageChoice.UNSPECIFIED && d.damage.length > 1
      ? d.damage.map((x) => ({ key: x.damageTypeKey, name: x.damageTypePt }))
      : [];
  });
  /** The picked damage type; the first until the caster picks another. */
  private readonly pickedType = signal('');
  protected readonly damageType = computed(() => {
    const choices = this.damageChoices();
    const picked = this.pickedType();
    return choices.find((c) => c.key === picked)?.key ?? choices[0]?.key ?? '';
  });
  /** Aprimorar Habilidade names the ability it is cast for; empty until picked, and for every other spell. */
  protected readonly asksAbility = needsAbility(this.data.spellKey);
  protected readonly ability = signal('');
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
    const ability = abilityMissing(this.data.spellKey, this.ability());
    if (ability) {
      return ability;
    }
    // A placed area takes no target: the server works out who is inside from the point.
    const target = this.flow ? '' : this.targetsMissing();
    return (
      target ||
      metamagicMissing(this.metaOptions, this.chosenMeta(), this.picks(), this.sorceryPoints)
    );
  });

  /** What the target step still needs, in words; `''` when it is complete. */
  private targetsMissing(): string {
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
  }
  protected readonly ready = computed(() => this.missing() === '');

  // ---- Sculpt Spells (SRD 5.1, School of Evocation wizard) ----

  /** The caster has Sculpt Spells and the spell is of evocation: the creatures it spares are asked for. */
  protected readonly sculptOffered = this.data.sculptSpells ?? false;
  protected readonly sculpted = signal<string[]>([]);
  /** 1 + the level the spell is cast at (a cantrip is level 0). */
  protected readonly sculptLimit = computed(() => 1 + (this.data.level > 0 ? this.slotLevel() : 0));
  /** The creatures the caster can spare: the others in the placed area, or the ones ticked in the list; allies first. */
  protected readonly sculptRows = computed<SculptRow[]>(() => {
    if (!this.sculptOffered) {
      return [];
    }
    const preview = this.flow?.preview();
    const rows: SculptRow[] = this.flow
      ? (preview?.targets ?? [])
          .filter((t) => !t.self)
          .map((t) => ({ id: t.combatantId, label: t.label, ally: t.ally }))
      : this.targetRows()
          .filter((t) => !t.blocked && this.chosen().includes(t.id))
          .map((t) => ({ id: t.id, label: t.label, ally: false }));
    return [...rows.filter((r) => r.ally), ...rows.filter((r) => !r.ally)];
  });
  /** What the cast sends: the ones marked that are still listed, no more than the limit. */
  private sculptedIds(): string[] {
    const listed = new Set(this.sculptRows().map((r) => r.id));
    return this.sculpted()
      .filter((id) => listed.has(id))
      .slice(0, this.sculptLimit());
  }

  protected toggleSculpt(id: string): void {
    const now = this.sculpted();
    this.sculpted.set(
      now.includes(id)
        ? now.filter((x) => x !== id)
        : now.length < this.sculptLimit()
          ? [...now, id]
          : now,
    );
    this.choiceChanged();
    this.error.set('');
  }

  // ---- an area placed on the map (PM-02a, PM-02b) ----

  /** How the spell's area is placed: only on a map with a grid; without one every area spell keeps its list. */
  protected readonly placement =
    this.data.map && this.data.targets ? this.data.targets.placement : AreaPlacement.UNSPECIFIED;
  protected readonly Placement = AreaPlacement;
  /** The two steps of the area, for a placed area spell; `null` for any other cast. */
  protected readonly flow =
    this.placement === AreaPlacement.UNSPECIFIED
      ? null
      : new AreaFlow(this.placement, (area) =>
          this.api.previewSpellArea(
            this.data.campaignId,
            this.data.encounterId,
            this.data.casterId,
            this.data.spellKey,
            this.slotRef(),
            area,
          ),
        );
  protected readonly areaShape: AreaShape = {
    shape: this.data.targets?.areaShape ?? SpellAreaShape.UNSPECIFIED,
    sizeFt: this.data.targets?.areaSizeFt ?? 0,
    widthFt: this.data.targets?.areaWidthFt ?? 0,
  };
  protected readonly stepOne = stepOneTitle(this.placement);
  protected readonly confirmText =
    this.placement === AreaPlacement.CASTER
      ? 'Ver quem está na área'
      : confirmLabel(this.placement);
  protected readonly notPlaced = notPlacedReason(this.placement);
  protected readonly combatants = computed(() => this.data.state.encounter()?.combatants ?? []);
  protected readonly casterLabel = computed(
    () => this.combatants().find((c) => c.id === this.data.casterId)?.label ?? '',
  );
  protected readonly areaRows = computed(() => {
    const p = this.flow?.preview();
    return p ? areaRows(p.targets, p.coverCounts, saveAbility(this.details()), this.placement) : [];
  });
  protected readonly areaCount = computed(() => countText(this.flow?.preview()?.targets ?? []));
  protected readonly areaWarning = computed(() => {
    const p = this.flow?.preview();
    return p ? allyWarning(p.targets, this.data.name, this.casterLabel()) : null;
  });
  /** "Ninguém que você vê está na área." (on the map) or "Ninguém está marcado." (the list), with what it spends. */
  protected readonly nobodyLine = computed(() =>
    nobodyText(
      !!this.flow,
      this.data.level > 0 ? this.slotLevel() : 0,
      this.data.economy === ActionEconomy.BONUS_ACTION,
    ),
  );
  protected readonly coverNote = computed(() => {
    const p = this.flow?.preview();
    if (!p || p.targets.length === 0) {
      return '';
    }
    return p.coverCounts
      ? `A cobertura é medida do ponto de origem, não de você, e só conta quando o teste é de Destreza (como o ${article(this.data.name) === 'a' ? 'da' : 'do'} ${this.data.name}).`
      : '';
  });
  /** The area waits for the caster's "Conjurar mesmo assim" (nobody in it) before the cast can be sent. */
  protected readonly areaAsks = computed(
    () => !!this.flow && this.flow.nobody() && !this.flow.nobodyAccepted(),
  );
  /** A list spell that may hit nobody, with nobody ticked: "Conjurar" asks in place first (PM-02d state 13). */
  protected readonly listAsking = signal(false);
  private readonly areaKey = new ActionKey();
  private readonly listTitle = viewChild<AreaStep>('listTitle');
  private readonly changePlace = viewChild('changePlace', { read: ElementRef<HTMLButtonElement> });
  private readonly backToList = viewChild('backToList', { read: ElementRef<HTMLButtonElement> });

  /** The slot of the cast, as the requests take it. */
  private slotRef(): { level: number; pact: boolean } | null {
    const slot = this.slot();
    return this.data.level > 0 && slot ? { level: slot.level, pact: slot.pact } : null;
  }
  /** Magic Missile's count, in the footer next to the button: it states what is missing, and when all are placed. */
  protected readonly dartsLine = computed(() => {
    if (this.rule().kind !== 'darts' || this.dartsTotal() === 0) {
      return '';
    }
    const n = this.dartsTotal();
    return dartsStatus(n, this.dealt());
  });
  /** The targets chosen for a spell attack, with the circumstances of each. */
  private readonly attacked = computed(() =>
    (this.data.targets?.targets ?? []).filter((t) => this.chosen().includes(t.combatantId)),
  );
  /** The targets suggest different modes: the pick, if any, counts for all of them. */
  protected readonly mixed = computed(
    () => new Set(this.attacked().map((t) => orNormal(t.rollMode))).size > 1,
  );
  protected readonly suggested = computed(() =>
    orNormal(this.attacked()[0]?.rollMode ?? RollMode.NORMAL),
  );
  protected readonly sources = computed(() =>
    this.attacked().length === 1 ? this.attacked()[0].sources : [],
  );
  protected readonly approval = this.data.master ? 'free' : 'blocked';
  protected readonly chosenMode = computed(() => this.picked() ?? this.suggested());
  protected readonly modeState = computed(() =>
    this.mixed() && this.picked() === null
      ? 'ready'
      : modeStatus(this.approval, this.suggested(), this.chosenMode(), this.reason(), null),
  );
  /** The d20 of the attack may be rolled: the mode is settled. */
  protected readonly modeReady = computed(() => this.modeState() === 'ready');
  protected readonly faceCount = computed(() => d20Count(this.chosenMode()));
  protected readonly d20Fields: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];
  protected readonly combine = computed(() =>
    this.chosenMode() === RollMode.DISADVANTAGE ? 'lower' : 'higher',
  );

  protected setMode(mode: RollMode): void {
    this.picked.set(mode);
    this.castKey = newKey();
  }

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
    if (this.hpDone() || (this.flow && this.done())) {
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
      if (this.flow) {
        return this.areaResultLine(circle, n);
      }
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
      this.data.cantripDice,
      this.damageType(),
      this.flow ? shapeText(this.areaShape) : '',
    );
  });

  /** "3º nível · 4 criaturas · teste de resistência de Destreza (CD 16)": what an area hit, counted from the cast's own targets
   * (a player's never holds a hidden creature, RN-10). */
  private areaResultLine(circle: string, n: number): string {
    const parts = [circle, `${n} ${n === 1 ? 'criatura' : 'criaturas'}`];
    const ability = saveAbility(this.details());
    if (ability) {
      const dc = this.cast()?.targets.find((t) => (t.save?.dc ?? 0) > 0)?.save?.dc ?? 0;
      parts.push(`teste de resistência de ${ability}${dc > 0 ? ` (CD ${dc})` : ''}`);
    }
    return parts.filter((p) => p !== '').join(' · ');
  }

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
    if (c.metamagicKeys.length > 0) {
      const names = c.metamagicKeys.map(
        (k) => this.metaOptions.find((o) => o.key === k)?.namePt ?? 'Metamagia',
      );
      out.push(`${metamagicSpentLine(names, c.sorceryPointsSpent, this.sorceryPoints)}.`);
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
    // A sphere around the caster has nothing to place: its list is asked for at once.
    void this.flow?.start();
    // Step 2 opens on its title; with nobody inside, on "Mudar o local", the safe answer.
    effect(() => {
      const step = this.flow?.step();
      const asks = this.areaAsks();
      if (step !== 'list') {
        return;
      }
      untracked(() =>
        afterNextRender(
          () => (asks ? this.changePlace()?.nativeElement.focus() : this.listTitle()?.focus()),
          { injector: this.injector },
        ),
      );
    });
    // "Ninguém está marcado.": the focus goes to "Voltar à lista".
    effect(() => {
      if (this.listAsking()) {
        untracked(() =>
          afterNextRender(() => this.backToList()?.nativeElement.focus(), {
            injector: this.injector,
          }),
        );
      }
    });
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

  protected pickAbility(key: string): void {
    this.ability.set(key);
    this.choiceChanged();
    this.error.set('');
  }

  protected pickDamageType(key: string): void {
    this.pickedType.set(key);
    this.choiceChanged();
    this.error.set('');
  }

  protected pickSlot(row: SlotRow): void {
    this.slot.set(row);
    this.choiceChanged();
    this.error.set('');
  }

  protected toggleMeta(key: string): void {
    this.chosenMeta.set(toggledOption(this.chosenMeta(), key));
    this.choiceChanged();
    this.error.set('');
  }

  protected setPicks(picks: MetamagicPicks): void {
    this.picks.set(picks);
    this.choiceChanged();
    this.error.set('');
  }

  /** "Sem Metamagia": the spell is cast as it is. */
  protected noMetamagic(): void {
    this.chosenMeta.set([]);
    this.picks.set(NO_PICKS);
    this.choiceChanged();
  }

  /** The creatures the options of Metamagic can pick among: the spell's own targets that can be chosen. */
  protected readonly metaCandidates = computed(() => this.targetRows().filter((t) => !t.blocked));

  protected toggle(id: string): void {
    this.listAsking.set(false);
    this.chosen.set(toggled(this.rule(), this.chosen(), id));
    // A creature no longer a target cannot stay the second target or the one with disadvantage.
    this.picks.update((p) => ({
      twinned: p.twinned === this.chosen()[0] ? '' : p.twinned,
      careful: p.careful,
      heightened: this.chosen().includes(p.heightened) ? p.heightened : '',
    }));
    this.choiceChanged();
    this.error.set('');
  }

  protected deal(change: { id: string; delta: 1 | -1 }): void {
    this.dealt.set(dealOne(this.dealt(), change.id, change.delta, this.dartsTotal()));
    this.choiceChanged();
    this.error.set('');
  }

  /** "Conjurar X" (a spell with no attack roll). A list that may hit nobody, with nobody ticked, asks in place first. */
  protected castNow(): Promise<void> {
    const r = this.rule();
    if (
      !this.flow &&
      r.kind === 'multi' &&
      r.min === 0 &&
      this.chosen().length === 0 &&
      !this.listAsking()
    ) {
      this.listAsking.set(true);
      return Promise.resolve();
    }
    return this.doCast(null);
  }

  /** "Conjurar mesmo assim" over an area with nobody the caster sees: a spell that rolls a pool goes on to its roll. */
  protected castAnyway(): Promise<void> {
    if (this.flow) {
      this.flow.nobodyAccepted.set(true);
      return this.usesPicker() ? Promise.resolve() : this.doCast(null);
    }
    return this.doCast(null);
  }

  /** "Confirmar local" (or the second tap): the list of who is inside. */
  protected confirmArea(): Promise<boolean> {
    return this.flow ? this.flow.confirm() : Promise.resolve(false);
  }

  /** "Mudar o local": back to the map, the point where it was. */
  protected changeArea(): void {
    this.flow?.back();
  }

  protected castRolled(die: AttackDie): Promise<void> {
    return this.doCast(die);
  }

  /** Two physical d20, in the order they were rolled. */
  protected castPair(faces: number[]): Promise<void> {
    return this.doCast({ faces });
  }

  /** A pool spell with the dice the table's mode allows: rolled in the app, or the typed sum. */
  protected castPooled(die: PoolDie): Promise<void> {
    return this.doCast(die);
  }

  private async doCast(die: AttackDie | PoolDie | PairDie | null): Promise<void> {
    if (this.busy() || !this.ready()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const slot = this.slotRef();
      const area = this.flow ? this.flow.choice() : null;
      const roll = die ?? (this.kind() === 'pool' ? { inApp: true as const } : null);
      // A placed area's key follows its request: the same slot, place and roll again is a retry of it.
      const key = this.flow
        ? this.areaKey.keyFor([slot, area, roll, this.damageType(), this.sculptedIds()])
        : this.castKey;
      const res = await this.api.castSpell(
        this.data.campaignId,
        this.data.encounterId,
        this.data.casterId,
        this.data.spellKey,
        slot,
        this.castTargets(),
        // A pool is always rolled by someone: the server does it when the app rolls every die.
        roll,
        key,
        undefined,
        this.damageType(),
        this.kind() === 'attack' && this.picked() !== null
          ? { mode: this.picked()!, reason: this.reason().trim() }
          : undefined,
        metamagicChoices(this.chosenMeta(), this.picks()),
        {
          ...(this.flow ? { area } : {}),
          ...(this.asksAbility ? { abilityKey: this.ability() } : {}),
          ...(this.sculptedIds().length ? { sculptedIds: this.sculptedIds() } : {}),
        },
      );
      this.showCast(res.encounter, res.cast);
    } catch (err) {
      if (!this.asks(err)) {
        this.error.set(combatErrorMessage(err, 'conjurar a magia'));
      }
    } finally {
      this.busy.set(false);
    }
  }

  /** A cast that answered a question instead of the cast (nothing was spent, the d20 is kept on the server): the sheet
   * shows it, Lucky's about a 1 first, else the Bardic Inspiration die's. False for any other error. */
  private asks(err: unknown): boolean {
    const question =
      err instanceof LuckyQuestion || err instanceof InspirationQuestion ? err : null;
    if (!question) {
      return false;
    }
    this.data.state.apply(question.encounter);
    if (question instanceof LuckyQuestion) {
      this.luckyOffer.set(question.offer);
    } else {
      this.offer.set(question.offer);
    }
    this.typing.set(false);
    // The d20 alone, no total and no word: the result is told after the answer.
    const held = showOfDice(this.data.name, question.offer.d20);
    if (held) {
      this.animator.play(held);
    }
    return true;
  }

  /** The cast as the result step shows it. */
  private showCast(encounter: Encounter, cast: SpellCast): void {
    this.data.state.apply(encounter);
    this.cast.set(cast);
    this.animateCast(cast);
    this.pendings.set(new Map(cast.pendingDamages.map((p) => [p.id, p])));
    this.typing.set(false);
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the spell is cast now with the new d20
   * (or the 1), or, when the caster holds a Bardic Inspiration die, the die's question comes next, on the new roll. */
  protected async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideLucky(
        this.data.campaignId,
        offer.holdId,
        reroll,
        die,
        this.luckyKey.keyFor({ hold: offer.holdId, reroll, die }),
      );
      const settled = res.result.case === 'spellCast' ? res.result.value : null;
      if (!settled?.encounter || (!settled.cast && !settled.inspirationOffer)) {
        throw new Error('AnswerOutsideLucky answered without its cast');
      }
      this.luckyKey.renew();
      this.luckyOffer.set(null);
      this.typing.set(false);
      if (settled.inspirationOffer) {
        this.data.state.apply(settled.encounter);
        this.offer.set(settled.inspirationOffer);
        this.animateLucky(settled.inspirationOffer.d20, die);
        return;
      }
      this.showCast(settled.encounter, settled.cast!);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The d20 the app rolled again for Lucky, tumbling, when the die's question comes next (a cast that is told animates
   * its own d20): the new face alone, "1 → 14". */
  private animateLucky(roll: DiceRoll | undefined, die: LuckyRoll | null): void {
    const again = die && 'inApp' in die ? luckyReroll(roll) : undefined;
    const show = again
      ? showOfDie('Sortudo', roll?.diceSides ?? 0, again.to, `${again.from} → ${again.to}`)
      : null;
    if (show) {
      this.animator.play(show);
    }
  }

  /** The answer about the die: the spell is cast now, with the d20 it rolled and the die when it was used. */
  protected async answerDie(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideInspiration(
        this.data.campaignId,
        offer.holdId,
        use,
        die,
        this.answerKey.keyFor({ hold: offer.holdId, use, die }),
      );
      if (
        res.result.case !== 'spellCast' ||
        !res.result.value.encounter ||
        !res.result.value.cast
      ) {
        throw new Error('AnswerOutsideInspiration answered without its cast');
      }
      this.offer.set(null);
      this.showCast(res.result.value.encounter, res.result.value.cast);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  /**
   * The dice the cast just rolled in the app, as the result step shows them: the pool of a spell that reads hit points, or the
   * spell attack d20 (one die per target in the same overlay; with a single target, the total and the word too). The targets'
   * saving throws are not animated: they are not the caster's roll.
   */
  private animateCast(cast: SpellCast): void {
    const name = this.data.name;
    let show: RollShow | null = showOfDice(name, cast.poolRoll, { withTotal: true });
    if (!show) {
      const attacks = cast.targets.filter((t) => t.attackRoll);
      const told = (t: (typeof attacks)[number]) => t.outcome !== AttackOutcome.UNSPECIFIED;
      if (attacks.length === 1) {
        const t = attacks[0];
        const hit = isHit(t.outcome);
        show = showOfDice(`Ataque com ${name}`, t.attackRoll, {
          withTotal: true,
          outcome: told(t) ? { word: outcomeWord(t.outcome), good: hit } : undefined,
          critical: t.outcome === AttackOutcome.CRITICAL_HIT,
          fumble: told(t) && !hit,
        });
      } else if (attacks.length > 1) {
        const dice = attacks.flatMap((t) => showOfDice(name, t.attackRoll)?.dice ?? []);
        show = dice.length > 0 ? { label: `Ataque com ${name}`, dice } : null;
      }
    }
    if (show) {
      this.animator.play(show);
    }
  }

  /** Who the cast names: nobody for a placed area (the server works it out), the darts, or the targets ticked. */
  private castTargets(): { combatantId: string; darts: number }[] {
    if (this.flow || this.rule().kind === 'none') {
      return [];
    }
    return this.rule().kind === 'darts'
      ? dartTargets(this.dealt())
      : this.chosen().map((combatantId) => ({ combatantId, darts: 0 }));
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
      const show = showOfDice('Dano', res.pending.roll, {
        withTotal: true,
        note: res.pending.damageTypePt || undefined,
      });
      if (show) {
        this.animator.play(show);
      }
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
