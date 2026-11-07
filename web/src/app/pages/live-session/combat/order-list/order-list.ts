import { NgTemplateOutlet } from '@angular/common';
import { type LimitLine, limitLines } from '../../../../core/combat/creature-action';
import {
  Component,
  DestroyRef,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  model,
  output,
  signal,
  untracked,
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
import {
  SHIELD_ENDED_MS,
  aidBar,
  aidRowLabel,
  aidTagForMaster,
  endAidText,
  shieldEndedForMaster,
  shieldLabelForMaster,
} from '../../../../core/combat/combat-effects';
import { NudgeButton } from '../../nudge-button/nudge-button';
import { tieNumbers } from '../../../../core/format/text';
import { joinDots } from '../../../../core/format/text';
import { conditionTags } from '../../../../core/combat/conditions';
import { labelTags } from '../../../../core/effects/effects';
import { coverMark, coverText, sideTags } from '../../../../core/combat/cover';
import { article } from '../../../../core/combat/combat-log';
import { combatantInitial, isPlayer } from '../../../../core/combat/combat-view';
import {
  groupFeminine,
  groupName,
  isCreature,
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
import { EndAidQuestion } from './end-aid-question';
import { LoseQuestion } from './lose-question';
import { OrderLegend } from './order-legend';
import { RowCover } from './row-cover';
import { downText, isDown, isDying, rowDetail, rowFacts, rowState, stateWordOf } from './row-words';
import { rowsBelow } from './rows-below';
import { ShieldWatch } from './shield-watch';
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
    NudgeButton,
    BeastPool,
    EndAidQuestion,
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
  styleUrls: ['./order-list.scss', './order-list.parts.scss'],
})
export class OrderList {
  readonly encounter = input.required<Encounter>();
  /** The campaign, for "Chamar a atenção" on a player's row (the master's nudge). */
  readonly campaignId = input('');
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
  /** "Encerrar Ajuda em Sálvia": the combatant whose Ajuda the master ended. */
  readonly endAid = output<string>();
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
  /** The master's "Aparar: perguntar a cada golpe" switch of an NPC with a Parry: `ask` is the mode it is switched to. */
  readonly parryAsk = output<{ id: string; ask: boolean }>();
  /** The characters whose "Confirmar a morte" question the master put away. */
  readonly deathDismissed = input<ReadonlySet<string>>(new Set());
  /** "Confirmar a morte": the question again. */
  readonly askDeath = output<string>();
  /** The column is folded to a strip of tokens (C1-02); the page keeps it in memory, never on the device. */
  readonly folded = model(false);
  /** The fold button is offered: the three-column layout only. */
  readonly foldable = input(false);
  /** The head says "Ordem" (not where a tab above already does: the phone's "Ordem" tab). */
  readonly titled = input(true);

  protected readonly Stable = CombatantState.STABLE;

  /** The count chip of the heading shows from this many combatants (the board's eight players). */
  protected readonly crowded = 7;
  /** The rows that are open in place (a tap opens or closes one); a question that is open keeps its row open too. */
  private readonly openIds = signal<ReadonlySet<string>>(new Set());
  protected readonly below = signal(0);
  private readonly scroller = viewChild<ElementRef<HTMLElement>>('scroller');
  protected readonly removing = signal<string | null>(null);
  /** The combatant whose "Perdeu a concentração" question is open. */
  protected readonly losing = signal<string | null>(null);
  /** The combatant whose "Encerrar Ajuda" question is open. */
  protected readonly endingAid = signal<string | null>(null);
  /** "O Escudo Arcano de Pensantus acabou", said in the list for a few seconds after the stream took the bonus away. */
  protected readonly shieldEnded = signal('');
  private endedTimer: ReturnType<typeof setTimeout> | undefined;
  private readonly shields = new ShieldWatch();
  /** The combatant whose cover mark is open. */
  protected readonly marking = signal<string | null>(null);
  protected readonly Party = CombatantSide.PARTY;
  protected readonly Enemy = CombatantSide.ENEMY;
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  protected readonly rows = computed(() => this.encounter().combatants);
  /** The order with the boxes of the joint turns (the master has every total). */
  protected readonly items = computed(() => orderItems(this.encounter(), true));
  private readonly joint = computed(() => jointTurn(this.encounter()));
  /** Every combatant in the order of the list, the joint turns' members in their place. */
  protected readonly flat = computed(() =>
    this.items().flatMap((i) => (i.kind === 'group' ? [...i.members] : [i.combatant])),
  );
  /** Everything is read-only after the combat ends. */
  protected readonly live = computed(() => this.encounter().status !== EncounterStatus.ENDED);

