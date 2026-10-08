import { NgTemplateOutlet } from '@angular/common';
import {
  Component,
  ElementRef,
  computed,
  effect,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';

import {
  type Combatant,
  CombatantSide,
  CombatantState,
  CoverDegree,
  CoverSource,
  type Encounter,
  EncounterStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { tieNumbers } from '../../../../core/format/text';
import { joinDots } from '../../../../core/format/text';
import { conditionTags } from '../../../../core/combat/conditions';
import { coverMark, coverText, sideTags } from '../../../../core/combat/cover';
import { article } from '../../../../core/combat/combat-log';
import { combatantInitial, isPlayer, stateWord } from '../../../../core/combat/combat-view';
import {
  groupFeminine,
  groupName,
  isCreature,
  kindWord,
  ofOwner,
} from '../../../../core/combat/creature-names';
import {
  type OrderItem,
  jointTurn,
  listNames,
  orderItems,
} from '../../../../core/combat/joint-turn';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import type { CombatantInfo } from '../combat-info';
import { CombatantTags } from '../combatant-tags/combatant-tags';
import { BeastPool, FormTag } from '../combatant-tags/form-tag';
import { ConcPill, LoseQuestion } from './lose-question';
import { OrderLegend } from './order-legend';
import { RowCover } from './row-cover';
import { DeathRow } from '../death-saves/death-marks';
import { OrderGroup } from '../joint-turn/order-group';
import { PartState } from '../joint-turn/part-state';

/** The spell a combatant concentrates on, as the question says it. */
function concentrationOf(c: Combatant): string {
  return c.concentrationSpellNamePt || 'A magia';
}

/**
 * The master's order of initiative while the combat runs (E6-11, E6-12): who
 * goes when, with the hit points the master sees, the "Vez" row, the
 * reveal/hide switch of each NPC, and "Dano/Cura" (the E5-05 adjust dialog)
 * for a player's character. An NPC's own damage comes with the actions.
 * "Remover do combate" asks in place, with the focus on "Voltar". The ⋮ menu of
 * every row has "Condições…" (E6-29); under the name stand the condition tags
 * and the spell it concentrates on, and a character at 0 hit points shows its
 * word ("Caída", "Estável", "Morrendo · 3 falhas" on the danger surface, "✕
 * Morta") and the marks of its death saves (E6-30).
 */
@Component({
  selector: 'app-order-list',
  imports: [
    BeastPool,
    ConcPill,
    FormTag,
    LoseQuestion,
    CombatantTags,
    CombatantToken,
    OrderLegend,
    RowCover,
    DeathRow,
    MatButtonModule,
    MatIconModule,
    MatMenuModule,
    NgTemplateOutlet,
    OrderGroup,
    PartState,
  ],
  templateUrl: './order-list.html',
  styleUrl: './order-list.scss',
})
export class OrderList {
  readonly encounter = input.required<Encounter>();
  readonly info = input<ReadonlyMap<string, CombatantInfo>>(new Map());
  /** Whether "Dano/Cura" can open for this character (a living player). */
  readonly adjustable = input<ReadonlySet<string>>(new Set());
  readonly busy = input(false);

  readonly adjust = output<string>();
  /** "Dano/Cura" on an NPC: its combatant ID (`AdjustCombatantHitPoints`). */
  readonly adjustNpc = output<string>();
  readonly reveal = output<{ id: string; hidden: boolean }>();
  readonly remove = output<string>();
  /** "Perdeu a concentração": ends the spell the combatant holds (the creatures it kept go with it). */
  readonly endConcentration = output<string>();
  readonly add = output<void>();
  /** "Condições…": the combatant's ID. */
  readonly conditions = output<string>();
  /** The table hides the death saves from the other players (RN-24): the master's marks say who else sees them. */
  readonly deathsHidden = input(false);
  /** A combat without a map: the cover is marked in its own panel ("Cobertura dos alvos"), not row by row. */
  readonly theatre = input(false);
  /** The cover each combatant has against whoever has the turn (`GetTurnOptions`
   * of the master's subject), and who that is: "Três quartos (do mapa) contra o Pensantus". */
  readonly coverAgainst = input<ReadonlyMap<string, { cover: CoverDegree; source: CoverSource }>>(
    new Map(),
  );
  readonly turnLabel = input('');
  /** The side of whoever has the turn: the cover is said only against an opponent. */
  readonly turnSide = input<CombatantSide>(CombatantSide.UNSPECIFIED);
  /** The master's "Aliado" (PARTY) or back to enemy (ENEMY). */
  readonly side = output<{ id: string; side: CombatantSide }>();
  /** The master's manual cover mark. */
  readonly cover = output<{ id: string; cover: CoverDegree }>();
  /** In a combat without a map the menu's "Marcar cobertura…" opens the theatre cover editor (the fallback for a joint or master turn). */
  readonly coverEdit = output<string>();
  /** The characters whose "Confirmar a morte" question the master put away. */
  readonly deathDismissed = input<ReadonlySet<string>>(new Set());
  /** "Confirmar a morte": the question again. */
  readonly askDeath = output<string>();

  protected readonly Stable = CombatantState.STABLE;
  protected readonly removing = signal<string | null>(null);
  /** The combatant whose "Perdeu a concentração" question is open. */
  protected readonly losing = signal<string | null>(null);
  /** The combatant whose cover mark is open. */
  protected readonly marking = signal<string | null>(null);
  protected readonly Party = CombatantSide.PARTY;
  protected readonly Enemy = CombatantSide.ENEMY;
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  protected readonly rows = computed(() => this.encounter().combatants);
  /** The order with the boxes of the joint turns (the master has every total). */
  protected readonly items = computed(() => orderItems(this.encounter(), true));
  private readonly joint = computed(() => jointTurn(this.encounter()));
  /** Everything is read-only after the combat ends. */
  protected readonly live = computed(() => this.encounter().status !== EncounterStatus.ENDED);

  constructor() {
    // Opening the confirmation puts the focus on the safe button.
    effect(() => this.back()?.nativeElement.focus({ focusVisible: true } as FocusOptions));
  }

  protected initial(c: Combatant): string {
    return combatantInitial(c.label);
  }

  /** An NPC (the rounded square); a player's creature is its own shape. */
  protected npc(c: Combatant): boolean {
    return !isPlayer(c) && !isCreature(c);
  }

  protected creature(c: Combatant): boolean {
    return isCreature(c);
  }

  /** An NPC the master marked "Aliado": a side, said apart from the conditions. */
  protected ally(c: Combatant): boolean {
    return sideTags(c).length > 0;
  }

  protected tags(c: Combatant): string[] {
    // The master's list says the cover in the line under the name; "Aliado" is a side, shown apart.
    return conditionTags(c);
  }

  /** The spell it concentrates on, written out (the combat sends its name). */
  protected concentration(c: Combatant): string {
    return c.concentrationSpellNamePt;
  }

  /** A player's character at 0 hit points that is still in the story. */
  protected down(c: Combatant): boolean {
    return (
      c.state === CombatantState.DOWN ||
      c.state === CombatantState.DYING ||
      c.state === CombatantState.STABLE
    );
  }

  protected dying(c: Combatant): boolean {
    return c.state === CombatantState.DYING;
  }

  /** "Caída", "Estável", "Morrendo · 3 falhas". */
  protected downText(c: Combatant): string {
    return this.dying(c)
      ? joinDots(['Morrendo', `${c.deathFailures} falhas`])
      : stateWord(c.state, c.label);
  }

  protected player(c: Combatant): boolean {
    return isPlayer(c);
  }

  protected word(c: Combatant): string {
    return stateWord(c.defeated && isPlayer(c) ? CombatantState.DEAD : c.state, c.label);
  }

  /** The legend of the three shapes, once a player's creature is in the order. */
  protected readonly hasCreatures = computed(() => this.rows().some(isCreature));

  /** "Lobos atrozes da Sálvia" in the box of a group that is only creatures; nothing for any other group. */
  protected creatureNames(item: OrderItem): string {
    if (item.kind !== 'group' || !item.members.every(isCreature)) {
      return '';
    }
    const owner = ofOwner(this.encounter(), item.members[0]);
    return `${groupName(item.members)}${owner ? ` ${owner}` : ''}`;
  }

  protected sub(c: Combatant): string {
    const info = this.info().get(c.characterId);
    const ac = c.armorClass === undefined ? '' : `CA ${c.armorClass}`;
    if (isCreature(c)) {
      // "CA 14 · da Sálvia": its armor class and whose it is (the round dashed token and the legend say it is a creature); the
      // book's name only when the table gave it another ("Lobo atroz 1" needs none, "Nanquim" is a "Corvo").
      const kind = kindWord(c);
      return [kind === 'Criatura' ? '' : kind, ac, ofOwner(this.encounter(), c)]
        .filter(Boolean)
        .join(' · ');
    }
    const first = isPlayer(c) ? (info?.classSummary ?? '') : (info?.kindLabel ?? 'NPC');
    // A monster of the bestiary (RN-29): the master alone is told its challenge rating ("ND 1/8").
    const nd = c.challengeRating ? `ND\u00a0${c.challengeRating}` : '';
    return [first, nd, ac].filter(Boolean).join(' · ');
  }

  protected readonly article = article;

  /** "PV do Lobo 11 de 11" on a phone, where the hit points are a line under the name; empty when the combat sends no pool (the master and the druid's player only). */
  protected formPool(c: Combatant): string {
    if (!c.wildShapeBeastKey || c.wildShapeHitPointsMax === undefined) {
      return '';
    }
    return `PV d${article(c.wildShapeBeastNamePt) === 'a' ? 'a' : 'o'} ${c.wildShapeBeastNamePt} ${c.wildShapeHitPointsCurrent} de ${c.wildShapeHitPointsMax}`;
  }

  protected percent(c: Combatant): number {
    const max = c.hitPointsMax ?? 0;
    return max > 0 ? Math.min(100, Math.round(((c.hitPointsCurrent ?? 0) / max) * 100)) : 0;
  }

  /** The row of the one on turn; in a joint turn the box is on turn instead. */
  protected current(c: Combatant): boolean {
    return !this.joint() && c.id === this.encounter().currentCombatantId;
  }

  /** The combatant is a member of the joint turn that is running. */
  protected inTurn(c: Combatant): boolean {
    return !!this.joint()?.members.some((m) => m.id === c.id);
  }

  protected key(item: OrderItem): string {
    return item.kind === 'group' ? item.members.map((m) => m.id).join('+') : item.combatant.id;
  }

  /** "Turno conjunto: Brisa e Toren, iniciativa 19", for a screen reader. */
  protected groupLabel(item: OrderItem): string {
    return item.kind === 'group'
      ? `Turno conjunto: ${listNames(item.members.map((m) => m.label))}, iniciativa ${item.total}`
      : '';
  }

  /** "Três quartos (do mapa) contra o Pensantus", or `''` when there is none or nobody has the turn. */
  protected coverLine(c: Combatant): string {
    const against = this.coverAgainst().get(c.id);
    const text = against ? coverText(against.cover, against.source) : '';
    const who = this.turnLabel();
    const side = this.turnSide();
    // Only against opponents: a combatant of the same side as whoever has the turn is no target of theirs.
    return text && who && c.label !== who && (side === CombatantSide.UNSPECIFIED || c.side !== side)
      ? tieNumbers(`${text} contra ${article(who)} ${who}`)
      : '';
  }

  protected coverPictogram(c: Combatant): 'half' | 'three' | null {
    return coverMark(this.coverAgainst().get(c.id)?.cover ?? CoverDegree.NONE);
  }

  /** The text action shows where cover matters: a line, or a mark already there. */
  protected hasCover(c: Combatant): boolean {
    return (
      !!this.coverLine(c) ||
      (c.coverMark !== CoverDegree.NONE && c.coverMark !== CoverDegree.UNSPECIFIED)
    );
  }

  protected pickCover(id: string, cover: CoverDegree): void {
    this.cover.emit({ id, cover });
  }

  protected canAdjust(c: Combatant): boolean {
    return isPlayer(c) && this.adjustable().has(c.characterId);
  }

  /** An NPC's hit points are the master's to change, always (a defeated one
   * healed above 0 comes back). */
  protected canAdjustNpc(c: Combatant): boolean {
    return !isPlayer(c) && c.hitPointsMax !== undefined;
  }

  protected ask(id: string): void {
    this.removing.set(id);
  }

  /** The creatures a player's character keeps with its concentration: its own creatures of a casting (they carry the group the combat made), whatever the spell. */
  protected heldBy(c: Combatant): readonly Combatant[] {
    return isPlayer(c) && c.concentrationSpell
      ? this.rows().filter(
          (x) =>
            isCreature(x) &&
            x.ownerCharacterId === c.characterId &&
            !!x.summonGroupId &&
            !x.defeated,
        )
      : [];
  }

  /** "· 2 Lobos atrozes" after the spell, in the line under the name. */
  protected held(c: Combatant): string {
    const held = this.heldBy(c);
    return held.length > 0
      ? ` · ${held.length === 1 ? held[0].label : `${held.length} ${groupName(held)}`}`
      : '';
  }

  /** What losing it does, for the question: "Conjurar Animais acaba e os 2 Lobos atrozes somem do combate, da ordem e do mapa.". */
  protected heldText(c: Combatant): string {
    const held = this.heldBy(c);
    return held.length > 0
      ? `${concentrationOf(c)} acaba e ${held.length === 1 ? held[0].label : `${groupFeminine(held) ? 'as' : 'os'} ${held.length} ${groupName(held)}`} ${held.length === 1 ? 'some' : 'somem'} do combate, da ordem e do mapa.`
      : `${concentrationOf(c)} acaba.`;
  }

  /** "Dispensar os Lobos", "Dispensar o Nanquim": the button says what it sends away. */
  protected dismissLabel(c: Combatant): string {
    const held = this.heldBy(c);
    if (held.length === 1) {
      return `Dispensar ${article(held[0].label) === 'a' ? 'a' : 'o'} ${held[0].label}`;
    }
    const kinds = new Set(held.map((x) => x.monsterKey));
    return kinds.size === 1
      ? `Dispensar ${groupFeminine(held) ? 'as' : 'os'} ${groupName(held).split(' ')[0]}`
      : 'Dispensar as criaturas';
  }

  /** "A Sálvia perdeu a concentração?". */
  protected loseQuestion(c: Combatant): string {
    return `${article(c.label) === 'a' ? 'A' : 'O'} ${c.label} perdeu a concentração?`;
  }

  protected confirmLose(id: string): void {
    // The page is busy with another change: the question stays, so its answer is not lost in silence.
    if (this.busy()) {
      return;
    }
    this.losing.set(null);
    this.endConcentration.emit(id);
  }

  protected confirmRemove(id: string): void {
    if (this.busy()) {
      return;
    }
    this.removing.set(null);
    this.remove.emit(id);
  }
}
