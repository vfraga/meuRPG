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
  AttackKind,
  type SpellDetails,
  type SpellOption,
  type TurnOptions,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { metersFixed, reachSquares, squaresText } from '../../../../core/units';
import { article } from '../../../../core/combat/combat-log';
import { tight } from '../../../../core/format/text';
import {
  attackDetail,
  attackName,
  attackMarksOf,
  attackTagsOf,
  beamsLeftLine,
  bonusAttackLine,
  circleLabel,
  groupOf,
  groupState,
  isBonusAttack,
  isCantrip,
  reasonText,
  spellLine,
  spellTags,
} from '../../../../core/combat/combat-options';
import {
  type ActionTabId,
  type SpentGroups,
  actionCounts,
  actionTabs,
  classTabName,
  featureGroup,
  featureVerb,
  partByEconomy,
} from '../../../../core/combat/action-tabs';
import { cannotAct } from '../../../../core/combat/conditions';
import { spellSummary } from '../../../../core/combat/spell-summary';
import {
  ESCAPE_DETAIL,
  GRAPPLE_DETAIL,
  hiddenAttackNote,
  SHOVE_DETAIL,
  SPECIAL_ATTACKS_NOTE,
  grappledWord,
  hiddenWord,
} from '../../../../core/combat/contest-view';
import { castRefOf } from '../../../../core/combat/cast-ref';
import { freeText } from '../../../../core/combat/cast-flow';
import { standUpRow } from '../../../../core/combat/stand-up';
import { type ListedDie, maneuverLine } from '../../../../core/effects/roll-die';
import type { GearFlow } from '../../../../core/combat/gear-flow';
import { GearFold } from './gear-fold';
import { Fold } from '../../../../shared/fold/fold';
import { TabList, tabId, tabPanelId } from '../../../../shared/tab-list/tab-list';
import type { PactSlotsVm, SlotUsageVm } from '../../../../core/play/resource-usage.types';
import { SlotDots } from '../../../../shared/slot-dots/slot-dots';
import { ActionRow, type RowEconomy } from './action-row';
import { featureIsOff, featureLine, featureWhy } from '../../../../core/combat/action-spend';
import { GroupState } from './group-state';
import { EXTRA_NAMES, FRENZY_QUESTION, TAB_ICONS, shortCircle } from './action-groups-text';
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
    ActionRow,
    Fold,
    GearFold,
    GroupState,
    MatButtonModule,
    MatIconModule,
    NgTemplateOutlet,
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
  /** "Você volta a ser a Sálvia, com os seus PV. Os PV que sobraram do Lobo se perdem.": the character and the beast, by name. */
  protected readonly leaveDetail = computed(() => {
    const name = this.own().label;
    const beast = this.beast();
    const a = (n: string) => (article(n) === 'a' ? 'a' : 'o');
    return `Você volta a ser ${a(name)} ${name}, com os seus PV. Os PV que sobraram d${a(beast)} ${beast} se perdem.`;
  });

  /** "Transformar": the page opens the list of beasts. */
  readonly transform = output<void>();
  /** "Voltar" (to the normal shape), a bonus action. */
  readonly leaveForm = output<void>();

  /** The ids of the tab buttons and of their panels start with this. */
  protected readonly prefix = 'acts';
  /** The tab the person chose; the first one until they choose (it lives here, in memory). */
  protected readonly tab = signal('');

  protected readonly surprised = computed(() => this.contest()?.surprised === true);
  protected readonly grappled = computed(() => this.contest()?.grappled === true);
  protected readonly hiddenNote = computed(() =>
    this.contest()?.hidden ? hiddenAttackNote(hiddenWord(this.own().label)) : '',
  );
  protected readonly specialNote = SPECIAL_ATTACKS_NOTE;
  /** The four groups of a surprised character's turn, each unavailable for the same reason (SRD 5.1, Surprise). */
  protected readonly surpriseRows: readonly { readonly name: string; readonly why: string }[] = [
    { name: 'Atacar', why: 'Surpresa.' },
    { name: 'Movimento', why: 'Surpresa.' },
    { name: 'Ação bônus', why: 'Surpresa.' },
    { name: 'Reação', why: 'Surpresa até o fim do turno.' },
  ];
  protected readonly escapeDetail = ESCAPE_DETAIL;
  protected readonly escapeWhy = computed(() => reasonText(this.contest()?.escapeReason));
  /** The word of the attack list for Agarrar and Empurrar. */
  protected special(o: ContestAttackOption): { name: string; detail: string } {
    return o.kind === ContestAttackOptionKind.SHOVE
      ? { name: 'Empurrar', detail: SHOVE_DETAIL }
      : { name: 'Agarrar', detail: GRAPPLE_DETAIL };
  }
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
  protected readonly hasSpells = computed(() => this.spells().length > 0 || !!this.beast());
  protected readonly hasClassTab = computed(
    () => this.classFeatures().length > 0 || !!this.beast(),
  );
  protected readonly tabItems = computed(() =>
    actionTabs(
      actionCounts(this.options(), {
        contestAttacks: this.contestAttacks().length,
        escape: this.grappled(),
        standard: this.standard().length,
      }),
      classTabName(this.classFeatures()),
      { beast: !!this.beast() },
    ).map((t) => ({
      id: t.id,
      label: t.label,
      icon: TAB_ICONS[t.id],
      badge: t.badge,
      words: t.words,
    })),
  );
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
  protected readonly slotRows = computed(() => {
    const { usage, pact } = this.slots();
    const rows = usage
      .filter((u) => u.total > 0)
      .map((u) => ({
        title: circleLabel(u.level),
        short: shortCircle(u.level),
        total: u.total,
        used: u.used,
        text: freeText(u.total - u.used, u.total),
      }));
    if (pact && pact.total > 0) {
      rows.push({
        title: `${circleLabel(pact.slotLevel)} (pacto)`,
        short: `${shortCircle(pact.slotLevel)} pacto`,
        total: pact.total,
        used: pact.used,
        text: freeText(pact.total - pact.used, pact.total),
      });
    }
    return rows;
  });
  /** Atacar and Conjurar are not repeated here: they are the rows above. */
  protected readonly standard = computed(() =>
    this.options().standardActions.filter(
      (a) => !/^standard:(attack|cast-a-spell)$/.test(a.action?.key ?? ''),
    ),
  );
  protected readonly standardWhy = computed(() => {
    const first = this.standard().find((a) => !a.enabled);
    return first ? reasonText(first.reason) : '';
  });
  protected readonly movement = computed(() => {
    const own = this.own();
    const left = own.movementLeftFt;
    if (this.grappled()) {
      // Grappled: the speed is 0 (SRD 5.1, Conditions), and the line says why.
      return {
        left: 0,
        pill: 'Sem movimento',
        text: `Indisponível: ${grappledWord(own.label)}.`,
      };
    }
    // Tenths of a foot, metres with one decimal ("Restam 6,9 m").
    const leftM = metersFixed(own.movementLeftDft / 10);
    const sentence = `${leftM} (${squaresText(reachSquares(left))})`;
    return {
      left,
      pill: left > 0 ? tight(`Restam ${leftM}`) : 'Sem movimento',
      text:
        left > 0 && own.disengaged
          ? tight(`Dá para andar até ${sentence} sem provocar.`)
          : left > 0
            ? tight(
                own.movementUsedDft > 0
                  ? `Você já andou ${metersFixed(own.movementUsedDft / 10)}. Dá para andar mais ${sentence}.`
                  : `Você ainda não andou. Dá para andar até ${sentence}.`,
              )
            : 'Você já usou todo o movimento deste turno.',
    };
  });

  /** The word on the Ação header: with Extra Attack, once the first attack
   * spent the action, "1 ataque restante" (an open circle: it is not over). */
  protected readonly cannot = computed(() => cannotAct(this.own()));
  protected readonly actionWord = computed(() => {
    if (cannotAct(this.own())) {
      return { word: 'Indisponível', used: true };
    }
    const used = this.own().actionUsed;
    const left = this.attacksLeft();
    if (used && left > 0 && this.attacksPerAction() > 1) {
      return {
        word: `${left} ${left === 1 ? 'ataque restante' : 'ataques restantes'}`,
        used: false,
      };
    }
    return { word: groupState(used), used };
  });

  protected readonly attackName = attackName;

  /** The weapon attacks the extra action may make (one weapon attack, SRD 5.1, Velocidade): no cantrip and no save. */
  protected readonly extraWeapons = computed(() =>
    this.extraAction()?.allowedActions.includes('attack')
      ? this.options().attacks.filter(
          (a) => a.attack?.kind === AttackKind.WEAPON && a.attack.saveDc === 0,
        )
      : [],
  );
  /** The standard actions the extra action may be, but the attack: "Disparada", "Desengajar", "Esconder", "Usar um objeto". */
  protected readonly extraStandards = computed(() =>
    (this.extraAction()?.allowedActions ?? [])
      .filter((k) => k !== 'attack' && k in EXTRA_NAMES)
      .map((k) => ({ key: `standard:${k}`, name: EXTRA_NAMES[k] })),
  );

  /** The line of why under an attack: its bonus action rule, or the beams of a cast still to fire. */
  protected attackNote(o: AttackOption): string {
    return bonusAttackLine(o) || beamsLeftLine(o);
  }

  protected readonly attackTags = attackTagsOf;
  protected readonly attackMarks = attackMarksOf;
  protected readonly attackDetail = (a: Parameters<typeof attackDetail>[0]) => attackDetail(a);
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
  protected readonly featureDetail = featureLine;

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