  constructor() {
    inject(DestroyRef).onDestroy(() => clearTimeout(this.endedTimer));
    // A shield that goes from above 0 to 0 on a combatant that is still there ended (it ends by itself when the caster's turn begins).
    effect(() => {
      const e = this.encounter();
      untracked(() => {
        const ended = this.shields.update(e).map(shieldEndedForMaster);
        if (ended.length > 0) {
          this.shieldEnded.set(ended.join('. '));
          clearTimeout(this.endedTimer);
          this.endedTimer = setTimeout(() => this.shieldEnded.set(''), SHIELD_ENDED_MS);
        }
      });
    });
    // The "Mais N abaixo" cue follows the rows and the room: a list that changes or a window that resizes.
    effect(() => {
      this.items();
      this.folded();
      queueMicrotask(() => this.measure());
    });
    const observer =
      typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => this.measure());
    effect(() => {
      const el = this.scroller()?.nativeElement;
      observer?.disconnect();
      if (el) {
        observer?.observe(el);
      }
    });
    inject(DestroyRef).onDestroy(() => observer?.disconnect());
    // Opening the confirmation puts the focus on the safe button.
    effect(() => this.back()?.nativeElement.focus({ focusVisible: true } as FocusOptions));
  }

  /** A row is open when it was tapped, or while one of its questions is (so the answer is never hidden). */
  protected isOpen(c: Combatant): boolean {
    return (
      this.openIds().has(c.id) ||
      this.marking() === c.id ||
      this.losing() === c.id ||
      this.removing() === c.id ||
      this.endingAid() === c.id
    );
  }

  protected toggle(id: string): void {
    const next = new Set(this.openIds());
    if (!next.delete(id)) {
      next.add(id);
    }
    this.openIds.set(next);
  }

  protected fold(on: boolean): void {
    this.folded.set(on);
  }

  /** A token of the strip: the column opens with that row open. */
  protected openFromStrip(id: string): void {
    this.openIds.set(new Set([id]));
    this.folded.set(false);
  }

  /** The strip's token has no words of its own: "Goblin 1, 7 de 7 PV, é a vez". */
  protected stripLabel(c: Combatant): string {
    return this.headName(c);
  }

  /** The name of a row's head for a screen reader: who, whether it is their turn, their state and hit points. */
  protected headName(c: Combatant): string {
    const hp = c.hitPointsMax === undefined ? '' : `${c.hitPointsCurrent} de ${c.hitPointsMax} PV`;
    return [c.label, this.current(c) ? 'é a vez' : '', rowState(c), hp].filter(Boolean).join(', ');
  }

  /** The line under the name of a compact row: "Inic. 14 · CA 15", or the state word first ("Escondido · inic. 7"). */
  protected compactSub(c: Combatant, inGroup: boolean): string {
    const init = c.initiative !== undefined && !inGroup ? c.initiative : null;
    const state = rowState(c);
    if (state) {
      return joinDots([state, init === null ? '' : `inic. ${init}`].filter(Boolean));
    }
    return joinDots(
      [
        init === null ? '' : `Inic. ${init}`,
        rowDetail(c, this.encounter(), this.info().get(c.characterId), this.shield(c) > 0),
      ].filter(Boolean),
    );
  }

  /** The limits of a creature's actions, one line each in the opened row (the server sends them to the master and the controller only). */
  protected limits(c: Combatant): readonly LimitLine[] | null {
    return c.limitedActions.length ? limitLines(c.limitedActions) : null;
  }

  protected facts(c: Combatant): string {
    return rowFacts(c, this.shield(c), this.aid(c) > 0 ? this.aidTag(this.aid(c)) : '');
  }

  /** How many rows lie wholly below the room of the list: "Mais 6 abaixo". */
  protected measure(): void {
    this.below.set(rowsBelow(this.scroller()?.nativeElement));
  }

  protected scrollDown(): void {
    const el = this.scroller()?.nativeElement;
    el?.scrollTo({ top: el.scrollTop + el.clientHeight, behavior: 'smooth' });
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
    return labelTags(conditionTags(c), c.effectLabels);
  }

  /** The spell it concentrates on, written out (the combat sends its name). */
  protected concentration(c: Combatant): string {
    return c.concentrationSpellNamePt;
  }

  protected readonly down = isDown;
  protected readonly dying = isDying;
  protected readonly downText = downText;
  protected readonly player = isPlayer;
  protected readonly word = stateWordOf;

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

  /** The Escudo Arcano's bonus on the combatant: the row says "CA 13 + 5" and the label under the name. */
  protected shield(c: Combatant): number {
    return c.armorClass === undefined ? 0 : Math.max(0, c.armorClassBonus);
  }

  protected readonly shieldLabel = shieldLabelForMaster;

  /** Ajuda's bonus on the combatant's maximum (0 without it). */
  protected aid(c: Combatant): number {
    return Math.max(0, c.hitPointsMax === undefined ? 0 : c.hitPointsMaxBonus);
  }

  protected readonly aidTag = aidTagForMaster;
  protected readonly aidRowLabel = aidRowLabel;

  /** The bar's solid and striped pieces for a combatant under Ajuda. */
  protected aidBar(c: Combatant): { solid: number; striped: number } {
    return aidBar(c.hitPointsCurrent ?? 0, c.hitPointsMax ?? 0, this.aid(c));
  }

  /** "Encerrar a Ajuda de Sálvia?". */
  protected endAidQuestion(c: Combatant): string {
    return `Encerrar a Ajuda ${article(c.label) === 'a' ? 'da' : 'do'} ${c.label}?`;
  }

  protected endAidText(c: Combatant): string {
    return endAidText(c.hitPointsCurrent ?? 0, c.hitPointsMax ?? 0, this.aid(c));
  }

  protected confirmEndAid(id: string): void {
    if (this.busy()) {
      return;
    }
    this.endingAid.set(null);
    this.endAid.emit(id);
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
