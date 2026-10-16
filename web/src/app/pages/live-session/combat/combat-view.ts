import { NgTemplateOutlet } from '@angular/common';
import {
  Component,
  DestroyRef,
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
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatEffect,
  type InspirationOffer,
  type LuckyOffer,
  CombatLogKind,
  CombatantState,
  DeathSaveOutcome,
  CombatantSide,
  CoverDegree,
  type Encounter,
  EncounterStatus,
  type OpportunityOffer,
  type PendingDamage,
  PendingDamageStatus,
  type ReactionWindow,
  type GetTurnOptionsResponse,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { type AttackDie, CombatClient, newKey } from '../../../core/combat/combat-client';
import { CombatUndone } from '../../../core/combat/combat-undone';
import { ActionKey } from '../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import {
  type FormEnded,
  ConcentrationWatch,
  FormNotices,
  type LostConcentration,
  formNoticeText,
  lostNoticeText,
  wildActionLine,
} from '../../../core/combat/combat-notices';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import {
  coverTargets,
  everyoneStanding,
  isTheatre,
  reactorRows,
} from '../../../core/combat/theatre';
import { CreatureOptionsState } from '../../../core/combat/creature-options-state';
import {
  CHARACTER_TAB,
  type MineTab,
  actingTab,
  membersToEnd,
  mineTabs,
  ownCreatures,
  validTab,
} from '../../../core/combat/mine';
import {
  type ReactorAttack,
  offersHolding,
  offersToAnswer,
  reactorAttacks,
  reactorIsMasters,
  waitingText,
} from '../../../core/combat/opportunity';
import type { FogView } from '../../../core/maps/fog-view';
import { LayersState } from '../../../core/maps/layers-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { type SearchSkills, searchRoute } from '../../../core/traps/trap-search';
import type { TrapBoard } from '../../../core/traps/trap-board';
import type { OfferMark } from '../../../shared/combat-map/combat-map';
import type { Square } from '../../../core/combat/combat-grid';
import { openDamages, pendingNote } from '../../../core/combat/attack-flow';
import { CombatLogState } from '../../../core/combat/combat-log-state';
import type { CombatState } from '../../../core/combat/combat-state';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { TurnOptionsState } from '../../../core/combat/turn-options-state';
import { outcomeText, saveAnnouncement } from '../../../core/combat/death-saves';
import { RollAnimator, showOfDice, showOfDie } from '../../../shared/roll-overlay/roll-animator';
import { currentCombatant, isDown, isPlayer, ownCombatant } from '../../../core/combat/combat-view';
import { acts, jointTurn, masterActor } from '../../../core/combat/joint-turn';
import type { MapState } from '../../../core/maps/map-state';
import { RosterClient } from '../../../core/maps/roster-client';
import type { TokenDrop } from '../../../shared/combat-map/combat-map';
import { TurnAlert } from '../../../core/combat/turn-alert';
import { CheckRow } from '../../../shared/form-fields/check-row';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import type { PartyMemberInfoVm, VitalsVm } from '../../../core/play/live-session.types';
import { MasterAutoPass, masterWaitNote } from '../../../core/combat/auto-pass';
import { CombatCalls } from '../../../core/combat/combat-calls';
import {
  coverAgainst,
  creatureTrapNotes,
  deathDueOf,
  featureNote,
  initiativeLine,
  dyingOf,
  offerMarks,
  opportunityAttacks,
  pendingRollOf,
  stoppedByReaction,
  trapNote,
} from '../../../core/combat/combat-derived';
import { CombatReads } from './combat-reads';
import { CombatSheets } from './combat-sheets';
import { MoveFlow } from './move-flow';
import { ActionGroups } from './action-groups/action-groups';
import { ContestHost } from './contest-host';
import { ContestClient } from '../../../core/combat/contest-client';
import { DeathQuestion } from './death-question/death-question';
import { DeathSaves } from './death-saves/death-saves';
import { DeathSaveDue, type DeathSaveRoll } from './death-save-due/death-save-due';
import { EffectsPanel } from '../effects/effects-panel';
import { HitRiderCard, type RiderPick } from './hit-rider/hit-rider-card';
import { RageQuestion } from './rage-end/rage-question';
import { InspirationCard } from './inspiration-card/inspiration-card';
import { resourceFlow } from '../../../core/resources/resource-actions';
import { openWindows, reactionWait, sheetWindow } from '../../../core/combat/reactions';
import { ContestMaster } from './contest-master/contest-master';
import { ReactionQueue } from './reaction-queue/reaction-queue';
import { SurpriseCard } from './surprise-card/surprise-card';
import { CombatLogPanel } from './combat-log/combat-log-panel';
import type { CombatantInfo } from './combat-info';
import { CombatBar } from './combat-bar/combat-bar';
import { CombatMapCard } from './combat-map-card/combat-map-card';
import { CombatSummary } from './combat-summary/combat-summary';
import { JointCard } from './joint-turn/joint-card';
import { InitiativeSetup } from './initiative-setup/initiative-setup';
import { InitiativeSide } from './initiative-side/initiative-side';
import { type JumpRequest, MovePage } from './move-page/move-page';
import { OpportunityCard, type MasterAnswer } from './opportunity/opportunity-card';
import { HiddenRevealCard, type RevealAnswer } from './hidden-reveal/hidden-reveal-card';
import { heldWait, questions, revealWhy } from '../../../core/combat/hidden-reveal';
import { NpcCard } from './npc-card/npc-card';
import { DeathsBlock } from './deaths-block/deaths-block';
import { RollModeQueue } from './roll-mode/roll-mode-queue';
import { OrderList } from './order-list/order-list';
import { PlayerInitiative } from './player-initiative/player-initiative';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { BeastTraits } from '../../../shared/wild-shape/beast-traits';
import { CreatureBar } from './creature-turn/creature-bar';
import { CreatureBlock } from './creature-turn/creature-block';
import { CreatureHero } from './creature-turn/creature-hero';
import { MineTabs } from './mine-tabs/mine-tabs';
import type { DoorSquare } from '../../../core/maps/layers';
import {
  type StartCombatData,
  StartCombatDialog,
} from '../../../shared/start-combat/start-combat-dialog';
import { TrapDamages } from '../traps/trap-damages/trap-damages';
import { OrderColumn } from './turn-panel/order-column';
import { OrderStrip } from './turn-panel/order-strip';
import { TurnBar } from './turn-panel/turn-bar';
import { TurnPanel } from './turn-panel/turn-panel';
import { CoverPanel } from './theatre/cover-panel';
import { NoMapPanel, TheatreReaction } from './theatre/no-map-panel';
import { OfferPanel } from './theatre/offer-panel';

/**
 * The combat on the session page (MR-013, E6-01 to E6-16): it picks the
 * screen by who is looking and where the combat is, and runs every call.
 *
 * - **Master:** SETUP is the initiative list with its side panel (E6-04);
 *   ACTIVE is the combat bar, the map and the order (E6-11, E6-12); ENDED is
 *   the summary (E6-16).
 * - **Player:** SETUP is "Role a iniciativa" (E6-03); ACTIVE is the turn
 *   banner with the order strip, the map and, projected in, the vitals
 *   (E6-05), or the "Mover" page (E6-10); ENDED is the summary.
 *
 * It never reads the combat by itself: the page does, after
 * `encounter_changed`. A call's answer is the combat as it is now, so it is
 * applied at once; a refusal that means "the screen is stale" reads it again.
 */
/** The d20 the initiative is rolled with. */
const INITIATIVE_DIE = 20;

/** The questions of a held attack roll the page reopens the attack sheet at: the halfling's Lucky about a 1 (asked
 * first) and the Bardic Inspiration die. */
interface HeldRoll {
  readonly offer?: InspirationOffer;
  readonly lucky?: LuckyOffer;
}

