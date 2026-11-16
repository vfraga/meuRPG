import { MASTER_WAIT_NOTE_ID, isMasterWait } from '../../../../core/combat/reactions';
import { NgTemplateOutlet } from '@angular/common';
import { Component, computed, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant, PendingDamage } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { ExtraActionOption } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import {
  type ContestAttackOption,
  type ContestTurnState,
  ContestAttackOptionKind,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import {
  ActionEconomy,
  type ActionOption,
  type Attack,
  type AttackOption,
  type SpellDetails,
  type SpellOption,
  type TurnOptions,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { AMMUNITION } from '../../../../core/combat/ammunition-line';
import {
  throwNote,
  attackName,
  attackMarksOf,
  attackTagsOf,
  beamsLeftLine,
  bonusAttackLine,
  groupOf,
  groupState,
  isBonusAttack,
  isCantrip,
  magicBonusOf,
  reasonText,
  spellLine,
  spellTags,
} from '../../../../core/combat/combat-options';
import { tradeLinesOf } from '../../../../core/combat/power-attack-trade';
import {
  type ActionTabId,
  type SpentGroups,
  actionCounts,
  actionTabs,
  WILD_SHAPE_TAB,
  classTabName,
  featureGroup,
  featureVerb,
  partByEconomy,
} from '../../../../core/combat/action-tabs';
import { cannotAct } from '../../../../core/combat/conditions';
import { spellSummary } from '../../../../core/combat/spell-summary';
import {
  ESCAPE_DETAIL,
  hiddenAttackNote,
  SHOVE_ATTACK_NOTE,
  SPECIAL_ATTACKS_NOTE,
  hiddenWord,
} from '../../../../core/combat/contest-view';
import { castRefOf } from '../../../../core/combat/cast-ref';
import { standUpRow } from '../../../../core/combat/stand-up';
import { type ListedDie, maneuverLine } from '../../../../core/effects/roll-die';
import type { GearFlow } from '../../../../core/combat/gear-flow';
import { GearFold } from './gear-fold';
import { HitEffectCards } from './hit-effect-cards';
import { hitCards } from '../../../../core/effects/hit-effect-text';
import { AttackLegend } from './attack-legend';
import { Fold } from '../../../../shared/fold/fold';
import { EconomyShapes } from '../../../../shared/economy-shapes/economy-shapes';
import { movementOf, turnLineOf } from './turn-line';
import { leaveDetailOf, SURPRISE_ROWS } from './surprise-and-form-text';
import { holdsReadied, isReadyOption, withoutReady } from '../../../../core/combat/ready';
import { ReadiedCard } from '../readied-card/readied-card';
import { TabList, tabId, tabPanelId } from '../../../../shared/tab-list/tab-list';
import type { PactSlotsVm, SlotUsageVm } from '../../../../core/play/resource-usage.types';
import { SlotDots } from '../../../../shared/slot-dots/slot-dots';
import { ActionRow, type RowEconomy } from './action-row';
import { featureIsOff, featureLine, featureWhy } from '../../../../core/combat/action-spend';
import { resourceOf, useLine } from '../../../../core/combat/feature-use';
import type { ResourceUsageVm } from '../../../../core/play/resource-usage.types';
import { GroupState } from './group-state';
import {
  FRENZY_QUESTION,
  actionWordOf,
  extraStandardsOf,
  extraWeaponsOf,
  TAB_ICONS,
  slotRowsOf,
  specialAttack,
  attackLines,
} from './action-groups-text';
import { offersWeaponAttack } from './weapon-offer';

/**
 * "O que você pode fazer" (E6-06 on a phone, E6-14 on a laptop): the options
 * `GetTurnOptions` works out, grouped by economy. Each group header carries
 * its state word (the open circle and "Disponível", or "Usada"). Ação has
 * the attacks, the spells and the standard actions; Ação bônus and Reação
 * the features of their economy; Movimento, "Mover". The spells of every
 * economy are one list in Ação, in the order the server sends (the ones that can
 * be cast now first, then by circle and name: it is not sorted again here), each
 * with the "?" that opens its description (`describe`) and a tag for the economy
 * when it is not an action; the slot rows above them are the one explanation of
 * every "Sem espaço". A spell row has "Conjurar" (the cast sheet, `cast`; a
 * cantrip that asks for a saving throw is cast too, not attacked), a feature row
 * "Usar" (`feature`, a TakeAction); Escudo, a reaction, has none: it asks when a
 * hit lands, and on the character's own turn it says "Só fora da sua vez".
 * With Extra Attack the Ação header says how many attacks are left and the
 * attacks stay enabled. Disabled options keep their place with the reason, by
 * code (`reasonText`).
 */
@Component({
  selector: 'app-action-groups',
  imports: [
    EconomyShapes,
    ActionRow,
    AttackLegend,
    Fold,
    GearFold,
    GroupState,
    HitEffectCards,
    MatButtonModule,
    MatIconModule,
    NgTemplateOutlet,
    ReadiedCard,
    SlotDots,
    TabList,
  ],
  templateUrl: './action-groups.html',
  styleUrl: './action-groups.scss',
})
export class ActionGroups {
  readonly options = input.required<TurnOptions>();
  /** The player's gear in the combat (`GearFlow`), for "Sacar ou guardar" under the attacks; none outside the player's own turn screen. */
  readonly gearFlow = input<GearFlow | null>(null);

  /** The key of the action whose target the player is choosing on the map (C7-05): its row says "Escolhendo o alvo". */
  readonly choosing = input('');
  /** The player's own combatant: its economy and movement. */
  readonly own = input.required<Combatant>();
  /** Everyone in the combat, to name the creature a marked-target effect is on. */
  readonly combatants = input<readonly Combatant[]>([]);
  /** Extra Attack: the attacks of this Attack action that remain, and how many
   * it makes (`TurnEconomy`). */
  readonly attacksLeft = input(0);
  readonly attacksPerAction = input(1);
  /** What the last feature said ("Você tem outra ação"), in a live region. */
  readonly note = input('');
  /** A hit of this turn whose damage is still to roll. */
  readonly pendingRoll = input<PendingDamage | null>(null);
  readonly busy = input(false);
  /** Why nothing can be done now ("Esperando a reação do mestre"): every row is off with this
   * reason instead of failing on click (an opportunity attack waits for its answer); `''` when free. */
  readonly locked = input('');
  /** The lock is the master's wait: the turn card says it once, and what is off points to that card. */
  protected readonly masterWait = computed(() => isMasterWait(this.locked()));
  protected readonly waitNoteId = MASTER_WAIT_NOTE_ID;
  /** The line under "Ações padrão": the lock, unless it is the master's wait (the turn card says that once), else why one of them is off. */
  protected readonly standardReason = computed(() =>
    this.masterWait() ? this.standardWhy() : this.locked() || this.standardWhy(),
  );
  /** The player's familiar ('Nanquim'): "Ver pelos olhos do Nanquim" is an action (MR-036, E9-04). `null`: none, or already looking. */
  readonly familiar = input<string | null>(null);
  /** The character's slots, for the rows above the spells ("1º nível ○ ✕ ✕ ✕ 1 livre de 4"). */
  readonly slots = input<{
    readonly usage: readonly SlotUsageVm[];
    readonly pact: PactSlotsVm | null;
  }>({
    usage: [],
    pact: null,
  });

  /** What `GetSpellDetails` said about each spell of the list, for the line under its name. */
  readonly details = input<ReadonlyMap<string, SpellDetails>>(new Map());
  /**
   * The druid's Wild Shape as a line of the Ação (MR-037, E9-11 state 1): the line under its name ("Vire uma fera
   * · restam 2 de 2 usos · volta no descanso curto ou longo") and, when it cannot be used, the reason. `null` for a
   * character without it, or already in a beast form (then `leaveForm` is the bonus action).
   */
  readonly wild = input<{
    readonly key: string;
    readonly detail: string;
    readonly reason: string;
  } | null>(null);
  /** The character's resources, for the name of the one a feature spends from ("gasta 1 uso de Forma Selvagem"). */
  readonly resources = input<readonly ResourceUsageVm[]>([]);
  /** The character directs a mount and the screen is 768 px or wider (588-04): the pair card, projected in `[pair]`, comes
   * first and holds the character's shapes, so the economy row keeps only "Mover". On a phone the row is whole (588-05). */
  readonly shapesAside = input(false);
  /** The pair card comes right under the economy row (a phone, 588-05): the row has no divider and no room of its own under it. */
  readonly paired = input(false);
  /** The master acts for the character: "Mover" is in the map's header (C8-01), so the economy row does not repeat it. */
  readonly moveAside = input(false);
  /** "Permanecer na forma": what it costs and why it cannot be pressed ('' when it can); `null` outside a form. */
  readonly stay = input<{ readonly detail: string; readonly reason: string } | null>(null);
  /** The beast the character is in ("Lobo"): no spells, and "Voltar à forma normal" is a bonus action. */
  readonly beast = input('');
  /** The combat is played without a map (RN-25): Movimento says so and its button is "Gastar movimento". */
  readonly theatre = input(false);
  /** The special attacks of the Attack action, Agarrar and Empurrar (`GetTurnOptions.contest_attack_options`, W7-X). */
  readonly contestAttacks = input<readonly ContestAttackOption[]>([]);
  /** The contest facts of the turn: grappled (and who can escape), hidden, surprised (W7-X). */
  readonly contest = input<ContestTurnState | undefined>(undefined);

  /** The table dice (Golpe Certeiro) a weapon attack of the player's own turn may take, with the uses left (`GetTurnOptions.maneuver_dice`); none for an NPC. */
  readonly maneuverDice = input<readonly ListedDie[]>([]);

  /** The extra action an effect gives (Velocidade): its line, what it may be and why it is not there; `null` without one. */
  readonly extraAction = input<ExtraActionOption | null>(null);

  /** "Atacar": the key of the attack, as in `Attack.key`. */
  readonly attack = output<string>();
  /** "Agarrar" or "Empurrar": the sheet that picks the target. */
  readonly contestAttack = output<ContestAttackOptionKind>();
  /** "Escapar" of a grappled combatant. */
  readonly escape = output<void>();
  /** The weapon attack of the extra action: the key of the attack. */
  readonly extraAttack = output<string>();
  /** A standard action paid with the extra action ("standard:dash"). */
  readonly extraStandard = output<string>();
  /** A standard action, by key ("standard:dash"). */
  readonly action = output<string>();
  /** "Preparar" (#582): the sheet of the Ready action, which names the trigger. */
  readonly ready = output<void>();
  /** "Desistir da ação preparada": the id of the readied action to give up (`CancelReadied`). */
  readonly giveUp = output<string>();
  /** Whether the combatant holds an action for its trigger: its card sits under the economy row, in every tab. */
  protected readonly holdsReadied = holdsReadied;
  /** "Conjurar": the key of the spell, or of a cantrip that asks for a save. */
  readonly cast = output<string>();
  /** "Usar": the key of a feature action ("feature:second-wind"). */
  readonly feature = output<string>();
  /** The Berserker's answer to "Entrar em frenesi?": the key of Fúria and whether the rage is a frenzy. */
  readonly rage = output<{ readonly key: string; readonly frenzy: boolean }>();
  /** The Fúria whose question "Entrar em frenesi?" is open, in place of the next tap. */
  protected readonly frenzyAsk = signal<string | null>(null);
  protected readonly frenzyText = FRENZY_QUESTION;
  readonly move = output<void>();
  /** "Levantar-se" of a prone combatant (half the speed). */
  readonly standUp = output<void>();
  /** "Rolar o dano" of a hit that waits for its roll. */
  readonly rollDamage = output<void>();
  /** "Ver pelos olhos do ...": the page asks the question. */
  readonly familiarEyes = output<void>();
  /** The "?" of a spell: the key and the Portuguese name. */
  readonly describe = output<{ key: string; name: string }>();
  protected readonly leaveDetail = computed(() => leaveDetailOf(this.own().label, this.beast()));

  /** "Transformar": the page opens the list of beasts. */
  readonly transform = output<void>();
  /** "Voltar" (to the normal shape), a bonus action. */
  readonly leaveForm = output<void>();
  /** "Permanecer na forma": one more use, no action, the time starts over. */
  readonly stayForm = output<void>();

  /** The ids of the tab buttons and of their panels start with this. */
  protected readonly prefix = 'acts';
  /** The tab the person chose; the first one until they choose (it lives here, in memory). */
  protected readonly tab = signal('');

  protected readonly surprised = computed(() => this.contest()?.surprised === true);
  protected readonly grappled = computed(() => this.contest()?.grappled === true);
  protected readonly hiddenNote = computed(() =>
    this.contest()?.hidden ? hiddenAttackNote(hiddenWord(this.own().label)) : '',
  );
  protected readonly specialNote = computed(() =>
    this.beast() ? SHOVE_ATTACK_NOTE : SPECIAL_ATTACKS_NOTE,
  );
  protected readonly surpriseRows = SURPRISE_ROWS;
  protected readonly escapeDetail = ESCAPE_DETAIL;
  protected readonly escapeWhy = computed(() => reasonText(this.contest()?.escapeReason));
  /** The word of the attack list for Agarrar and Empurrar. */
  protected readonly special = specialAttack;
  /** Spent this turn, by economy: its rows fold to one line. With Extra Attack the Action stays open while attacks remain. */
  protected readonly spent = computed<SpentGroups>(() => {
    const own = this.own();
    const cannot = cannotAct(own);
    const partial = this.attacksLeft() > 0 && this.attacksPerAction() > 1;
    return {
      action: cannot || (own.actionUsed && !partial),
      bonus: cannot || own.bonusActionUsed,
      reaction: cannot || own.reactionUsed,
    };
  });
  /** A row the server still offers (a follow-up strike, the next attack) is never folded away as spent. */
  private readonly canUse = (row: { readonly enabled: boolean }) =>
    row.enabled && !cannotAct(this.own());
  protected readonly spentWord = computed(() => (cannotAct(this.own()) ? 'Indisponível' : 'Usada'));
  protected readonly attackParts = computed(() =>
    partByEconomy(
      this.options().attacks,
      (a) => (isBonusAttack(a) ? 'bonus' : 'action'),
      this.spent(),
      this.spentWord(),
      this.canUse,
    ),
  );
  /** The effects waiting for the caster's next weapon hit (#577), the card at the top of "Ataques". */
  protected readonly hitEffects = computed(() => hitCards(this.own(), this.combatants()));
  /** The one line above the weapon rows: only with dice and at least one weapon attack still on offer. */
  protected readonly maneuverNote = computed(() =>
    offersWeaponAttack(this.attackParts().open) ? maneuverLine(this.maneuverDice(), 'list') : '',
  );
  protected readonly spellParts = computed(() =>
    partByEconomy(
      this.spells(),
      (s) => groupOf(s.economy),
      this.spent(),
      this.spentWord(),
      this.canUse,
    ),
  );
  /** The class's abilities, in the order the server sends them: all of an economy but the form's own way out (the beast has "Voltar à forma normal"). */
  protected readonly classFeatures = computed(() =>
    this.options().featureActions.filter(
      (f) => !(this.beast() && f.action?.key?.startsWith('feature:wild-shape')),
    ),
  );
  protected readonly featureParts = computed(() =>
    partByEconomy(this.classFeatures(), featureGroup, this.spent(), this.spentWord(), this.canUse),
  );
  protected readonly hasSpells = computed(() => this.spells().length > 0 && !this.beast());
  protected readonly hasClassTab = computed(
    () => this.classFeatures().length > 0 || !!this.beast(),
  );
  /** What a form's class tab holds besides the class's abilities: "Voltar à forma normal" and, while it is on, "Permanecer na forma". */
  private readonly formRows = computed(() => (this.beast() ? 1 + (this.stay() ? 1 : 0) : 0));
  /**
   * In a form the attacks are the beast's own (its bite), plus the shove: any creature shoves with the Attack action (SRD 5.1), but a
   * grapple needs a free hand and a beast has none. Nothing is drawn or put away.
   */
  protected readonly contestRows = computed(() =>
    this.beast()
      ? this.contestAttacks().filter((c) => c.kind !== ContestAttackOptionKind.GRAPPLE)
      : this.contestAttacks(),
  );
  protected readonly tabItems = computed(() => {
    const counts = actionCounts(this.options(), {
      contestAttacks: this.contestRows().length,
      escape: this.grappled(),
      standard: this.standard().length,
    });
    return actionTabs(
      { ...counts, features: this.classFeatures().length + this.formRows() },
      this.beast() ? WILD_SHAPE_TAB : classTabName(this.classFeatures()),
      { beast: !!this.beast() },
    ).map((t) => ({
      id: t.id,
      label: t.label,
      icon: TAB_ICONS[t.id],
      badge: t.badge,
      words: t.words,
    }));
  });
  /** The tab shown: the person's choice while it is still there, else the first. */
  protected readonly current = computed<ActionTabId>(() => {
    const items = this.tabItems();
    const chosen = items.find((t) => t.id === this.tab());
    return (chosen ?? items[0]).id as ActionTabId;
  });
  protected panelId(id: string): string {
    return tabPanelId(this.prefix, id);
  }
  protected tabButtonId(id: string): string {
    return tabId(this.prefix, id);
  }
  protected readonly isBonusAttack = isBonusAttack;
  /** The mark of a spell's or a feature's economy. */
  protected economyOf(economy: ActionEconomy): RowEconomy | null {
    return groupOf(economy);
  }
  protected featureEconomy(a: ActionOption): RowEconomy | null {
    return a.action?.economy === ActionEconomy.FREE ? 'free' : groupOf(a.action?.economy ?? 0);
  }
  protected isReaction(a: ActionOption): boolean {
    return a.action?.economy === ActionEconomy.REACTION;
  }
  protected readonly standRow = computed(() => standUpRow(this.own()));
  /** Every spell, whatever its economy, as the server ordered them. */
  protected readonly spells = computed(() => this.options().spells);
  /** One row for each circle the character has slots in, then the pact slots. */
  protected readonly slotRows = computed(() => slotRowsOf(this.slots()));
  /** Atacar and Conjurar are not repeated here: they are the rows above. */
  protected readonly standard = computed(() =>
    this.options().standardActions.filter(
      (a) => !/^standard:(attack|cast-a-spell)$/.test(a.action?.key ?? ''),
    ),
  );
  /** "Preparar" (`via_ready_action`) is a row of its own, with its sheet; the others stay the buttons below it. */
  protected readonly readyOption = computed(() => this.standard().find((a) => isReadyOption(a)));
  protected readonly standardButtons = computed(() => withoutReady(this.standard()));
  protected readonly standardWhy = computed(() => {
    const first = this.standardButtons().find((a) => !a.enabled);
    return first ? reasonText(first.reason) : '';
  });
  /** The first row of the panel: what is left of the turn (Ação, Bônus, Reação and the walker with the metres), and "Mover". */
  protected readonly turnLine = computed(() =>
    turnLineOf(this.own(), {
      grappled: this.grappled(),
      blocked: this.theatre() || this.surprised() || this.moveAside(),
      attacksLeft: this.attacksLeft(),
      attacksPerAction: this.attacksPerAction(),
    }),
  );
  protected readonly movement = computed(() => movementOf(this.own(), this.grappled()));

  /** The word on the Ação header: with Extra Attack, once the first attack
   * spent the action, "1 ataque restante" (an open circle: it is not over). */
  protected readonly cannot = computed(() => cannotAct(this.own()));
  protected readonly actionWord = computed(() =>
    actionWordOf(this.own(), this.attacksLeft(), this.attacksPerAction()),
  );

  protected readonly attackName = attackName;

  /** What the extra action of an effect (Velocidade) may be: one weapon attack, or a standard action but the attack. */
  protected readonly extraWeapons = computed(() =>
    extraWeaponsOf(this.extraAction(), this.options().attacks),
  );
  protected readonly extraStandards = computed(() => extraStandardsOf(this.extraAction()));

  /** The line of why under an attack: its bonus action rule, or the beams of a cast still to fire. */
  protected attackNote(o: AttackOption): string {
    return (
      bonusAttackLine(o) ||
      beamsLeftLine(o) ||
      (o.attack ? throwNote(o.attack) || AMMUNITION.note(o.attack, this.options().attacks) : '')
    );
  }

  /** The line of each power attack trade the server lists on a weapon ("Golpe Pesado: −5 no ataque, +10 no dano"). */
  protected readonly powerLines = tradeLinesOf;
  protected readonly ammo = AMMUNITION;
  protected readonly countsAmmo = computed(() => AMMUNITION.counted(this.options().attacks));
  protected readonly attackTags = attackTagsOf;
  protected readonly attackMarks = attackMarksOf;
  /** The counting magic bonus the server says is in an attack's numbers. */
  protected readonly bonusOf = magicBonusOf;
  protected readonly lines = attackLines;
  protected readonly reasonText = reasonText;
  protected readonly castRef = castRefOf;
  protected readonly state = (used: boolean): string =>
    cannotAct(this.own()) ? 'Indisponível' : groupState(used);
  protected readonly isCantrip = isCantrip;

  /** The circle and "Concentração"/"Ritual"; the economy is the row's mark, not a tag. */
  protected spellTags(s: SpellOption): string[] {
    return spellTags(s).filter((t) => t !== 'Ação bônus' && t !== 'Reação');
  }

  /** The tags ("1º nível", "Concentração", "Truque") as the first words of the line under the name: the row shows the name, its economy and one line. */
  protected withTags(tags: readonly string[], line: string): string {
    return [...tags, line].filter(Boolean).join(' · ');
  }

  /** The line under a spell's name. */
  protected spellLine(s: SpellOption): string {
    return spellLine(s, spellSummary(this.details().get(s.spell?.key ?? '')));
  }

  /** The name a spell goes by on screen and in an accessible name. */
  protected spellName(s: SpellOption): string {
    return s.spell?.namePt || s.spell?.name || '';
  }

  /** A reaction (Escudo) keeps a dashed, disabled "Conjurar": it asks when its trigger happens. */
  protected spellButton(): string {
    return 'Conjurar';
  }

  /** "Usar" on a bonus action feature: Fúria of a Berserker asks about the frenzy first (in place); the others act. */
  protected pressFeature(f: ActionOption): void {
    const key = f.action?.key ?? '';
    if (f.offersFrenzy) {
      this.frenzyAsk.set(key);
      return;
    }
    this.feature.emit(key);
  }

  /** One of the two buttons of the frenzy question. */
  protected answerFrenzy(key: string, frenzy: boolean): void {
    this.frenzyAsk.set(null);
    this.rage.emit({ key, frenzy });
  }

  protected readonly featureVerb = featureVerb;

  /** Whether a feature is off, why, and the line under its name: what a use costs and what is left (the server's reason, or "Faltam usos"). */
  protected readonly featureOff = featureIsOff;
  protected readonly featureWhy = featureWhy;
  /** The master may use the feature although its part of the turn is spent: the server marked the row, and nothing locks it. */
  protected overridable(a: ActionOption): boolean {
    return a.masterMayOverride && !this.locked();
  }
  /** The line under a feature's name: a feature taken with `UseFeature` says its scope, whom it takes and what it spends. */
  protected featureDetail(a: ActionOption): string {
    return a.viaUseFeature ? useLine(a, resourceOf(a, this.resources())) : featureLine(a);
  }

  /** The feature that is the druid's Wild Shape: it opens the beasts' sheet instead of being "used". */
  protected isWild(a: ActionOption): boolean {
    const wild = this.wild();
    return !!wild && a.action?.key === wild.key;
  }

  /** The SRD's own text of a creature's attack, under it (English); a character's attack has none shown. */
  protected rider(a: Attack): string {
    return a.key.includes('#') ? a.notes : '';
  }

  /** A save cantrip (Chama Sagrada) is in the attacks, but it is cast. */
  protected asksSave(a: Attack): boolean {
    return a.saveDc > 0;
  }
}