@Component({
  selector: 'app-combat-view',
  imports: [
    CheckRow,
    ActionGroups,
    BeastTraits,
    CreatureBar,
    CreatureBlock,
    CreatureHero,
    DeathQuestion,
    DeathSaves,
    DeathSaveDue,
    EffectsPanel,
    RageQuestion,
    HitRiderCard,
    InspirationCard,
    CombatBar,
    HiddenRevealCard,
    CombatLogPanel,
    CombatMapCard,
    CombatSummary,
    CoverPanel,
    InitiativeSetup,
    InitiativeSide,
    JointCard,
    MatButtonModule,
    MatIconModule,
    MineTabs,
    MovePage,
    NgTemplateOutlet,
    NoMapPanel,
    NpcCard,
    OfferPanel,
    OpportunityCard,
    OrderColumn,
    DeathsBlock,
    OrderStrip,
    OrderList,
    PlayerInitiative,
    RollModeQueue,
    TheatreReaction,
    ReactionQueue,
    ContestMaster,
    SurpriseCard,
    TurnBar,
    TrapDamages,
    TurnPanel,
  ],
  templateUrl: './combat-view.html',
  styleUrl: './combat-view.scss',
})
export class CombatView {
  private readonly api = inject(CombatClient);
  private readonly animator = inject(RollAnimator);
  private readonly undone = inject(CombatUndone);
  private readonly roster = inject(RosterClient);
  private readonly maps = inject(MapsClient);
  private readonly catalog = inject(SpellCatalog);
  private readonly dialog = inject(MatDialog);
  private readonly creaturesApi = inject(CreaturesClient);
  private readonly tableRules = inject(TableRulesClient);
  private readonly bottomSheet = inject(MatBottomSheet);
  protected readonly turnAlert = inject(TurnAlert);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  protected readonly phone = mediaQuery(PHONE_QUERY);
  /** From 1024px the master has the combat bar (E6-11); below it the turn card does it all (E6-12). */
  protected readonly laptop = mediaQuery('(min-width: 1024px)');
  /** From 1280px the player's order is a column on the left (E6-14). */
  protected readonly wideLayout = mediaQuery('(min-width: 1280px)');
  readonly campaignId = input.required<string>();
  readonly isMaster = input(false);
  readonly state = input.required<CombatState>();
  readonly mapState = input.required<MapState>();
  /** What the player sees of the map with the fog on (MR-036): the combat map draws it, and its layers are the ones filtered to it. */
  readonly fog = input<FogView | null>(null);
  readonly vitals = input<readonly VitalsVm[]>([]);
  readonly partyInfo = input<ReadonlyMap<string, PartyMemberInfoVm>>(new Map());
  readonly sessionNumber = input(0);
  /** The players' "O combate acabou" card is above: the summary does not repeat its heading on screen. */
  readonly cardAbove = input(false);
  /** The player's own armor class from their sheet (without Escudo's +5), for the Escudo result. */
  readonly armorClass = input<number | null>(null);
  /** "Clérigo 5": the player's class line, for the Revivificar sheet's header. */
  readonly casterClasses = input('');
  readonly diceMode = input.required<DiceMode>();
  readonly dicePreference = input.required<DicePreference>();
  /** The player's bonuses in Percepção and Investigação, for "Procurar" (E9-08); `null` while unknown. */
  readonly trapSkills = input<SearchSkills | null>(null);
  /** The traps' board: the master's damage that waits for him (a trap that fired in the combat), read here too. */
  readonly traps = input<TrapBoard | null>(null);

  /** "Dano/Cura" on a player's row: the page opens its adjust dialog. */
  readonly adjust = output<VitalsVm>();
  /** "Mudar" (how the player rolls). */
  readonly changeDice = output<void>();

  /** The character's own vitals row: the hit points that wait while it is a beast, and the uses of its resources. */
  protected readonly ownVitals = computed(() => {
    const own = this.own();
    return own ? (this.vitals().find((v) => v.characterId === own.characterId) ?? null) : null;
  });

  /** The calls that answer with the combat: one at a time, the answer applied, a refusal said (`CombatCalls`). */
  private readonly calls = new CombatCalls({
    api: this.api,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    state: () => this.state(),
  });
  protected readonly busy = this.calls.busy;
  protected readonly error = this.calls.error;

  /** What the screen reads from the services around the combat (`CombatReads`). */
  private readonly reads = new CombatReads({
    campaignId: () => this.campaignId(),
    isMaster: () => this.isMaster(),
    encounter: () => this.encounter(),
    own: () => this.own(),
    options: () => this.options(),
    tableRules: this.tableRules,
    catalog: this.catalog,
    roster: this.roster,
    creatures: this.creaturesApi,
  });
  protected readonly rosterInfo = this.reads.rosterInfo;
  protected readonly spellDetails = this.reads.spellDetails;
  protected readonly revealSpellNames = this.reads.revealSpellNames;
  protected readonly beastBlock = this.reads.beastBlock;
  protected readonly familiar = this.reads.familiar;
  protected readonly deathsHidden = this.reads.deathsHidden;

  /** Every sheet and dialog the screen opens (`CombatSheets`). */
  private readonly sheets = new CombatSheets({
    dialog: this.dialog,
    bottomSheet: this.bottomSheet,
    api: this.api,
    creatures: this.creaturesApi,
    catalog: this.catalog,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    own: () => this.own(),
    isMaster: () => this.isMaster(),
    state: () => this.state(),
    diceMode: () => this.diceMode(),
    preference: () => this.dicePreference(),
    options: () => this.options(),
    vitals: () => this.vitals(),
    ownVitals: this.ownVitals,
    casterClasses: () => this.casterClasses(),
    armorClass: () => this.armorClass(),
    trapSkills: () => this.trapSkills(),
    mapState: () => this.mapState(),
    image: () => this.image(),
    theatre: () => this.theatre(),
    mapName: () => this.mapName(),
    layers: () => this.layers(),
    fogVision: () => this.fogVision(),
    spendClosed: () => this.moves.moveNote.set(''),
  });
  private readonly attackSheetOpen = this.sheets.attackOpen;
  private readonly reactionOpen = this.sheets.reactionOpen;
  private readonly reactionHandled = this.sheets.reactionHandled;
  private readonly inspirationHandled = this.sheets.inspirationHandled;

  /** Moving: the "Mover" page, the jump, the master's drag and the reach (`MoveFlow`). */
  protected readonly moves = new MoveFlow({
    api: this.api,
    calls: this.calls,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    state: () => this.state(),
    own: () => this.own(),
    isMaster: () => this.isMaster(),
    active: () => this.active(),
    theatre: () => this.theatre(),
    options: () => this.options(),
    creatureOptions: () => this.creatureOptions.data(),
    takeAction: (id, key) => this.takeActionFor(id, key),
    openSpend: (id) => this.sheets.spend(id),
  });
  protected readonly moverId = this.moves.moverId;
  protected readonly dropStart = this.moves.dropStart;
  protected readonly moveError = this.moves.moveError;
  protected readonly moveNote = this.moves.moveNote;
  protected readonly lockedDoor = this.moves.lockedDoor;
  protected readonly forcedMove = this.moves.forcedMove;
  protected readonly forcedNote = this.moves.forcedNote;
  protected readonly moveJump = this.moves.jump;
  protected readonly reachOn = this.moves.reachOn;
  protected readonly moveOptions = this.moves.options;
  protected readonly canDisengage = this.moves.canDisengage;
  protected readonly moverJumps = this.moves.jumps;
  protected readonly masterReach = this.moves.masterReach;
  protected readonly reachSwitch = this.moves.reachSwitch;

  /** The master's screen passes by itself the reactions a player leaves unanswered (`MasterAutoPass`). */
  private readonly autoPass = new MasterAutoPass({
    api: this.api,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    isMaster: () => this.isMaster(),
    state: () => this.state(),
  });
  protected readonly autoLeft = this.autoPass.windowsLeft;
  protected readonly offerLeft = this.autoPass.offersLeft;

  protected readonly Status = EncounterStatus;
  /** What the combatant in the spotlight can do: the player's own on their
   * turn, the master's current one (`GetTurnOptions`). */
  protected readonly turn = new TurnOptionsState();
  protected readonly log = new CombatLogState();
  /** What the last feature said ("Surto de Ação: você tem outra ação"), and the
   * death save just rolled this turn; both are cleared when the turn changes. */
  protected readonly actionNote = signal('');
  protected readonly deathResult = signal('');
  /** The turn whose damage was just settled: the master's card stays for its note. */
  private readonly settledTurn = signal('');
  /** The characters at three failures the master put away with "Ainda não". */
  protected readonly deathLater = signal<ReadonlySet<string>>(new Set());
  private readonly deathKey = new ActionKey();
  /** The master tapped "Próximo turno" while a death save was due: the question waits for "Rolar agora" or "Passar sem rolar". */
  protected readonly passAsking = signal<{ readonly discard: boolean } | null>(null);
  /** The sentence of the death save the master rolled for an absent player, kept after the save is no longer due. */
  protected readonly masterDeathResult = signal('');
  private readonly confirmDeathKey = new ActionKey();
  private readonly leaveFormKey = new ActionKey();
  protected readonly encounter = computed(() => this.state().shown());
  /** "Combate atualizado agora." (PM-02c 9c): said once when the page comes up with the turn on hold, that is, after a reload;
   * it goes when the wait is over. Nothing here is kept in the browser: the combat says what waits. */
  protected readonly refreshed = signal('');
  private refreshChecked = false;
  protected readonly image = computed(() => {
    const e = this.encounter();
    const map = this.mapState().map();
    return e && map && map.id === e.mapId && map.image
      ? { url: map.image.url, width: map.image.width, height: map.image.height }
      : null;
  });
  protected readonly mapName = computed(() => this.mapState().map()?.name ?? '');
  /** The combat is played without a map (RN-25): a list instead of a map, movement by number, the master's offers (E10-04). */
  protected readonly theatre = computed(() => isTheatre(this.encounter()));
  /** The master's "Oferecer ataque de oportunidade" form is open. */
  protected readonly offering = signal(false);
  protected readonly offerError = signal('');
  /** Who the mover can have left the reach of (the other side, standing): the form's rows. */
  protected readonly offerRows = computed(() => {
    const e = this.encounter();
    return e
      ? reactorRows(
          e,
          (c) =>
            this.info().get(c.characterId)?.classSummary ??
            this.info().get(c.characterId)?.kindLabel ??
            '',
        )
      : [];
  });
  /** Why the offer cannot be made now (nobody on the other side can react), or `''`. */
  protected readonly offerWhy = computed(() => {
    const rows = this.offerRows();
    return rows.every((r) => r.spent || r.offered) ? 'Ninguém pode reagir agora.' : '';
  });
  /** One idempotency key per form and reactor: a repeated tap never offers twice, a new form is a new offer. */
  private offerKeys = new Map<string, string>();
  /** The cover editor asked for from the order's menu (the fallback for a joint or master turn): the panel opens on that combatant. */
  protected readonly coverRequest = signal<{ id: string; n: number } | null>(null);
  protected readonly everyone = computed(() => {
    const e = this.encounter();
    return e ? everyoneStanding(e) : [];
  });
  /** The log says the table hides the death saves, to the other players and only when someone is down. */
  protected readonly deathNote = computed(() => {
    const e = this.encounter();
    return (
      this.deathsHidden() &&
      !this.isMaster() &&
      !!e &&
      e.combatants.some((c) => isDown(c) && !c.mine)
    );
  });
  /** Whose cover the master marks (the other side of whoever is on turn). */
  protected readonly coverTargets = computed(() => {
    const e = this.encounter();
    return e ? coverTargets(e) : [];
  });
  protected readonly info = computed<ReadonlyMap<string, CombatantInfo>>(() => {
    const merged = new Map(this.rosterInfo());
    // A player has no roster: their own combatant says what the table needs.
    for (const [id, p] of this.partyInfo()) {
      if (!merged.has(id)) {
        merged.set(id, {
          classSummary: p.classSummary,
          playerName: p.playerName,
          kindLabel: '',
          raceName: '',
        });
      }
    }
    return merged;
  });
  /** The characters whose "Dano/Cura" has a vitals row to adjust. */
  protected readonly adjustable = computed(() => new Set(this.vitals().map((v) => v.characterId)));
  // ---- Forma Selvagem (MR-037, E9-11) ----

  /** The druid's Wild Shape as a line of the Ação: its uses ("restam 2 de 2 usos") and why it cannot be used now. `null` without it, and while in a form. */
  protected readonly wild = computed(() => {
    const own = this.own();
    const feature = this.options()?.options?.featureActions.find((a) =>
      a.action?.key.startsWith('feature:wild-shape'),
    );
    const resource = feature?.action
      ? this.ownVitals()?.resources?.find((r) => r.key === feature.action?.resourceKey)
      : undefined;
    return own ? wildActionLine(feature, !!own.wildShapeBeastKey, resource) : null;
  });
  /** What the form's end said, from the combat log's line (the server's number and reason): until it is touched. */
  protected readonly formEnded = signal<FormEnded | null>(null);
  protected readonly formNotice = computed(() => {
    const ended = this.formEnded();
    return ended ? formNoticeText(ended) : '';
  });
  private readonly formNotices = new FormNotices();
  /** "Voltar à forma normal": a bonus action. The druid knows it came back: the log's line says "left", and no notice comes of it. */
  protected async leaveForm(): Promise<void> {
    const own = this.own();
    if (!own || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.creaturesApi.leaveWildShape(
        this.campaignId(),
        own.characterId,
        this.leaveFormKey.keyFor(own.characterId),
      );
      this.leaveFormKey.renew();
      if (res.encounter) {
        this.state().apply(res.encounter);
      }
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'voltar à forma normal'));
      await this.calls.refreshAfter(err);
    } finally {
      this.busy.set(false);
    }
  }

  // ---- losing the concentration that holds the creatures (MR-037, E9-12 state 5) ----

  /** What the player's own concentration ending took with it: the spell and the creatures that left, until it is touched. */
  protected readonly lostConcentration = signal<LostConcentration | null>(null);
  protected readonly lostNotice = computed(() => {
    const lost = this.lostConcentration();
    return lost ? lostNoticeText(lost) : '';
  });
  private readonly concentration = new ConcentrationWatch();
  protected readonly lookingThroughFamiliar = computed(() => !!this.own()?.familiarSightCreatureId);
  protected readonly own = computed(() => {
    const e = this.encounter();
    return e ? ownCombatant(e) : null;
  });
  protected readonly myTurn = computed(() => {
    const e = this.encounter();
    const own = this.own();
    return !!e && !!own && acts(e, own);
  });
  /** On a phone, on the player's own turn, the order strip goes under the actions instead of between the title and them. */
  protected readonly orderBelow = computed(
    () => !this.laptop() && this.myTurn() && !this.ownDown() && !this.creatureTab(),
  );
  // ---- what the player plays: the character and its creatures (MR-037, E9-12) ----

  /** The tabs of the turn bar: the character, then each group of creatures (a casting's, in a joint turn, or one alone). */
  protected readonly tabs = computed<readonly MineTab[]>(() => {
    const e = this.encounter();
    return !this.isMaster() && e && e.status === EncounterStatus.ACTIVE ? mineTabs(e) : [];
  });
  /** The tabs show only when the player plays more than one combatant. */
  protected readonly showTabs = computed(() => this.tabs().length > 1);
  /** The tab the page shows; it follows the turn to where the player acts, and a tap changes it. */
  protected readonly tabId = signal(CHARACTER_TAB);
  protected readonly tab = computed<MineTab | null>(
    () => this.tabs().find((t) => t.id === this.tabId()) ?? this.tabs()[0] ?? null,
  );
  /** The page of a creature's turn is open (not the character's). */
  protected readonly creatureTab = computed(() => {
    const t = this.tab();
    return t?.creature ? t : null;
  });
  /** `GetTurnOptions` of each creature the player plays: the attacks with their numbers, the standard actions. */
  protected readonly creatureOptions = new CreatureOptionsState();
  /** The question to end a group's part is open: the tabs are inert until it is answered. */
  protected readonly partAsking = signal(false);
  /** What it waits for ("Esperando a reação do mestre") on the page of a creature's turn. */
  protected readonly creatureLocked = computed(() => this.waiting()?.title ?? '');

  /** The joint turn that is running (a group of two or more), or `null`. */
  protected readonly joint = computed(() => {
    const e = this.encounter();
    return e ? jointTurn(e) : null;
  });
  /** In a joint turn, who else must end their part, for the player's footer. */
  protected readonly jointOthers = computed(() => {
    const joint = this.joint();
    if (!joint || !this.myTurn()) {
      return null;
    }
    return [
      ...joint.acting.filter((m) => !m.mine).map((m) => m.label),
      ...(joint.waitsForMaster ? ['o mestre'] : []),
    ];
  });
  /** The player's own slots, for the rows above the spell list. */
  protected readonly ownSlots = computed(() => {
    const own = this.own();
    const v = own ? this.vitals().find((x) => x.characterId === own.characterId) : undefined;
    return { usage: v?.spellSlots ?? [], pact: v?.pactSlots ?? null };
  });
  /** The map's painted layers (walls, difficult terrain, cover), read again when the map's `layers_revision` changes. */
  private readonly layersState = new LayersState(
    async (mapId) => this.maps.layers(this.campaignId(), mapId),
    () => !this.isMaster(),
  );
  /** On a fog map the player's layers come with the vision (filtered to what they see); otherwise they are the map's. */
  protected readonly fogVision = computed(() =>
    this.isMaster() ? null : (this.fog()?.vision() ?? null),
  );
  protected readonly layers = computed(() =>
    this.fogVision()
      ? (this.fog()?.layers() ?? this.layersState.layers())
      : this.layersState.layers(),
  );
  // ---- opportunity attacks (E9-13) ----

  /** The offers the caller answers, and the ones on the player's own mover. */
  protected readonly toAnswer = computed(() => {
    const e = this.encounter();
    return e ? offersToAnswer(e) : [];
  });
  /** What the waiting mover reads: their move landed, and each offer waits for its answer. Any other wait for the master
   * (`turn_held`: a question about hidden creatures) is only "Esperando o mestre", on the player's turn and on another's,
   * never why (RN-10). */
  protected readonly waiting = computed(() => {
    const e = this.encounter();
    const own = this.own();
    if (!e || !own || this.isMaster()) {
      return null;
    }
    // What the server says the combat waits for (a reaction, a concentration save), written for this player.
    return (
      reactionWait(e) ?? waitingText(e, offersHolding(e, own.id)) ?? heldWait(e, this.myTurn())
    );
  });
  /** Why "Próximo turno" waits while questions about hidden creatures are open ("Responda ao pedido abaixo para seguir."). */
  protected readonly waitWhy = computed(() => {
    const e = this.encounter();
    return e && this.isMaster() ? revealWhy(e) : '';
  });
  /** The NPC reactors' attacks with their numbers, by offer (read from the reactor's own options). */
  protected readonly reactorOptions = signal<ReadonlyMap<string, GetTurnOptionsResponse>>(
    new Map(),
  );
  protected readonly masterAttacks = computed<ReadonlyMap<string, readonly ReactorAttack[]>>(
    () =>
      new Map(
        this.toAnswer().map((o) => [
          o.id,
          reactorAttacks(
            o,
            (this.reactorOptions().get(o.id)?.options?.attacks ?? []).flatMap((a) =>
              a.attack ? [a.attack] : [],
            ),
          ),
        ]),
      ),
  );
  private readonly offersHandled = new Set<string>();
  private readonly reactorReading = new Set<string>();
  /** The offers whose reactor's attacks could not be read: the master's card offers "Tentar de novo". */
  protected readonly reactorFailed = signal<ReadonlySet<string>>(new Set());
  /** The master's last action is a move: the log's undoable entry says so ("Desfazer o movimento"). */
  protected readonly canUndoMove = computed(
    () => this.log.undoable()?.kind === CombatLogKind.MOVED,
  );
  protected readonly turnSide = computed(() => this.subject()?.side ?? CombatantSide.UNSPECIFIED);
  protected readonly turnLabel = computed(() =>
    this.isMaster() ? (this.subject()?.label ?? '') : '',
  );
  /** The one whose options the screen asks for: the player's own character
   * on their turn, and for the master whoever is on turn (in a joint turn that holds a player, the NPC that still acts). */
  protected readonly subject = computed(() => {
    const e = this.encounter();
    if (!e || e.status !== EncounterStatus.ACTIVE) {
      return null;
    }
    // A player's options are asked off turn too: the opportunity attack
    // (`attack_targets` is filled, `as_reaction`) needs them.
    return this.isMaster() ? (masterActor(e) ?? currentCombatant(e)) : this.own();
  });
  protected readonly options = computed(() => this.turn.data());
  /** The player's side of the contests (W7-X): grapple, shove, escape, Hide, Help, and the questions a contest asks. */
  protected readonly contest = new ContestHost({
    dialog: this.dialog,
    bottomSheet: this.bottomSheet,
    api: inject(ContestClient),
    campaignId: () => this.campaignId(),
    isMaster: () => this.isMaster(),
    encounter: this.encounter,
    own: this.own,
    options: this.options,
    state: () => this.state(),
    diceMode: () => this.diceMode(),
    preference: () => this.dicePreference(),
  });
  /** The contest facts of the player's turn (hidden, surprised, grappled) and the special attacks, from `GetTurnOptions`. */
  protected readonly contestFacts = computed(() => this.options()?.contestState);
  protected readonly contestAttacks = computed(() => this.options()?.contestAttackOptions ?? []);
  protected readonly economy = computed(() => this.options()?.options?.economy);
  /** Extra Attack: the attacks of this Attack action that remain, and how many it makes. */
  protected readonly attacksLeft = computed(() => this.economy()?.attacksLeft ?? 0);
  protected readonly attacksPerAction = computed(() => this.economy()?.attacksPerAction ?? 1);
  /** The player's maximum hit points, for "com 0 de 24 pontos de vida". */
  protected readonly ownMaxHp = computed(
    () =>
      this.vitals().find((v) => v.characterId === this.own()?.characterId)?.hitPointsMax ?? null,
  );
  protected readonly ownDown = computed(() => {
    const own = this.own();
    return !!own && isDown(own);
  });
  /** The spell the player concentrates on, written out. */
  protected readonly ownConcentration = computed(() => {
    return this.own()?.concentrationSpellNamePt ?? '';
  });
  /** What the master's turn still owes a hit: "Falta aplicar 5 de dano". */
  protected readonly owed = computed(() =>
    this.isMaster() ? pendingNote(this.options()?.pendingDamages ?? []) : null,
  );
  /** The master's card shows for an NPC on turn, and for any turn that left
   * damage to apply. */
  protected readonly masterCard = computed(() => {
    const c = this.subject();
    return this.isMaster() && c ? c : null;
  });
  protected readonly cardVisible = computed(() => {
    const c = this.masterCard();
    return (
      !!c &&
      (!isPlayer(c) ||
        openDamages(this.options()?.pendingDamages ?? []).length > 0 ||
        this.settledTurn() === c.id)
    );
  });
  protected readonly unplaced = computed(() =>
    this.theatre() ? [] : (this.encounter()?.combatants ?? []).filter((c) => !c.placed),
  );
  protected readonly setup = computed(() => this.encounter()?.status === EncounterStatus.SETUP);
  protected readonly active = computed(() => this.encounter()?.status === EncounterStatus.ACTIVE);
  protected readonly ended = computed(() => this.encounter()?.status === EncounterStatus.ENDED);

  /** Changes only when the turn passes (or the combat ends): a string, so the effects that read it ignore the combat's other updates. */
  private readonly turnKey = computed(() => {
    const e = this.encounter();
    return e && e.status === EncounterStatus.ACTIVE
      ? `${e.id}:${e.round}:${e.currentCombatantId}`
      : '';
  });

  /** The master's bar: "Esperando a sua resposta: escondidas atingidas", or "Esperando a sua reação: Goblin 2". */
  protected readonly waitNote = computed(() => {
    const e = this.encounter();
    return e && this.isMaster()
      ? masterWaitNote(e, this.autoLeft(), (id) => this.info().get(id)?.playerName ?? '')
      : '';
  });
  /** The reactors drawn on the map for whoever answers, with the square the mover left. */
  protected readonly offerMarks = computed<readonly OfferMark[]>(() => {
    const e = this.encounter();
    return e ? offerMarks(e) : [];
  });
  /** Who has the cover each combatant has against whoever has the turn (the master's order list). */
  protected readonly coverAgainst = computed(() => coverAgainst(this.options()));
  /** The melee attacks an opportunity attack can use: off turn, with the reaction free, someone in reach and the character
   * on its feet (E6-28). */
  protected readonly opportunities = computed(() =>
    opportunityAttacks(
      this.own(),
      this.options(),
      !this.isMaster() && !this.myTurn() && this.active(),
    ),
  );
  /** The characters at three failures: the master is asked to confirm each death (E6-30). */
  protected readonly dying = computed(() => (this.isMaster() ? dyingOf(this.encounter()) : []));
  /** The downed player characters whose death save is due this turn: the master rolls it when the player is away (RN-03). */
  protected readonly deathDue = computed(() =>
    this.isMaster() ? deathDueOf(this.encounter()) : [],
  );
  /** The player's hit whose damage is still to roll (the sheet was closed). */
  protected readonly ownPendingRoll = computed(() => pendingRollOf(this.options(), this.own()?.id));
  /** The newest attack of the one on turn was stopped by the target's Escudo (from the log). */
  protected readonly reactionStopped = computed(() =>
    stoppedByReaction(this.log.rounds(), this.subject()?.id),
  );
  /** What a trap did to this player's character this round (E9-08 E), from the combat log. */
  protected readonly trapNote = computed(() =>
    trapNote(this.encounter(), this.own(), this.log.rounds()),
  );
  /** The same note for each of the player's creatures that a trap caught (a wolf or Nanquim that walked into one on its own part). */
  protected readonly creatureTrapNotes = computed(() =>
    creatureTrapNotes(this.encounter(), this.isMaster(), this.log.rounds()),
  );

  constructor() {
    effect(() => {
      const e = this.encounter();
      untracked(() => {
        const held =
          !!e && e.status === EncounterStatus.ACTIVE && (e.turnHeld || questions(e).length > 0);
        if (e && !this.refreshChecked) {
          this.refreshChecked = true;
          this.refreshed.set(held ? 'Combate atualizado agora.' : '');
        } else if (!held) {
          this.refreshed.set('');
        }
      });
    });
    // "Ainda não" holds while the character stays dying: when they leave DYING the question is
    // new the next time they fall.
    effect(() => {
      const dying = new Set(
        (this.encounter()?.combatants ?? [])
          .filter((c) => c.state === CombatantState.DYING)
          .map((c) => c.id),
      );
      untracked(() => {
        const later = this.deathLater();
        if ([...later].some((id) => !dying.has(id))) {
          this.deathLater.set(new Set([...later].filter((id) => dying.has(id))));
        }
      });
    });
    // The offer's form belongs to a turn: when it passes, or the combat ends, it closes.
    effect(() => {
      void this.turnKey();
      untracked(() => this.offering.set(false));
    });
    // The options are asked for again whenever the combat changes (every
    // call and every event bumps its revision) and whoever is in the
    // spotlight changes. A turn that ends leaves nothing to show.
    effect(() => {
      const e = this.encounter();
      const who = this.subject();
      const campaignId = this.campaignId();
      if (!e || !who) {
        untracked(() => this.turn.clear());
        return;
      }
      void e.revision;
      untracked(() => void this.turn.load(this.api, campaignId, e.id, who.id));
    });
    // The log is read again on every `combat_log_changed` and after every
    // reconnection (the page bumps `logTick`).
    effect(() => {
      const e = this.encounter();
      const campaignId = this.campaignId();
      this.state().logTick();
      if (!e || e.status === EncounterStatus.SETUP) {
        untracked(() => this.log.clear());
        return;
      }
      untracked(() => void this.log.load(this.api, campaignId, e.id));
    });
    // What a turn said is not carried into the next one.
    let turnOf = '';
    effect(() => {
      const id = this.encounter()?.currentCombatantId ?? '';
      if (id !== turnOf) {
        turnOf = id;
        untracked(() => {
          this.actionNote.set('');
          this.deathResult.set('');
          this.masterDeathResult.set('');
          this.passAsking.set(null);
          this.moveNote.set('');
        });
      }
    });
    // A reaction window the player answers (Escudo, Esquiva, Contramágica, the concentration save...) opens its sheet
    // by itself, once; a reload brings it back, as the window lives on the server.
    effect(() => {
      const e = this.encounter();
      const own = this.own();
      if (
        this.isMaster() ||
        !e ||
        !own ||
        e.status !== EncounterStatus.ACTIVE ||
        this.reactionOpen()
      ) {
        return;
      }
      const window = sheetWindow(e);
      if (window && !this.reactionHandled.has(window.id)) {
        this.reactionHandled.add(window.id);
        untracked(() => this.sheets.reaction(window));
      }
    });
    // A roll held for a question, Lucky or Bardic Inspiration (the player read the screen again, or left the sheet): the
    // question opens by itself, once.
    effect(() => {
      const e = this.encounter();
      const offer = this.ownLucky() ?? this.ownOffer();
      // The turn options arrive after the combat: the question opens when its attack is in them.
      const ready = this.options() !== null;
      // While an attack sheet is open it is the one that gets the answer of its own roll.
      if (
        this.isMaster() ||
        !e ||
        !offer ||
        !ready ||
        this.attackSheetOpen() ||
        e.status !== EncounterStatus.ACTIVE
      ) {
        return;
      }
      if (!this.inspirationHandled.has(offer.holdId)) {
        untracked(() => {
          if (this.openHeldRoll()) {
            this.inspirationHandled.add(offer.holdId);
          }
        });
      }
    });
    // The map's layers: read when the combat's map is known, and again when the map's `layers_revision` changes.
    effect(() => {
      const e = this.encounter();
      const map = this.mapState().map();
      const id = e && map && map.id === e.mapId ? map.id : null;
      const revision = map?.layersRevision ?? 0;
      untracked(() => void this.layersState.open(id, revision));
    });
    // An offer the player answers opens its prompt by itself, once (E9-13).
    effect(() => {
      const e = this.encounter();
      if (this.isMaster() || !e || e.status !== EncounterStatus.ACTIVE) {
        return;
      }
      for (const offer of offersToAnswer(e)) {
        if (!this.offersHandled.has(offer.id)) {
          this.offersHandled.add(offer.id);
          untracked(() => this.sheets.opportunity(offer));
        }
      }
    });
    // The turn alert: the turn passing to a group the player plays (vibration, chime, shake, title); the first look is only the baseline.
    effect(() => {
      const tab = this.tabs().find((t) => t.state === 'turn');
      const turn = tab ? { key: tab.id, label: tab.label } : null;
      untracked(() => this.turnAlert.observe(turn));
    });
    inject(DestroyRef).onDestroy(() => this.turnAlert.reset());
    // The tab follows the turn: when it comes to a group of the player's (the character or its creatures), the page shows it.
    let acting = '';
    effect(() => {
      const now = actingTab(this.tabs());
      if (now && now !== acting) {
        untracked(() => this.tabId.set(now));
      }
      acting = now;
    });
    // The tabs go away when the player is back to one combatant: the page is the character's again.
    effect(() => {
      const keep = validTab(this.tabs(), this.tabId());
      if (keep !== this.tabId()) {
        untracked(() => this.tabId.set(keep));
      }
    });
    // The creatures' own options (their attacks, their standard actions), asked again whenever the combat changes.
    effect(() => {
      const e = this.encounter();
      const campaignId = this.campaignId();
      const ids =
        e && !this.isMaster() && e.status === EncounterStatus.ACTIVE
          ? ownCreatures(e).map((c) => c.id)
          : [];
      if (!e || ids.length === 0) {
        untracked(() => this.creatureOptions.clear());
        return;
      }
      void e.revision;
      untracked(() => void this.creatureOptions.load(this.api, campaignId, e.id, ids));
    });
    // When the form ends without the druid asking (the beast fell to 0, the master ended it), the player is told, until they touch it:
    // the combat log's line says why and how much damage passed on (the server's number; nothing is worked out here). The lines that
    // were there when the combat was read are not news, so a reconnect or a re-read never repeats a notice.
    effect(() => {
      const id = this.encounter()?.id ?? '';
      const own = this.own();
      const rounds = this.log.rounds();
      const loaded = this.log.loaded();
      untracked(() => {
        const { reset, notice } = this.formNotices.read(
          id,
          own?.id ?? '',
          loaded,
          rounds.flatMap((r) => r.entries),
        );
        if (reset) {
          this.formEnded.set(null);
          this.lostConcentration.set(null);
        }
        if (notice) {
          this.formEnded.set(notice);
        }
      });
    });
    // When the concentration that held creatures ends (a lost save, another spell, the master), the player is told too. It reads the
    // combat's own data, for this combat only: another combat starts without it, and an undo that brings the creatures (or the spell)
    // back takes it away.
    effect(() => {
      const e = this.encounter();
      const spell = this.own()?.concentrationSpellNamePt ?? '';
      const creatures =
        e && !this.isMaster() ? ownCreatures(e).filter((c) => !!c.summonGroupId) : [];
      untracked(() => {
        const out = this.concentration.read(
          e?.id ?? '',
          spell,
          creatures,
          this.lostConcentration(),
        );
        if (out !== undefined) {
          this.lostConcentration.set(out);
        }
      });
    });
    // The master's prompts need each NPC reactor's attacks with their numbers.
    effect(() => {
      const e = this.encounter();
      if (!this.isMaster() || !e) {
        return;
      }
      for (const offer of this.toAnswer()) {
        if (
          reactorIsMasters(e, offer) &&
          !untracked(() => this.reactorOptions().has(offer.id) || this.reactorReading.has(offer.id))
        ) {
          untracked(() => void this.loadReactor(offer));
        }
      }
    });
  }

  /** Reads an NPC reactor's own options (its attacks with their numbers) for the master's prompt. */
  protected async loadReactor(offer: OpportunityOffer): Promise<void> {
    const e = this.encounter();
    if (!e) {
      return;
    }
    this.reactorReading.add(offer.id);
    this.reactorFailed.update((s) => {
      const next = new Set(s);
      next.delete(offer.id);
      return next;
    });
    try {
      const res = await this.api.turnOptions(this.campaignId(), e.id, offer.reactorId);
      this.reactorOptions.update((m) => new Map(m).set(offer.id, res));
    } catch {
      this.reactorFailed.update((s) => new Set(s).add(offer.id));
    } finally {
      this.reactorReading.delete(offer.id);
    }
  }

  // ---- calls ----

  protected begin(): Promise<boolean> {
    return this.calls.run((e) => this.api.begin(this.campaignId(), e.id));
  }

  /** `discard`: the master passes the turn although a damage waits. */
  protected nextTurn(discard = false): Promise<boolean> {
    // A death save still due: the server lets the master pass it, but he is asked first, in place.
    if (this.deathDue().length > 0) {
      this.passAsking.set({ discard });
      return Promise.resolve(false);
    }
    return this.endTurn(discard);
  }

  /** "Passar sem rolar": the turn ends with the death save unrolled. */
  protected passUnrolled(): Promise<boolean> {
    const asked = this.passAsking();
    this.passAsking.set(null);
    return this.endTurn(asked?.discard ?? false);
  }

  private endTurn(discard: boolean): Promise<boolean> {
    // In a joint turn a player ends their own part; the master ends the part of
    // the first member who still acts (the others have their own button).
    return this.calls.run((e) => {
      const own = this.own();
      const who = !this.isMaster() && own && acts(e, own) ? own.id : e.currentCombatantId;
      return this.api.endTurn(this.campaignId(), e.id, who, discard, e.round);
    });
  }

  /** "Revelar" or "Manter escondidas" on a question about hidden creatures (PM-02c state 9). */
  protected answerReveal(a: RevealAnswer): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.resolveHiddenReveal(this.campaignId(), e.id, a.id, a.reveal),
    );
  }

  /** The last question answered: the focus goes on to the order, the next stop of the column. */
  protected focusOrder(): void {
    afterNextRender(
      () =>
        this.host.nativeElement
          .querySelector<HTMLElement>(
            'app-order-list button:not([disabled]), app-order-list [tabindex="0"]',
          )
          ?.focus(),
      { injector: this.injector },
    );
  }

  /** "Encerrar a parte da Brisa": the master ends one member's part. */
  protected endPart(id: string): Promise<boolean> {
    return this.calls.run((e) => this.api.endTurn(this.campaignId(), e.id, id, false, e.round));
  }

  /** A standard action ("Disparada"): it spends the action; Dash doubles the
   * movement. */
  protected takeAction(key: string): Promise<boolean> {
    const own = this.own();
    return own ? this.takeActionFor(own.id, key) : Promise.resolve(false);
  }

  /** The same for a combatant of the player's: their character, or one of their creatures. */
  protected takeActionFor(id: string, key: string): Promise<boolean> {
    return this.calls.run(
      async (e) => (await this.api.takeAction(this.campaignId(), e.id, id, key)).encounter,
    );
  }

  /** A standard action of one of the player's creatures: "Ajudar" and "Esconder" open the same sheets as the character's,
   * for the creature (a familiar's Help is the classic one, SRD 5.1 Find Familiar); the others spend its action. */
  protected creatureAction(id: string, key: string): void {
    const options = this.creatureOptions.data().get(id) ?? null;
    if (key === 'standard:help') {
      this.contest.openHelpFor(id, options);
      return;
    }
    if (key === 'standard:hide') {
      this.contest.openHideFor(id, key, options);
      return;
    }
    void this.takeActionFor(id, key);
  }

  /** A standard action from the list: "Procurar" opens the search for traps (E9-08, it spends the action
   * through `SearchForTraps`); the others spend the action (`takeAction`). */
  protected standardAction(key: string): void {
    // Hide and Help are the contest service's (W7-X): the check, and the master's answer, are not a plain action.
    if (this.contest.hideKeys().includes(key)) {
      this.contest.openHide(key);
      return;
    }
    if (key === 'standard:help') {
      this.contest.openHelp();
      return;
    }
    if (
      key === 'standard:search' &&
      searchRoute(this.mapState().map()?.gridColumns ?? 0, this.own()?.placed ?? false) === 'traps'
    ) {
      this.sheets.searchTraps();
      return;
    }
    void this.takeAction(key);
  }

  /** "Usar" on a feature: Retomar o Fôlego rolls (its own sheet); the others
   * only spend the action and remind the table ("Você tem outra ação"). */
  protected async useFeature(key: string, frenzy = false): Promise<void> {
    const own = this.own();
    const e = this.encounter();
    const option = this.options()?.options?.featureActions.find((a) => a.action?.key === key);
    if (!own || !e || !option?.action) {
      return;
    }
    if (this.contest.hideKeys().includes(key)) {
      // The rogue's Cunning Action: Hide as a bonus action.
      this.contest.openHide(key);
      return;
    }
    const name = option.action.namePt;
    const flow = resourceFlow(key);
    if (flow) {
      this.sheets.resource(flow, key, own.id, e.id);
      return;
    }
    if (key === 'feature:second-wind') {
      this.sheets.secondWind(own, e, name, option.action.economy);
      return;
    }
    const actionKey = key;
    if (
      await this.calls.run(
        async (current) =>
          (
            await this.api.takeAction(
              this.campaignId(),
              current.id,
              own.id,
              actionKey,
              undefined,
              undefined,
              false,
              frenzy,
            )
          ).encounter,
      )
    ) {
      this.actionNote.set(featureNote(name, option.action.resourceKey, frenzy));
    }
  }

  /** The Bardic Inspiration die the player's character holds (the server sends it to the holder and the master only). */
  protected readonly ownDie = computed(() => this.own()?.inspirationDie ?? null);
  /** The attack roll of the player's character that waits for the answer about the die. */
  protected readonly ownOffer = computed(() => this.own()?.inspirationOffer ?? null);
  /** The attack roll of the player's halfling that waits for the Lucky answer ("Tirou 1: rolar de novo?"), asked first. */
  protected readonly ownLucky = computed(() => this.own()?.luckyOffer ?? null);
  /** "Responder": the question of a held roll (Lucky first, then the die's), at the d20 that was rolled. `false` while
   * the attack is not in the options. */
  protected openHeldRoll(): boolean {
    const held: HeldRoll = {
      offer: this.ownOffer() ?? undefined,
      lucky: this.ownLucky() ?? undefined,
    };
    const key = (held.lucky ?? held.offer)?.attackKey;
    const attack = this.options()?.options?.attacks.find((a) => a.attack?.key === key);
    if (!key || !attack) {
      return false;
    }
    this.sheets.attack({ key, held });
    return true;
  }

  /** "Atacar": the attack sheet (Alvo, Rolar, Dano). `asReaction` is the
   * opportunity attack, off turn. */
  protected openAttack(key: string, asReaction = false, useExtraAction = false): void {
    this.sheets.attack({ key, asReaction, useExtraAction });
  }

  /** A standard action paid with the extra action an effect gives (Velocidade): Disparada, Desengajar, Esconder, Usar um objeto. */
  protected takeExtra(key: string): void {
    const own = this.own();
    if (own) {
      void this.calls.run(
        async (e) =>
          (
            await this.api.takeAction(
              this.campaignId(),
              e.id,
              own.id,
              key,
              undefined,
              undefined,
              true,
            )
          ).encounter,
      );
    }
  }

  /** "Rolar o dano" of a hit whose sheet was closed before the damage. */
  protected rollPendingDamage(): void {
    const p = this.ownPendingRoll();
    if (!p) {
      return;
    }
    const attack = this.options()?.options?.attacks.find(
      (a) => a.attack?.key === p.attackKey,
    )?.attack;
    if (attack && attack.saveDc === 0) {
      this.sheets.attack({ key: p.attackKey, resume: p });
      return;
    }
    // A spell's damage (Mísseis Mágicos, Mãos Flamejantes): the cast sheet, at the damage.
    const own = this.own();
    const owed = (this.options()?.pendingDamages ?? []).filter(
      (x) =>
        x.status === PendingDamageStatus.AWAITING_ROLL &&
        x.attackerId === own?.id &&
        x.attackKey === p.attackKey,
    );
    this.sheets.castSheet(p.attackKey, owed);
  }

  /** "Atacar" on one of the player's creatures (E9-12): the same sheet, with the creature as the attacker; `asReaction` for the familiar of the Pacto da Corrente. */
  protected openCreatureAttack(id: string, key: string, asReaction = false): void {
    const who = this.encounter()?.combatants.find((c) => c.id === id);
    if (who) {
      this.sheets.attack({
        key,
        asReaction,
        who,
        creatureOpts: this.creatureOptions.data().get(id) ?? null,
      });
    }
  }

  /** A hit of one of the player's creatures whose damage waits to be rolled (from the creature's own options). */
  protected creaturePending(id: string): PendingDamage | null {
    return (
      this.creatureOptions
        .data()
        .get(id)
        ?.pendingDamages.find(
          (p) => p.status === PendingDamageStatus.AWAITING_ROLL && p.attackerId === id,
        ) ?? null
    );
  }

  /** "Rolar o dano" of a creature's hit: the attack sheet again, at the damage. */
  protected rollCreatureDamage(id: string): void {
    const p = this.creaturePending(id);
    const who = this.encounter()?.combatants.find((c) => c.id === id);
    if (p && who) {
      this.sheets.attack({
        key: p.attackKey,
        resume: p,
        who,
        creatureOpts: this.creatureOptions.data().get(id) ?? null,
      });
    }
  }

  /** The death save of the player's own turn (RN-03, RN-18 per roll). */
  protected async rollDeathSave(die: AttackDie): Promise<void> {
    const own = this.own();
    if (!own) {
      return;
    }
    const text = await this.rollDeathSaveOf(own.id, own.label, die);
    if (text) {
      this.deathResult.set(text);
    }
  }

  /** The master rolls the death save of a player who is not there (the server accepts the master, RN-03). */
  protected async rollMasterDeathSave(roll: DeathSaveRoll): Promise<void> {
    const who = this.encounter()?.combatants.find((c) => c.id === roll.id);
    if (!who) {
      return;
    }
    this.passAsking.set(null);
    const text = await this.rollDeathSaveOf(who.id, who.label, roll.die);
    if (text) {
      this.masterDeathResult.set(text);
    }
  }

  /** One death save call, with the dice animation; the sentence it announced, or '' when the call failed. */
  private async rollDeathSaveOf(id: string, label: string, die: AttackDie): Promise<string> {
    let text = '';
    await this.calls.run(async (e) => {
      const key = this.deathKey.keyFor([e.id, id, die]);
      const res = await this.api.rollDeathSave(this.campaignId(), e.id, id, die, key);
      this.deathKey.renew();
      text = saveAnnouncement(res.save, label);
      // The death save, as the line under the sheet tells it (hidden saves arrive without a roll: nothing plays).
      const show = showOfDice('Teste contra a morte', res.save.roll, {
        withTotal: true,
        outcome: {
          word: outcomeText(res.save.outcome),
          good:
            res.save.outcome === DeathSaveOutcome.SUCCESS ||
            res.save.outcome === DeathSaveOutcome.REVIVED,
        },
        critical: res.save.outcome === DeathSaveOutcome.REVIVED,
        fumble: res.save.outcome === DeathSaveOutcome.CRITICAL_FAILURE,
      });
      if (show) {
        this.animator.play(show);
      }
      if (res.save.outcome === DeathSaveOutcome.REVIVED && id === this.own()?.id) {
        this.actionNote.set(text);
      }
      return res.encounter;
    });
    return text;
  }

  /** The master's "Confirmar a morte" (ConfirmDeath): the character is dead for good. */
  protected async confirmDeath(id: string): Promise<void> {
    await this.calls.run(async (e) => {
      const res = await this.api.confirmDeath(
        this.campaignId(),
        e.id,
        id,
        this.confirmDeathKey.keyFor([e.id, id]),
      );
      this.confirmDeathKey.renew();
      return res;
    });
  }

  /** The raging barbarian the turn waits for, when the question is mine to answer: the owner's, or the master's. */
  protected readonly rageAsked = computed(() => {
    const e = this.encounter();
    const id = e?.ragePendingCombatantId;
    const who = id ? e.combatants.find((c) => c.id === id) : undefined;
    return who && (this.isMaster() || who.mine) ? who : null;
  });

  /** "Voltar e atacar" (`endRage` false) or "Deixar a fúria acabar": the answer to the rage question. */
  protected async answerRage(id: string, endRage: boolean): Promise<void> {
    // "Deixar a fúria acabar" ends the rage and then passes the turn, in one tap, as the
    // question says: the server only ends the rage, so the turn goes in the same call.
    await this.calls.run(async (e) => {
      const answered = await this.api.answerRageEnd(this.campaignId(), e.id, id, endRage);
      if (!endRage) {
        return answered;
      }
      this.state().apply(answered);
      return this.api.endTurn(this.campaignId(), e.id, id, false, answered.round);
    });
  }

  /** "Técnica da Mão Aberta" and "Golpe Atordoante": the monk's answer to an offer after a hit. */
  protected answerRider(pick: RiderPick): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.useHitRider(this.campaignId(), e.id, pick.id, pick.choice),
    );
  }

  /** "Encerrar fúria": the bonus action. */
  protected endRageOf(id: string): Promise<boolean> {
    return this.calls.run((e) => this.api.endRage(this.campaignId(), e.id, id));
  }

  protected deathLaterFor(id: string): void {
    this.deathLater.update((s) => new Set(s).add(id));
  }

  protected deathAskAgain(id: string): void {
    this.deathLater.update((s) => {
      const next = new Set(s);
      next.delete(id);
      return next;
    });
  }

  /** The player ends their own concentration (the conditions are the master's). */
  protected endOwnConcentration(): Promise<boolean> {
    const own = this.own();
    this.concentration.endedByMe = true;
    return own
      ? this.calls
          .run((e) =>
            this.api.setConditions(this.campaignId(), e.id, own.id, { endConcentration: true }),
          )
          .then((ok) => {
            if (!ok) {
              this.concentration.endedByMe = false;
            }
            return ok;
          })
      : Promise.resolve(false);
  }

  /** "Encerrar a parte dos Lobos" / "Encerrar a vez do Nanquim": the part of every member of the tab that still acts, one call each (a joint turn ends part by part). */
  protected async endTab(tab: MineTab): Promise<void> {
    for (const member of tab.members) {
      const e = this.encounter();
      // Read again before each call: the answer to the one before may have moved the turn on.
      if (!e || !membersToEnd(e, { ...tab, members: [member] }).length) {
        continue;
      }
      if (
        !(await this.calls.run((current) =>
          this.api.endTurn(this.campaignId(), current.id, member.id, false, current.round),
        ))
      ) {
        return;
      }
    }
  }

  /** The saving throw of an effect that waits for this player: it has to be answered, so it can be opened again after closing it. */
  protected readonly ownEffectSave = computed(() => {
    const e = this.encounter();
    if (this.isMaster() || !e || e.status !== EncounterStatus.ACTIVE) {
      return null;
    }
    return (
      openWindows(e).find(
        (w) => w.forYou && w.prompt.case === 'effectSave' && !this.reactionOpen(),
      ) ?? null
    );
  });

  /** A damage was settled: the master's card stays for its note. */
  protected settled(): void {
    this.settledTurn.set(this.subject()?.id ?? '');
  }

  protected endCombat(): Promise<boolean> {
    return this.calls.run((e) => this.api.end(this.campaignId(), e.id));
  }

  protected submitFace(combatantId: string, face: number): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.submitInitiative(this.campaignId(), e.id, combatantId, { face }),
    );
  }

  protected async rollInApp(combatantId: string): Promise<boolean> {
    const done = await this.calls.run((e) =>
      this.api.submitInitiative(this.campaignId(), e.id, combatantId, { inApp: true }),
    );
    if (done) {
      this.animateInitiative(combatantId);
    }
    return done;
  }

  /** The initiative d20 the app just rolled for the player's own combatant, as the initiative card tells it (d20 + bonus = total). */
  private animateInitiative(combatantId: string): void {
    const shown = initiativeLine(this.encounter()?.combatants.find((x) => x.id === combatantId));
    const show = shown && showOfDie('Iniciativa', INITIATIVE_DIE, shown.face, shown.line);
    if (show) {
      this.animator.play(show);
    }
  }

  protected setOrder(ids: string[]): Promise<boolean> {
    return this.calls.run((e) => this.api.setOrder(this.campaignId(), e.id, ids));
  }

  protected reveal(change: { id: string; hidden: boolean }): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.setHidden(this.campaignId(), e.id, change.id, change.hidden),
    );
  }

  /** The master: "Perdeu a concentração" (the app does not roll the save; the damage card reminds the DC). */
  protected loseConcentration(id: string): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.setConditions(this.campaignId(), e.id, id, { endConcentration: true }),
    );
  }

  /** The master: "Encerrar Ajuda": the bonus goes, and the hit points lose only what passes the new maximum. */
  protected endAid(id: string): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.endCombatEffect(this.campaignId(), e.id, id, CombatEffect.AID),
    );
  }

  protected remove(id: string): Promise<boolean> {
    return this.calls.run((e) => this.api.remove(this.campaignId(), e.id, id));
  }

  protected adjustFor(characterId: string): void {
    const v = this.vitals().find((x) => x.characterId === characterId);
    if (v) {
      this.adjust.emit(v);
    }
  }

  /** "Adicionar combatente": the start dialog, with the NPCs only. */
  protected addCombatants(): void {
    const e = this.encounter();
    if (!e) {
      return;
    }
    const phone = this.phone();
    this.dialog
      .open<StartCombatDialog, StartCombatData, Encounter>(StartCombatDialog, {
        data: {
          campaignId: this.campaignId(),
          mode: 'add',
          map: null,
          encounterId: e.id,
          existing: e.combatants.length,
        },
        width: phone ? '100vw' : '760px',
        maxWidth: phone ? '100vw' : 'calc(100vw - 32px)',
        height: phone ? '100dvh' : undefined,
        maxHeight: phone ? '100dvh' : '92dvh',
        ariaLabelledBy: 'start-title',
        autoFocus: 'first-tabbable',
      })
      .afterClosed()
      .subscribe((added) => added && this.state().apply(added));
  }

  // ---- opportunity attacks (E9-13) ----

  /** The master answers for an NPC: "Não atacar", or "Atacar com <arma>". */
  protected answerOpportunity(a: MasterAnswer): Promise<boolean> | void {
    if (!a.attack) {
      return this.calls.run((e) =>
        this.api.declineOpportunity(this.campaignId(), e.id, a.offer.id),
      );
    }
    this.sheets.opportunityAttack(
      a.offer,
      a.attack.key,
      this.reactorOptions().get(a.offer.id),
      true,
    );
  }

  /** "Desfazer o movimento": the master takes back his last action when it is the move that made the offers. */
  protected undoMove(): Promise<boolean> {
    const entry = this.log.undoable();
    if (!entry) {
      return Promise.resolve(false);
    }
    return this.calls.run(async (e) => {
      const undone = await this.api.undo(this.campaignId(), e.id, entry.id);
      this.undone.mark();
      return undone;
    });
  }

  /** "Seguir sem esperar": the master passes over an offer the player does not answer. */
  protected skipOpportunity(offer: OpportunityOffer): Promise<boolean> {
    return this.calls.run((e) => this.api.skipOpportunity(this.campaignId(), e.id, offer.id));
  }

  /** "Oferecer a Toren": the master says the one on turn left that combatant's reach (a combat without a map). */
  protected async makeOffer(reactorId: string): Promise<void> {
    const e = this.encounter();
    const mover = e ? currentCombatant(e) : null;
    if (!e || !mover) {
      return;
    }
    this.offerError.set('');
    let key = this.offerKeys.get(reactorId);
    if (!key) {
      key = newKey();
      this.offerKeys.set(reactorId, key);
    }
    const ok = await this.calls.run((current) =>
      this.api.offerOpportunity(this.campaignId(), current.id, mover.id, reactorId, key),
    );
    if (ok) {
      this.offering.set(false);
    } else if (this.error()) {
      // The refusal stays in the form, where the master is looking (the page's notice has it too).
      this.offerError.set(this.error());
      this.error.set('');
      // A refused offer is not a retry: the next try is a new one.
      this.offerKeys.delete(reactorId);
    }
  }

  /** "Oferecer ataque de oportunidade": opens the form under its button (a new form, new keys), or closes it. */
  protected toggleOffer(): void {
    if (!this.offering()) {
      this.offerKeys = new Map();
      this.offerError.set('');
    }
    this.offering.set(!this.offering());
  }

  /** "Retirar a oferta": the master takes back an offer nobody answered; the reactor keeps its reaction. */
  protected withdrawOpportunity(offer: OpportunityOffer): Promise<boolean> {
    return this.calls.run((e) => this.api.withdrawOpportunity(this.campaignId(), e.id, offer.id));
  }

  /** The master's "Marcar como aliado" / "Voltar a ser inimigo". */
  protected setSide(change: { id: string; side: CombatantSide }): Promise<boolean> {
    return this.calls.run((e) => this.api.setSide(this.campaignId(), e.id, change.id, change.side));
  }

  /** The master's cover mark, applied at once. */
  protected setCover(change: { id: string; cover: CoverDegree }): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.setCover(this.campaignId(), e.id, change.id, change.cover),
    );
  }

  // ---- moving ----

  /** "Levantar-se": the combatant (the player's own, a creature of theirs, or the one on the master's card) stands up from Prone. */
  protected standUp(id?: string): Promise<boolean> {
    const who = id ?? this.own()?.id;
    return who
      ? this.calls.run((e) => this.api.standUp(this.campaignId(), e.id, who))
      : Promise.resolve(false);
  }

  /** "Colocar no mapa": the server picks the square, by the rule that places the
   * NPCs when the combat begins (free floor, away from the players). */
  protected async place(id: string): Promise<void> {
    await this.calls.run((current) => this.api.place(this.campaignId(), current.id, id));
  }

  // ---- sheets and moving: the template and the screens below call these ----

  protected askFamiliarEyes(): void {
    const name = this.familiar();
    if (name) {
      this.sheets.familiarEyes(name);
    }
  }
  protected openTransform(): void {
    this.sheets.wildShape();
  }
  protected openDoor(door: DoorSquare): void {
    this.sheets.door(door);
  }
  protected describeSpell(key: string, name: string): void {
    this.sheets.describeSpell(key, name);
  }
  protected openCast(key: string): Promise<void> {
    return this.sheets.cast(key);
  }
  protected openMasterArea(key: string): void {
    this.sheets.masterArea(key);
  }
  protected openConditions(combatantId: string): void {
    this.sheets.conditions(combatantId);
  }
  protected openEffectSave(window: ReactionWindow): void {
    this.sheets.effectSave(window);
  }
  protected adjustNpc(combatantId: string): void {
    this.sheets.adjustNpc(combatantId);
  }
  protected openSpend(id?: string): void {
    this.sheets.spend(id);
  }
  protected drop(drop: TokenDrop): Promise<void> {
    return this.moves.drop(drop);
  }
  protected confirmMove(to: Square): Promise<void> {
    return this.moves.confirmMove(to);
  }
  protected confirmJump(req: JumpRequest): Promise<void> {
    return this.moves.confirmJump(req);
  }
  protected disengage(): Promise<boolean> {
    return this.moves.disengage();
  }
  protected closeFullPage(): void {
    this.moves.closeFullPage();
  }
  protected openMove(): void {
    this.moves.open();
  }
  protected openCreatureMove(id: string): void {
    this.moves.openCreature(id);
  }

  protected leave(): void {
    this.state().dismissEnded();
  }
}
