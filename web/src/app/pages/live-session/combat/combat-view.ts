import { NgTemplateOutlet } from '@angular/common';
import {
  Component,
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
import { Code, ConnectError } from '@connectrpc/connect';

import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import {
  type Combatant,
  CombatLogKind,
  CombatantState,
  DeathSaveOutcome,
  CombatantSide,
  CoverDegree,
  CoverSource,
  type Encounter,
  EncounterStatus,
  type OpportunityOffer,
  type PendingDamage,
  PendingDamageStatus,
  type ReactionPrompt,
  type GetTurnOptionsResponse,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  type Attack,
  AttackKind,
  type SpellDetails,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  type AttackDie,
  CombatClient,
  type MoveResult,
  newKey,
} from '../../../core/combat/combat-client';
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
import { MoveOptionsState } from '../../../core/combat/move-options-state';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { DeathSaveVisibility } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
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
import { article } from '../../../core/combat/combat-log';
import { ofThe } from '../../../core/combat/move-plan';
import {
  type ReactorAttack,
  barText,
  offersHolding,
  offersToAnswer,
  reactorAttacks,
  reactorIsMasters,
  waitingText,
} from '../../../core/combat/opportunity';
import type { FogView } from '../../../core/maps/fog-view';
import { LayersState } from '../../../core/maps/layers-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { type FallNote, fallNote } from '../../../core/traps/trap-log';
import { type SearchSkills, searchRoute } from '../../../core/traps/trap-search';
import type { TrapBoard } from '../../../core/traps/trap-board';
import type { OfferMark, Reach } from '../../../shared/combat-map/combat-map';
import type { Square } from '../../../core/combat/combat-grid';
import { openDamages, pendingNote } from '../../../core/combat/attack-flow';
import { CombatLogState } from '../../../core/combat/combat-log-state';
import type { CombatState } from '../../../core/combat/combat-state';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { TurnOptionsState } from '../../../core/combat/turn-options-state';
import { saveAnnouncement } from '../../../core/combat/death-saves';
import {
  currentCombatant,
  isDown,
  isPlayer,
  ownCombatant,
  npcKindLabel,
} from '../../../core/combat/combat-view';
import { acts, jointTurn } from '../../../core/combat/joint-turn';
import type { MapState } from '../../../core/maps/map-state';
import { MoveSaves } from '../../../core/maps/move-saves';
import { metersFixed } from '../../../core/units';
import { RosterClient } from '../../../core/maps/roster-client';
import type { TokenDrop } from '../../../shared/combat-map/combat-map';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import type { SpellDetailsData } from '../../../shared/spell-details/spell-details';
import { openSpellDetails } from '../../../shared/spell-details/open-spell-details';
import { spellDetailsFromGen } from '../../../shared/spell-details/spell-details-map';
import type { PartyMemberInfoVm, VitalsVm } from '../live-session.types';
import { ActionGroups } from './action-groups/action-groups';
import { AdjustNpc, type AdjustNpcData } from './adjust-npc/adjust-npc';
import { AttackSheet, type AttackSheetData } from './attack-sheet/attack-sheet';
import { CastSheet, type CastSheetData } from './cast-sheet/cast-sheet';
import { ConditionsDialog, type ConditionsData } from './conditions-dialog/conditions-dialog';
import { DeathQuestion } from './death-question/death-question';
import { DeathSaves } from './death-saves/death-saves';
import { FeatureSheet, type FeatureSheetData } from './feature-sheet/feature-sheet';
import { ShieldSheet, type ShieldSheetData } from './shield-sheet/shield-sheet';
import { CombatLogPanel } from './combat-log/combat-log-panel';
import type { CombatantInfo } from './combat-info';
import { CombatBar } from './combat-bar/combat-bar';
import { CombatMapCard } from './combat-map-card/combat-map-card';
import { CombatSummary } from './combat-summary/combat-summary';
import { JointCard } from './joint-turn/joint-card';
import { LegendaryOffers } from './monster-turn/legendary-offers';
import { InitiativeSetup } from './initiative-setup/initiative-setup';
import { InitiativeSide } from './initiative-side/initiative-side';
import { type JumpRequest, MovePage } from './move-page/move-page';
import { OpportunityCard, type MasterAnswer } from './opportunity/opportunity-card';
import {
  OpportunitySheet,
  type OpportunityAnswer,
  type OpportunitySheetData,
} from './opportunity/opportunity-sheet';
import { NpcCard } from './npc-card/npc-card';
import { OrderList } from './order-list/order-list';
import { PlayerInitiative } from './player-initiative/player-initiative';
import { CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
import type { Creature } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { openFamiliarEyes } from '../../../shared/familiar-eyes/familiar-eyes-sheet';
import { openWildShape } from '../../../shared/wild-shape/wild-shape-sheet';
import { BeastTraits } from '../../../shared/wild-shape/beast-traits';
import {
  SummonSheet,
  type SummonSheetData,
} from '../../character-sheet/creatures-panel/summon-sheet';
import { CreatureBar } from './creature-turn/creature-bar';
import { CreatureBlock } from './creature-turn/creature-block';
import { CreatureHero } from './creature-turn/creature-hero';
import { MineTabs } from './mine-tabs/mine-tabs';
import { openSheet } from './sheet-host';
import { type DoorSheetData, openDoorSheet } from '../door-sheet/door-sheet';
import type { DoorSquare } from '../../../core/maps/layers';
import { wallUnderDoor } from '../../../core/maps/door-paint';
import { type StartCombatData, StartCombatDialog } from './start-combat/start-combat-dialog';
import { TrapDamages } from '../traps/trap-damages/trap-damages';
import { openTrapSearch } from '../traps/trap-search-sheet/trap-search-sheet';
import { OrderColumn } from './turn-panel/order-column';
import { TurnBar } from './turn-panel/turn-bar';
import { TurnPanel } from './turn-panel/turn-panel';
import { CoverPanel } from './theatre/cover-panel';
import { NoMapPanel, TheatreReaction } from './theatre/no-map-panel';
import { OfferPanel } from './theatre/offer-panel';
import { SpendSheet, type SpendSheetData } from './theatre/spend-sheet';

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
@Component({
  selector: 'app-combat-view',
  imports: [
    ActionGroups,
    BeastTraits,
    CreatureBar,
    CreatureBlock,
    CreatureHero,
    DeathQuestion,
    DeathSaves,
    CombatBar,
    CombatLogPanel,
    CombatMapCard,
    CombatSummary,
    CoverPanel,
    InitiativeSetup,
    InitiativeSide,
    JointCard,
    LegendaryOffers,
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
    OrderList,
    PlayerInitiative,
    TheatreReaction,
    TurnBar,
    TrapDamages,
    TurnPanel,
  ],
  templateUrl: './combat-view.html',
  styleUrl: './combat-view.scss',
})
export class CombatView {
  private readonly api = inject(CombatClient);
  private readonly roster = inject(RosterClient);
  private readonly maps = inject(MapsClient);
  private readonly catalog = inject(SpellCatalog);
  private readonly dialog = inject(MatDialog);
  private readonly creaturesApi = inject(CreaturesClient);
  private readonly tableRules = inject(TableRulesClient);
  private readonly bottomSheet = inject(MatBottomSheet);
  protected readonly phone = mediaQuery(PHONE_QUERY);
  /** From 1024px the master has the combat bar (E6-11); below it the turn card does it all (E6-12). */
  protected readonly laptop = mediaQuery('(min-width: 1024px)');
  /** From 1280px the player's order is a column on the left (E6-14). */
  protected readonly wideLayout = mediaQuery('(min-width: 1280px)');
  /** One save in flight per combatant (see `MoveSaves`). */
  private readonly moves = new MoveSaves();
  /** The key of a jump: the same jump again is a retry of it. */
  private readonly jumpKey = new ActionKey();

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

  protected readonly Status = EncounterStatus;
  /** What the combatant in the spotlight can do: the player's own on their
   * turn, the master's current one (`GetTurnOptions`). */
  protected readonly turn = new TurnOptionsState();
  protected readonly log = new CombatLogState();
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** A refusal of the move on the "Mover" page (TOO_FAR, SQUARE_OCCUPIED). */
  protected readonly moveError = signal('');
  protected readonly rosterInfo = signal<ReadonlyMap<string, CombatantInfo>>(new Map());

  /** What the last feature said ("Surto de Ação: você tem outra ação"), and the
   * death save just rolled this turn; both are cleared when the turn changes. */
  protected readonly actionNote = signal('');
  protected readonly deathResult = signal('');
  /** The turn whose damage was just settled: the master's card stays for its note. */
  private readonly settledTurn = signal('');
  /** The characters at three failures the master put away with "Ainda não". */
  protected readonly deathLater = signal<ReadonlySet<string>>(new Set());
  private readonly deathKey = new ActionKey();
  private readonly confirmDeathKey = new ActionKey();
  private readonly leaveFormKey = new ActionKey();
  private readonly shieldHandled = new Set<string>();

  protected readonly encounter = computed(() => this.state().shown());
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
  /** The table hides the death saves from the other players (RN-24); `null` while the rule is not read (then every screen shows them as before). */
  private readonly deathRule = signal<DeathSaveVisibility | null>(null);
  protected readonly deathsHidden = computed(
    () => this.deathRule() === DeathSaveVisibility.OWNER_AND_MASTER,
  );
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
  /** The player's familiar ("Nanquim"), for the action "Ver pelos olhos do Nanquim": read once for the character, and again when the combat changes who they are. */
  protected readonly familiar = signal<string | null>(null);
  /** "Ver pelos olhos do Nanquim": the question first; the stream brings the band, the action spent and the blind line. */
  protected askFamiliarEyes(): void {
    const mine = this.own();
    const name = this.familiar();
    if (!mine || !name) {
      return;
    }
    openFamiliarEyes(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      characterId: mine.characterId,
      characterName: mine.label,
      familiarName: name,
      inCombat: true,
    }).subscribe();
  }

  // ---- Forma Selvagem (MR-037, E9-11) ----

  /** The character's own vitals row: the hit points that wait while it is a beast, and the uses of its resources. */
  protected readonly ownVitals = computed(() => {
    const own = this.own();
    return own ? (this.vitals().find((v) => v.characterId === own.characterId) ?? null) : null;
  });
  /** The book's stat block of the beast the druid is, for its traits (read once per beast). */
  protected readonly beastBlock = signal<Creature | null>(null);
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
  /** "Transformar": the list of beasts the level allows. */
  protected openTransform(): void {
    const own = this.own();
    if (!own) {
      return;
    }
    const resource = this.ownVitals()?.resources?.find((r) => r.key === 'wild_shape');
    openWildShape(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      characterId: own.characterId,
      inCombat: true,
      uses: resource ? { left: resource.total - resource.used, total: resource.total } : null,
    }).subscribe((result) => {
      if (result?.encounter) {
        this.state().apply(result.encounter);
      }
    });
  }

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
      await this.refreshAfter(err);
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
  /** The creature whose "Mover" page is open (`''`: the character's own). */
  protected readonly moverId = signal('');
  /** Whoever the "Mover" page is for: one of the creatures, or the character. */
  private readonly mover = computed(() => {
    const e = this.encounter();
    const id = this.moverId();
    // A creature that left the combat is not the character: nothing moves, and the page closes (the effect below says so).
    return e ? (id ? (e.combatants.find((c) => c.id === id) ?? null) : this.own()) : null;
  });
  /** The name of the creature the page was opened for, to say it left. */
  private moverName = '';
  private readonly moverActs = computed(() => {
    const e = this.encounter();
    const who = this.mover();
    return !!e && !!who && acts(e, who);
  });
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
  /** Where the combatant of the move page can go (`GetMoveOptions`): the player's own, or whoever the master's reach is on for. */
  protected readonly moveOptions = new MoveOptionsState();
  /** The master's "Mostrar o alcance": the reach of whoever is on turn, drawn on the map. */
  protected readonly reachOn = signal(false);
  /** The master's "Movimento forçado" box: it is for one drag, and goes off when the drag is taken. */
  protected readonly forcedMove = signal(false);
  /** What the last forced drag did, for the live region; cleared by the next drag. */
  protected readonly forcedNote = signal('');
  /** "Você parou antes: algo bloqueou o caminho.": what the last move said when it stopped short. */
  protected readonly moveNote = signal('');
  /** The last move stopped before a locked door: the "Mover" page stays open and says so (RN-26). */
  protected readonly lockedDoor = signal(false);
  /** The reach the master's map draws, as the server said it. */
  protected readonly masterReach = computed<Reach | null>(() => {
    const e = this.encounter();
    const who = e ? currentCombatant(e) : null;
    const options = this.moveOptions.data();
    return this.isMaster() && this.reachOn() && who?.placed && options
      ? {
          origin: { col: who.col, row: who.row },
          leftDft: options.movementLeftDft,
          squares: options.reachable.map((q) => ({ col: q.col, row: q.row })),
        }
      : null;
  });
  protected readonly reachSwitch = computed(() => {
    const e = this.encounter();
    const who = e ? currentCombatant(e) : null;
    return this.isMaster() && who?.placed ? { name: ofThe([who.label]), on: this.reachOn() } : null;
  });

  // ---- opportunity attacks (E9-13) ----

  /** The offers the caller answers, and the ones on the player's own mover. */
  protected readonly toAnswer = computed(() => {
    const e = this.encounter();
    return e ? offersToAnswer(e) : [];
  });
  /** What the waiting mover reads: their move landed, and each offer waits for its answer. */
  protected readonly waiting = computed(() => {
    const e = this.encounter();
    const own = this.own();
    return e && own && !this.isMaster() ? waitingText(e, offersHolding(e, own.id)) : null;
  });
  /** The master's bar: "Esperando a sua reação: Goblin 2". */
  protected readonly waitNote = computed(() => {
    const e = this.encounter();
    return e && this.isMaster()
      ? barText(e, (characterId) => this.info().get(characterId)?.playerName ?? '')
      : '';
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
  /** The reactors drawn on the map for whoever answers, with the square the mover left. */
  protected readonly offerMarks = computed<readonly OfferMark[]>(() => {
    const e = this.encounter();
    if (!e) {
      return [];
    }
    return e.opportunityOffers.flatMap((o) => {
      const r = e.combatants.find((c) => c.id === o.reactorId && c.placed);
      return r
        ? [
            {
              reactor: { col: r.col, row: r.row },
              left: o.forYou ? { col: o.leftCol, row: o.leftRow } : null,
            },
          ]
        : [];
    });
  });
  /** "Desengajar" can still be taken here: the action is free and it was not. */
  protected readonly canDisengage = computed(() => {
    const who = this.mover();
    return !!who && !who.actionUsed && !who.disengaged;
  });
  /** How far and how high the mover can jump: the creature's own options, or the character's. */
  protected readonly moverJumps = computed(() =>
    this.moverId()
      ? this.creatureOptions.data().get(this.moverId())?.options?.jumps
      : this.options()?.options?.jumps,
  );
  private readonly offersHandled = new Set<string>();
  private readonly reactorReading = new Set<string>();
  /** The offers whose reactor's attacks could not be read: the master's card offers "Tentar de novo". */
  protected readonly reactorFailed = signal<ReadonlySet<string>>(new Set());
  /** Where the player dropped their token on the main map: the "Mover" page starts there (the drop never moves). */
  protected readonly dropStart = signal<Square | null>(null);
  /** The master's last action is a move: the log's undoable entry says so ("Desfazer o movimento"). */
  protected readonly canUndoMove = computed(
    () => this.log.undoable()?.kind === CombatLogKind.MOVED,
  );
  protected readonly turnSide = computed(() => this.subject()?.side ?? CombatantSide.UNSPECIFIED);
  /** Who has the cover each combatant has against whoever has the turn (the master's order list). */
  protected readonly coverAgainst = computed(() => {
    const out = new Map<string, { cover: CoverDegree; source: CoverSource }>();
    const opts = this.options();
    for (const t of [
      ...(opts?.attackTargets ?? []).flatMap((a) => a.targets),
      ...(opts?.spellTargets ?? []).flatMap((s) => s.targets),
    ]) {
      if (
        !out.has(t.combatantId) &&
        t.cover !== CoverDegree.NONE &&
        t.cover !== CoverDegree.UNSPECIFIED
      ) {
        out.set(t.combatantId, { cover: t.cover, source: t.coverSource });
      }
    }
    return out;
  });
  protected readonly turnLabel = computed(() =>
    this.isMaster() ? (this.subject()?.label ?? '') : '',
  );
  /** The one whose options the screen asks for: the player's own character
   * on their turn, and for the master whoever is on turn. */
  protected readonly subject = computed(() => {
    const e = this.encounter();
    if (!e || e.status !== EncounterStatus.ACTIVE) {
      return null;
    }
    // A player's options are asked off turn too: the opportunity attack
    // (`attack_targets` is filled, `as_reaction`) needs them.
    return this.isMaster() ? currentCombatant(e) : this.own();
  });
  protected readonly options = computed(() => this.turn.data());
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
  /** The melee attacks an opportunity attack can use: off turn, with the reaction
   * free, someone in reach and the character on its feet (E6-28). */
  protected readonly opportunities = computed(() => {
    const own = this.own();
    const opts = this.options();
    if (
      this.isMaster() ||
      !own ||
      this.myTurn() ||
      !this.active() ||
      own.reactionUsed ||
      isDown(own) ||
      !opts?.options
    ) {
      return [];
    }
    return opts.options.attacks.flatMap((a) => {
      const atk = a.attack;
      // The melee reach is 5 ft, whatever range the weapon has when thrown.
      const reach = atk
        ? opts.attackTargets
            .find((t) => t.attackKey === atk.key)
            ?.targets.some((t) => t.distanceFt !== undefined && t.distanceFt <= 5)
        : false;
      return atk && atk.kind === AttackKind.WEAPON && atk.saveDc === 0 && atk.melee && reach
        ? [{ key: atk.key, name: atk.namePt || atk.name }]
        : [];
    });
  });
  /** The characters at three failures: the master is asked to confirm each death (E6-30). */
  protected readonly dying = computed(() =>
    this.isMaster()
      ? (this.encounter()?.combatants ?? []).filter((c) => c.state === CombatantState.DYING)
      : [],
  );
  /** The player's hit whose damage is still to roll (the sheet was closed). */
  protected readonly ownPendingRoll = computed(() => {
    const own = this.own();
    return (
      this.options()?.pendingDamages.find(
        (p) => p.status === PendingDamageStatus.AWAITING_ROLL && p.attackerId === own?.id,
      ) ?? null
    );
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
  /** The newest attack of the one on turn was stopped by the target's Escudo (from the log). */
  protected readonly reactionStopped = computed(() => {
    const who = this.subject();
    const entry = this.log
      .rounds()
      .flatMap((r) => r.entries)
      .find((x) => x.kind === CombatLogKind.ATTACK && x.actorId === who?.id);
    return !!entry?.stoppedByReaction;
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

  /** The SRD's details of each spell in the list (read once, cached by the catalog): the line under a spell's name. */
  protected readonly spellDetails = signal<ReadonlyMap<string, SpellDetails>>(new Map());

  private ruleKey = '';
  /** Changes only when the turn passes (or the combat ends): a string, so the effects that read it ignore the combat's other updates. */
  private readonly turnKey = computed(() => {
    const e = this.encounter();
    return e && e.status === EncounterStatus.ACTIVE
      ? `${e.id}:${e.round}:${e.currentCombatantId}`
      : '';
  });

  /** The table's rule on who sees the death saves (RN-24): read when a combat shows and again when someone falls, so a rule changed meanwhile is caught. */
  /** Whose familiar to ask for: the character's id, so a re-read of the combat changes nothing. */
  private readonly familiarOwner = computed(() =>
    this.isMaster() || this.lookingThroughFamiliar() ? '' : (this.own()?.characterId ?? ''),
  );
  private familiarRequest = 0;

  private async readDeathRule(campaignId: string): Promise<void> {
    try {
      this.deathRule.set((await this.tableRules.get(campaignId)).saved.deathSaves);
    } catch {
      // Without the rule the screens show the marks as they always did.
    }
  }

  constructor() {
    effect(() => {
      const e = this.encounter();
      const campaignId = this.campaignId();
      const key = e ? `${e.id}:${e.combatants.some(isDown)}` : '';
      untracked(() => {
        if (key !== '' && key !== this.ruleKey) {
          this.ruleKey = key;
          void this.readDeathRule(campaignId);
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
    // The familiar's name, for the action "Ver pelos olhos do Nanquim" (the owner's list: RN-20).
    effect(() => {
      const characterId = this.familiarOwner();
      const campaignId = this.campaignId();
      const request = ++this.familiarRequest;
      if (!characterId) {
        untracked(() => this.familiar.set(null));
        return;
      }
      untracked(
        () =>
          void this.creaturesApi.list(campaignId, characterId).then(
            (list) => {
              if (request === this.familiarRequest) {
                this.familiar.set(
                  list.find((c) => c.source === CreatureSource.FAMILIAR)?.name ?? null,
                );
              }
            },
            () => {
              if (request === this.familiarRequest) {
                this.familiar.set(null);
              }
            },
          ),
      );
    });
    effect(() => {
      const spells = this.options()?.options?.spells ?? [];
      const campaignId = this.campaignId();
      for (const s of spells) {
        const key = s.spell?.key ?? '';
        if (!key || untracked(() => this.spellDetails().has(key))) {
          continue;
        }
        void this.catalog.details(campaignId, key).then((d) => {
          if (d) {
            this.spellDetails.update((m) => new Map(m).set(key, d));
          }
        });
      }
    });
    // The master's lists need the roster (the kind of each NPC, the class
    // line, the player's name).
    effect(() => {
      if (this.isMaster() && this.campaignId()) {
        void this.loadRoster(this.campaignId());
      }
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
          this.moveNote.set('');
        });
      }
    });
    // A hit on the player's character that waits for their reaction opens Escudo (E6-28).
    effect(() => {
      const e = this.encounter();
      const own = this.own();
      if (this.isMaster() || !e || !own || e.status !== EncounterStatus.ACTIVE) {
        return;
      }
      const prompt = e.reactionPrompts.find((p) => p.targetId === own.id);
      if (prompt && !this.shieldHandled.has(prompt.pendingDamageId)) {
        this.shieldHandled.add(prompt.pendingDamageId);
        untracked(() => this.openShield(prompt));
      }
    });
    // "Mover" belongs to the player's turn: when it ends, the page closes.
    effect(() => {
      if (!this.moverActs() || !this.active()) {
        untracked(() => {
          if (this.moverId() && this.active() && !this.mover() && this.moverName) {
            this.moveNote.set(
              `${article(this.moverName) === 'a' ? 'A' : 'O'} ${this.moverName} saiu do combate.`,
            );
          }
          this.state().moving.set(false);
          this.moverId.set('');
        });
      }
      if (!this.active()) {
        this.state().mapOpen.set(false);
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
    // Where the one on the "Mover" page (or the master's reach) can go: asked again whenever the combat changes.
    effect(() => {
      const e = this.encounter();
      const who = this.moveSubject();
      const campaignId = this.campaignId();
      if (!e || !who) {
        untracked(() => this.moveOptions.clear());
        return;
      }
      void e.revision;
      untracked(() => void this.moveOptions.load(this.api, campaignId, e.id, who));
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
          untracked(() => void this.openOpportunity(offer));
        }
      }
    });
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
    // The beast's book (its armor class and traits), read once for each beast the druid becomes.
    effect(() => {
      const key = this.own()?.wildShapeBeastKey ?? '';
      const campaignId = this.campaignId();
      untracked(() => {
        if (!key) {
          this.beastBlock.set(null);
          return;
        }
        void this.creaturesApi.statBlock(campaignId, key).then(
          (block) => this.beastBlock.set(block),
          () => this.beastBlock.set(null),
        );
      });
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

  /** Whose reach the server is asked for: the player's own on the "Mover" page, the master's on turn with the reach on. */
  private readonly moveSubject = computed(() => {
    const e = this.encounter();
    if (!e || e.status !== EncounterStatus.ACTIVE) {
      return null;
    }
    if (this.isMaster()) {
      const who = currentCombatant(e);
      return this.reachOn() && who?.placed ? who.id : null;
    }
    const who = this.mover();
    return this.state().moving() && this.moverActs() && who?.placed ? who.id : null;
  });

  private async loadRoster(campaignId: string): Promise<void> {
    try {
      const entries = await this.roster.list(campaignId);
      this.rosterInfo.set(
        new Map(
          entries.map((e) => [
            e.id,
            {
              classSummary: e.classSummary,
              playerName: e.playerName,
              kindLabel: e.kind === CharacterKind.PLAYER ? '' : npcKindLabel(e.kind),
              raceName: e.raceName,
            },
          ]),
        ),
      );
    } catch {
      // Best effort: the lists fall back to what the combat itself says.
    }
  }

  // ---- calls ----

  /** Runs a call that answers with the combat: applies it, or says why not.
   * A refusal that means the screen is stale reads the combat again. */
  private async run(call: (e: Encounter) => Promise<Encounter>): Promise<boolean> {
    const e = this.encounter();
    if (!e || this.busy()) {
      return false;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      this.state().apply(await call(e));
      return true;
    } catch (err) {
      this.error.set(combatErrorMessage(err));
      await this.refreshAfter(err);
      return false;
    } finally {
      this.busy.set(false);
    }
  }

  private async refreshAfter(err: unknown): Promise<void> {
    const code = ConnectError.from(err).code;
    if (
      code === Code.Aborted ||
      code === Code.FailedPrecondition ||
      code === Code.NotFound ||
      code === Code.Unavailable ||
      code === Code.DeadlineExceeded
    ) {
      const ticket = this.state().beginRead();
      try {
        this.state().applyRead(ticket, await this.api.get(this.campaignId()));
      } catch {
        // The stream's next `ready` reads it again.
      }
    }
  }

  protected begin(): Promise<boolean> {
    return this.run((e) => this.api.begin(this.campaignId(), e.id));
  }

  /** `discard`: the master passes the turn although a damage waits. */
  protected nextTurn(discard = false): Promise<boolean> {
    // In a joint turn a player ends their own part; the master ends the part of
    // the first member who still acts (the others have their own button).
    return this.run((e) => {
      const own = this.own();
      const who = !this.isMaster() && own && acts(e, own) ? own.id : e.currentCombatantId;
      return this.api.endTurn(this.campaignId(), e.id, who, discard, e.round);
    });
  }

  /** "Encerrar a parte da Brisa": the master ends one member's part. */
  protected endPart(id: string): Promise<boolean> {
    return this.run((e) => this.api.endTurn(this.campaignId(), e.id, id, false, e.round));
  }

  /** A standard action ("Disparada"): it spends the action; Dash doubles the
   * movement. */
  protected takeAction(key: string): Promise<boolean> {
    const own = this.own();
    return own ? this.takeActionFor(own.id, key) : Promise.resolve(false);
  }

  /** The same for a combatant of the player's: their character, or one of their creatures. */
  protected takeActionFor(id: string, key: string): Promise<boolean> {
    return this.run(
      async (e) => (await this.api.takeAction(this.campaignId(), e.id, id, key)).encounter,
    );
  }

  /** A standard action from the list: "Procurar" opens the search for traps (E9-08, it spends the action
   * through `SearchForTraps`); the others spend the action (`takeAction`). */
  protected standardAction(key: string): void {
    if (
      key === 'standard:search' &&
      searchRoute(this.mapState().map()?.gridColumns ?? 0, this.own()?.placed ?? false) === 'traps'
    ) {
      this.searchTraps();
      return;
    }
    void this.takeAction(key);
  }

  private searchTraps(): void {
    openTrapSearch(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      skills: this.trapSkills(),
      diceMode: this.diceMode(),
      preference: this.dicePreference(),
      state: this.mapState(),
      inCombat: true,
    }).subscribe();
  }

  /** What a trap did to this player's character this round (E9-08 E), from the combat log. */
  protected readonly trapNote = computed(() => {
    const e = this.encounter();
    const own = this.own();
    if (!e || !own) {
      return null;
    }
    const round = this.log.rounds().find((r) => r.round === e.round);
    for (const entry of round?.entries ?? []) {
      if (entry.kind === CombatLogKind.TRAP_TRIGGERED && entry.trap) {
        const note = fallNote(entry.trap, own.id);
        if (note) {
          return note;
        }
      }
    }
    return null;
  });

  /** The same note for each of the player's creatures that a trap caught (a wolf or Nanquim that walked into one on its own part). */
  protected readonly creatureTrapNotes = computed(() => {
    const e = this.encounter();
    const notes = new Map<string, FallNote>();
    if (!e || this.isMaster()) {
      return notes;
    }
    const round = this.log.rounds().find((r) => r.round === e.round);
    for (const c of ownCreatures(e)) {
      for (const entry of round?.entries ?? []) {
        if (entry.kind === CombatLogKind.TRAP_TRIGGERED && entry.trap) {
          const note = fallNote(entry.trap, c.id, c.label);
          if (note) {
            notes.set(c.id, note);
            break;
          }
        }
      }
    }
    return notes;
  });

  /** "Usar" on a feature: Retomar o Fôlego rolls (its own sheet); the others
   * only spend the action and remind the table ("Você tem outra ação"). */
  protected async useFeature(key: string): Promise<void> {
    const own = this.own();
    const e = this.encounter();
    const option = this.options()?.options?.featureActions.find((a) => a.action?.key === key);
    if (!own || !e || !option?.action) {
      return;
    }
    const name = option.action.namePt;
    if (key === 'feature:second-wind') {
      const data: FeatureSheetData = {
        campaignId: this.campaignId(),
        encounterId: e.id,
        combatantId: own.id,
        actionKey: key,
        name,
        cost: option.action.economy === ActionEconomy.BONUS_ACTION ? 'Ação bônus' : 'Ação',
        diceMode: this.diceMode(),
        preference: this.dicePreference(),
        state: this.state(),
      };
      openSheet<FeatureSheet, FeatureSheetData, boolean>(
        this.dialog,
        this.bottomSheet,
        FeatureSheet,
        {
          data,
          ariaLabel: name,
          labelledBy: 'sheet-t',
        },
      ).subscribe();
      return;
    }
    const actionKey = key;
    // Surto de Ação (any level's feature) gives the action back.
    const note =
      option.action.resourceKey === 'action_surge'
        ? `${name}: você tem outra ação.`
        : `Usou ${name}.`;
    if (
      await this.run(
        async (current) =>
          (await this.api.takeAction(this.campaignId(), current.id, own.id, actionKey)).encounter,
      )
    ) {
      this.actionNote.set(note);
    }
  }

  /** "Atacar": the attack sheet (Alvo, Rolar, Dano). `asReaction` is the
   * opportunity attack, off turn. */
  protected openAttack(key: string, asReaction = false): void {
    this.attackSheet(key, undefined, asReaction);
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
      this.attackSheet(p.attackKey, p);
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
    this.castSheet(p.attackKey, owed);
  }

  /** "Atacar" on one of the player's creatures (E9-12): the same sheet, with the creature as the attacker; `asReaction` for the familiar of the Pacto da Corrente. */
  protected openCreatureAttack(id: string, key: string, asReaction = false): void {
    const who = this.encounter()?.combatants.find((c) => c.id === id);
    if (who) {
      this.attackSheet(
        key,
        undefined,
        asReaction,
        who,
        this.creatureOptions.data().get(id) ?? null,
      );
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
      this.attackSheet(p.attackKey, p, false, who, this.creatureOptions.data().get(id) ?? null);
    }
  }

  private attackSheet(
    key: string,
    resume?: PendingDamage,
    asReaction = false,
    who?: Combatant,
    creatureOpts?: GetTurnOptionsResponse | null,
  ): void {
    const e = this.encounter();
    const own = who ?? this.own();
    const opts = who ? creatureOpts : this.options();
    const option = opts?.options?.attacks.find((a) => a.attack?.key === key);
    const attack = option?.attack;
    if (!e || !own || !attack) {
      return;
    }
    const data: AttackSheetData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      attackerId: own.id,
      round: e.round,
      attack,
      targets: opts?.attackTargets.find((t) => t.attackKey === key)?.targets ?? [],
      diceMode: this.diceMode(),
      preference: this.dicePreference(),
      state: this.state(),
      asReaction,
      bonusRule: option?.bonusRule,
      bonusAttacksLeft: option?.bonusAttacksLeft,
      beamsLeft: option?.beamsLeft || attack.beams,
      attacksLeft: who ? (opts?.options?.economy?.attacksLeft ?? 0) : this.attacksLeft(),
      attacksPerAction: who
        ? (opts?.options?.economy?.attacksPerAction ?? 1)
        : this.attacksPerAction(),
      resume: resume
        ? {
            pending: resume,
            targetLabel: e.combatants.find((c) => c.id === resume.targetId)?.label ?? '',
          }
        : undefined,
    };
    openSheet<AttackSheet, AttackSheetData, boolean>(this.dialog, this.bottomSheet, AttackSheet, {
      data,
      ariaLabel: asReaction
        ? `Ataque de oportunidade com ${attack.namePt || attack.name}`
        : `Atacar com ${attack.namePt || attack.name}${who ? `: ${who.label}` : ''}`,
      labelledBy: 'sheet-t',
    }).subscribe();
  }

  /** The master taps a door of the map: its sheet opens (open, close, lock, or reveal a secret door). The map reads itself again on the stream. */
  protected openDoor(door: DoorSquare): void {
    const e = this.encounter();
    if (!e) {
      return;
    }
    const data: DoorSheetData = {
      campaignId: this.campaignId(),
      mapId: e.mapId,
      door,
      wallSquares: wallUnderDoor(
        this.layers().walls,
        this.layers().columns,
        this.layers().rows,
        this.mapState().map()?.squareFactor ?? 1,
        door,
      ),
    };
    openDoorSheet(this.dialog, this.bottomSheet, data).subscribe();
  }

  /** The "?" of a spell: its description (the SRD's, read once through the catalog), a sheet on a phone. */
  protected describeSpell(key: string, name: string): void {
    openSpellDetails(this.dialog, this.bottomSheet, this.spellDetailsData(key, name));
  }

  /** What the details sheet needs: the name now, and the text when it loads. */
  private spellDetailsData(key: string, name: string): SpellDetailsData {
    const campaignId = this.campaignId();
    return {
      namePt: name,
      load: async () => {
        const details = await this.catalog.details(campaignId, key);
        if (!details) {
          throw new Error('spell details unavailable');
        }
        return spellDetailsFromGen(details);
      },
    };
  }

  /** "Conjurar": the cast sheet (the slot, the targets, the roll, the damage); a summoning spell asks for the creatures instead. */
  protected async openCast(key: string): Promise<void> {
    const spell = this.options()?.options?.spells.find((s) => s.spell?.key === key);
    if (spell?.spell && (await this.isSummon(key))) {
      this.summonSheet(key, spell.spell.namePt || spell.spell.name);
      return;
    }
    this.castSheet(key);
  }

  /** The cast sheet for a spell of the options, or a cantrip that asks for a
   * saving throw; with `resume` it opens at the damage the player never rolled. */
  private castSheet(key: string, resume?: readonly PendingDamage[]): void {
    const e = this.encounter();
    const own = this.own();
    const opts = this.options();
    if (!e || !own || !opts?.options) {
      return;
    }
    const spell = opts.options.spells.find((s) => s.spell?.key === key);
    const cantrip = opts.options.attacks.find((a) => a.attack?.key === key)?.attack;
    const name = spell?.spell
      ? spell.spell.namePt || spell.spell.name
      : cantrip
        ? cantrip.namePt || cantrip.name
        : key;
    const vitals = this.vitals().find((v) => v.characterId === own.characterId);
    const shield = opts.options.spells.find((s) => s.spell?.key === 'spell:shield');
    const attackBonus = this.spellAttackBonus(
      opts.options.attacks.flatMap((a) => (a.attack ? [a.attack] : [])),
    );
    const data: CastSheetData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      casterId: own.id,
      round: e.round,
      spellKey: key,
      name,
      level: spell?.spell?.level ?? 0,
      concentration: spell?.spell?.concentration ?? false,
      cantripDice: cantrip?.spellDice ?? '',
      economy: spell?.economy ?? ActionEconomy.ACTION,
      slots: spell?.slots ?? [],
      usage: vitals?.spellSlots ?? [],
      pact: vitals?.pactSlots ?? null,
      targets: opts.spellTargets.find((t) => t.spellKey === key),
      shieldFree: shield ? shield.slots.reduce((n, s) => n + s.free, 0) : null,
      shieldName: shield?.spell?.namePt || 'Escudo Arcano',
      attackBonus,
      diceMode: this.diceMode(),
      preference: this.dicePreference(),
      state: this.state(),
      resume,
    };
    openSheet<CastSheet, CastSheetData, boolean>(this.dialog, this.bottomSheet, CastSheet, {
      data,
      ariaLabel: resume ? `Rolar o dano de ${name}` : `Conjurar ${name}`,
      labelledBy: 'sheet-t',
    }).subscribe();
  }

  /** The spell attack bonus, for a typed d20: the one of a spell attack in the
   * options, or a save cantrip's DC minus 8 (the same proficiency and modifier). */
  private spellAttackBonus(attacks: readonly Attack[]): number {
    const spellAttack = attacks.find((a) => a.kind === AttackKind.SPELL && a.saveDc === 0);
    if (spellAttack) {
      return spellAttack.attackBonus;
    }
    const save = attacks.find((a) => a.saveDc > 0);
    return save ? save.saveDc - 8 : 0;
  }

  /** The death save of the player's own turn (RN-03, RN-18 per roll). */
  protected async rollDeathSave(die: AttackDie): Promise<void> {
    const own = this.own();
    if (!own) {
      return;
    }
    await this.run(async (e) => {
      const key = this.deathKey.keyFor([e.id, own.id, die]);
      const res = await this.api.rollDeathSave(this.campaignId(), e.id, own.id, die, key);
      this.deathKey.renew();
      const text = saveAnnouncement(res.save, own.label);
      this.deathResult.set(text);
      if (res.save.outcome === DeathSaveOutcome.REVIVED) {
        this.actionNote.set(text);
      }
      return res.encounter;
    });
  }

  /** The master's "Confirmar a morte" (ConfirmDeath): the character is dead for good. */
  protected async confirmDeath(id: string): Promise<void> {
    await this.run(async (e) => {
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
      ? this.run((e) =>
          this.api.setConditions(this.campaignId(), e.id, own.id, { endConcentration: true }),
        ).then((ok) => {
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
        !(await this.run((current) =>
          this.api.endTurn(this.campaignId(), current.id, member.id, false, current.round),
        ))
      ) {
        return;
      }
    }
  }

  /** What the player's combatants say a summoning spell is: the keys `GetSummonOptions` lists, read once for the character. */
  private summonSpells: Promise<ReadonlySet<string>> | null = null;

  private isSummon(key: string): Promise<boolean> {
    const own = this.own();
    if (!own) {
      return Promise.resolve(false);
    }
    // A failed read is not kept: the next cast asks again, so one error never hides Conjurar Animais for the whole session.
    this.summonSpells ??= this.creaturesApi.summonOptions(this.campaignId(), own.characterId).then(
      (options) => new Set(options.spells.map((s) => s.spellKey)),
      (err: unknown) => {
        this.summonSpells = null;
        throw err;
      },
    );
    return this.summonSpells.then(
      (keys) => keys.has(key),
      () => false,
    );
  }

  /** Conjurar Animais in a combat: the choices of the creatures and the group's one initiative roll (E9-12). */
  private summonSheet(key: string, name: string): void {
    const e = this.encounter();
    const own = this.own();
    if (!e || !own) {
      return;
    }
    const data: SummonSheetData = {
      campaignId: this.campaignId(),
      characterId: own.characterId,
      spellKey: key,
      combat: {
        encounterId: e.id,
        casterId: own.id,
        diceMode: this.diceMode(),
        preference: this.dicePreference(),
        state: this.state(),
        concentrating: own.concentrationSpellNamePt,
      },
    };
    openSheet<SummonSheet, SummonSheetData, unknown>(this.dialog, this.bottomSheet, SummonSheet, {
      data,
      ariaLabel: `Conjurar ${name}`,
      labelledBy: 'summon-t',
      width: '560px',
    }).subscribe();
  }

  /** "Condições…" in a combatant's menu (the master). */
  protected openConditions(combatantId: string): void {
    const e = this.encounter();
    const combatant = e?.combatants.find((c) => c.id === combatantId);
    if (!e || !combatant) {
      return;
    }
    const data: ConditionsData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      combatant,
      state: this.state(),
    };
    openSheet<ConditionsDialog, ConditionsData, boolean>(
      this.dialog,
      this.bottomSheet,
      ConditionsDialog,
      {
        data,
        ariaLabel: `Condições de ${combatant.label}`,
        labelledBy: 'sheet-t',
        width: '600px',
      },
    ).subscribe();
  }

  /** "Você foi atingido: usar Escudo?": opens by itself, and has to be answered. */
  private openShield(prompt: ReactionPrompt): void {
    const e = this.encounter();
    const own = this.own();
    const vitals = this.vitals().find((v) => v.characterId === own?.characterId);
    if (!e) {
      return;
    }
    const data: ShieldSheetData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      prompt,
      round: e.round,
      usage: vitals?.spellSlots ?? [],
      pact: vitals?.pactSlots ?? null,
      state: this.state(),
      armorClass: this.armorClass(),
    };
    openSheet<ShieldSheet, ShieldSheetData, boolean>(this.dialog, this.bottomSheet, ShieldSheet, {
      data,
      ariaLabel: 'Você foi atingido: usar Escudo Arcano?',
      alert: true,
    }).subscribe();
  }

  /** A damage was settled: the master's card stays for its note. */
  protected settled(): void {
    this.settledTurn.set(this.subject()?.id ?? '');
  }

  /** "Dano/Cura" on an NPC (`AdjustCombatantHitPoints`). */
  protected adjustNpc(combatantId: string): void {
    const e = this.encounter();
    const combatant = e?.combatants.find((c) => c.id === combatantId);
    if (!e || !combatant) {
      return;
    }
    openSheet<AdjustNpc, AdjustNpcData, boolean>(this.dialog, this.bottomSheet, AdjustNpc, {
      data: { campaignId: this.campaignId(), encounterId: e.id, combatant, state: this.state() },
      ariaLabel: `Dano ou cura em ${combatant.label}`,
      labelledBy: 'adjust-npc-title',
      width: '440px',
    }).subscribe();
  }

  protected endCombat(): Promise<boolean> {
    return this.run((e) => this.api.end(this.campaignId(), e.id));
  }

  protected submitFace(combatantId: string, face: number): Promise<boolean> {
    return this.run((e) =>
      this.api.submitInitiative(this.campaignId(), e.id, combatantId, { face }),
    );
  }

  protected rollInApp(combatantId: string): Promise<boolean> {
    return this.run((e) =>
      this.api.submitInitiative(this.campaignId(), e.id, combatantId, { inApp: true }),
    );
  }

  protected setOrder(ids: string[]): Promise<boolean> {
    return this.run((e) => this.api.setOrder(this.campaignId(), e.id, ids));
  }

  protected reveal(change: { id: string; hidden: boolean }): Promise<boolean> {
    return this.run((e) => this.api.setHidden(this.campaignId(), e.id, change.id, change.hidden));
  }

  /** The master: "Perdeu a concentração" (the app does not roll the save; the damage card reminds the DC). */
  protected loseConcentration(id: string): Promise<boolean> {
    return this.run((e) =>
      this.api.setConditions(this.campaignId(), e.id, id, { endConcentration: true }),
    );
  }

  protected remove(id: string): Promise<boolean> {
    return this.run((e) => this.api.remove(this.campaignId(), e.id, id));
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

  /** The player's prompt: "O Goblin 2 está saindo do seu alcance. Ataque de oportunidade?" It reads the
   * reactor's attacks itself (its buttons are off until they are in), so an offer is never stuck on a failed read. */
  private openOpportunity(offer: OpportunityOffer): void {
    const e = this.encounter();
    if (!e) {
      return;
    }
    const data: OpportunitySheetData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      offer,
      round: e.round,
      load: async () => {
        const options = await this.api.turnOptions(this.campaignId(), e.id, offer.reactorId);
        return {
          options,
          attacks: reactorAttacks(
            offer,
            (options.options?.attacks ?? []).flatMap((a) => (a.attack ? [a.attack] : [])),
          ),
        };
      },
      state: this.state(),
    };
    openSheet<OpportunitySheet, OpportunitySheetData, OpportunityAnswer>(
      this.dialog,
      this.bottomSheet,
      OpportunitySheet,
      {
        data,
        ariaLabel: 'Ataque de oportunidade',
        alert: true,
      },
    ).subscribe((answer) => {
      if (answer) {
        this.opportunityAttack(offer, answer.attackKey, answer.options, false);
      }
    });
  }

  /** The offer is still pending after the attack sheet closed (nobody rolled): ask again, it must be answered. */
  private rearm(offer: OpportunityOffer): void {
    const still =
      this.encounter()?.opportunityOffers.some((o) => o.id === offer.id && o.forYou) ?? false;
    if (still && !this.isMaster()) {
      untracked(() => this.openOpportunity(offer));
    }
  }

  /** "Atacar com <arma>": the attack sheet, with the mover as its target. */
  private opportunityAttack(
    offer: OpportunityOffer,
    key: string,
    options: GetTurnOptionsResponse | null | undefined,
    byMaster: boolean,
  ): void {
    const e = this.encounter();
    const attack = options?.options?.attacks.find((a) => a.attack?.key === key)?.attack;
    if (!e || !attack) {
      return;
    }
    const data: AttackSheetData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      attackerId: offer.reactorId,
      round: e.round,
      attack,
      targets: [],
      diceMode: this.diceMode(),
      preference: this.dicePreference(),
      state: this.state(),
      asReaction: true,
      opportunity: {
        offerId: offer.id,
        targetId: offer.moverId,
        targetLabel: offer.moverLabel,
        byMaster,
      },
    };
    openSheet<AttackSheet, AttackSheetData, boolean>(this.dialog, this.bottomSheet, AttackSheet, {
      data,
      ariaLabel: `Ataque de oportunidade com ${attack.namePt || attack.name}`,
      labelledBy: 'sheet-t',
    }).subscribe(() => this.rearm(offer));
  }

  /** The master answers for an NPC: "Não atacar", or "Atacar com <arma>". */
  protected answerOpportunity(a: MasterAnswer): Promise<boolean> | void {
    if (!a.attack) {
      return this.run((e) => this.api.declineOpportunity(this.campaignId(), e.id, a.offer.id));
    }
    this.opportunityAttack(a.offer, a.attack.key, this.reactorOptions().get(a.offer.id), true);
  }

  /** "Desfazer o movimento": the master takes back his last action when it is the move that made the offers. */
  protected undoMove(): Promise<boolean> {
    const entry = this.log.undoable();
    return entry
      ? this.run((e) => this.api.undo(this.campaignId(), e.id, entry.id))
      : Promise.resolve(false);
  }

  /** "Seguir sem esperar": the master passes over an offer the player does not answer. */
  protected skipOpportunity(offer: OpportunityOffer): Promise<boolean> {
    return this.run((e) => this.api.skipOpportunity(this.campaignId(), e.id, offer.id));
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
    const ok = await this.run((current) =>
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
    return this.run((e) => this.api.withdrawOpportunity(this.campaignId(), e.id, offer.id));
  }

  /** The master's "Marcar como aliado" / "Voltar a ser inimigo". */
  protected setSide(change: { id: string; side: CombatantSide }): Promise<boolean> {
    return this.run((e) => this.api.setSide(this.campaignId(), e.id, change.id, change.side));
  }

  /** The master's cover mark, applied at once. */
  protected setCover(change: { id: string; cover: CoverDegree }): Promise<boolean> {
    return this.run((e) => this.api.setCover(this.campaignId(), e.id, change.id, change.cover));
  }

  // ---- moving ----

  /** A token dropped on a square (the master's drag, or a player's inside
   * their reach). The token is where it was dropped at once; if the server
   * refuses, it goes back and says why. */
  protected async drop(drop: TokenDrop): Promise<void> {
    const e = this.encounter();
    const c = e?.combatants.find((x) => x.id === drop.id);
    if (!e || !c) {
      return;
    }
    const from: Square = { col: c.col, row: c.row };
    if (c.placed && from.col === drop.col && from.row === drop.row) {
      return;
    }
    // A player's drop picks the square on the "Mover" page: the same warnings, the same
    // question about a trap, the same confirm. It never moves by itself.
    if (!this.isMaster()) {
      this.moveError.set('');
      this.dropStart.set({ col: drop.col, row: drop.row });
      this.state().moving.set(true);
      return;
    }
    this.error.set('');
    // The box is for this drag only; a refusal gives it back.
    const forced = this.forcedMove();
    this.forcedMove.set(false);
    this.forcedNote.set('');
    this.state().applyMove({ encounterId: e.id, combatantId: c.id, col: drop.col, row: drop.row });
    // One save at a time per combatant: `MoveSaves` is about positions, and
    // here a square's column and row stand in for x and y.
    await this.moves.move(
      `${e.id}/${c.id}`,
      { xBp: from.col, yBp: from.row },
      { xBp: drop.col, yBp: drop.row },
      {
        // The server decides every move (reach, walls, creatures): a refusal puts the token back.
        save: async (to) => {
          const res = await this.api.move(
            this.campaignId(),
            e.id,
            c.id,
            to.xBp,
            to.yBp,
            undefined,
            undefined,
            forced,
          );
          this.state().apply(res.encounter);
          if (forced) {
            this.forcedNote.set(
              `${c.label} foi mov${article(c.label) === 'a' ? 'ida' : 'ido'} à força. Ninguém recebeu oferta de ataque de oportunidade.`,
            );
          }
          if (res.lockedDoor) {
            // The master walks through closed doors but a locked one stops them too (RN-26).
            this.error.set(
              'Uma porta trancada parou o movimento. Destranque a porta (toque nela no mapa) e mova de novo.',
            );
          } else if (res.stoppedEarly) {
            // A barred or secret door stops it too: the token stands where it stopped.
            this.error.set(
              'Uma grade levadiça ou uma porta secreta parou o movimento. Abra a porta (toque nela no mapa) e mova de novo.',
            );
          }
        },
        failed: (saved, err) => {
          if (forced) {
            this.forcedMove.set(true);
          }
          this.state().applyMove({
            encounterId: e.id,
            combatantId: c.id,
            col: saved.xBp,
            row: saved.yBp,
          });
          this.error.set(combatErrorMessage(err, 'mover o token'));
          void this.refreshAfter(err);
        },
      },
    );
  }

  /** "Mover para cá" on the "Mover" page. */
  protected async confirmMove(to: Square): Promise<void> {
    const own = this.mover();
    if (!own) {
      return;
    }
    this.moveError.set('');
    const ok = await this.runMove((e) =>
      this.api.move(this.campaignId(), e.id, own.id, to.col, to.row),
    );
    this.afterMove(ok);
  }

  /** "Saltar para cá" or "Saltar 1,8 m para cima": the same call, as a jump. The new movement
   * used, less what was used before, is what the jump cost: the server's own numbers. */
  protected async confirmJump(req: JumpRequest): Promise<void> {
    const own = this.mover();
    if (!own) {
      return;
    }
    const before = own.movementUsedDft;
    this.moveError.set('');
    const ok = await this.runMove((e) => {
      // The same jump again (a lost answer) is a retry: it keeps its key and is not charged twice.
      const key = this.jumpKey.keyFor([e.id, own.id, req]);
      return req.kind === 'long'
        ? this.api.move(
            this.campaignId(),
            e.id,
            own.id,
            req.square.col,
            req.square.row,
            { kind: 'long' },
            key,
          )
        : this.api.move(
            this.campaignId(),
            e.id,
            own.id,
            own.col,
            own.row,
            { kind: 'high', heightDft: req.heightDft },
            key,
          );
    });
    if (ok) {
      this.jumpKey.renew();
    }
    // A jump that stopped short already said so (runMove's note): that is not "saltou".
    if (ok && !this.moveNote()) {
      const spent = (this.mover()?.movementUsedDft ?? before) - before;
      this.moveNote.set(
        req.kind === 'long'
          ? `Você saltou ${metersFixed(spent / 10)}.`
          : `Você saltou ${metersFixed(spent / 10)} para cima.`,
      );
    }
    this.afterMove(ok);
  }

  private afterMove(ok: boolean): void {
    if (ok && this.lockedDoor()) {
      // A locked door stopped the move: the page stays, with the notice (E10-05 9). The token already stands where it stopped.
      return;
    }
    if (ok) {
      this.state().moving.set(false);
    } else {
      // The refusal's own words, next to the map where the person looks.
      this.moveError.set(this.error());
      this.error.set('');
    }
  }

  /** Runs a move: applies the combat it answers, and says when it stopped short. */
  private async runMove(call: (e: Encounter) => Promise<MoveResult>): Promise<boolean> {
    const e = this.encounter();
    if (!e || this.busy()) {
      return false;
    }
    this.busy.set(true);
    this.error.set('');
    this.moveNote.set('');
    this.lockedDoor.set(false);
    try {
      const res = await call(e);
      this.state().apply(res.encounter);
      if (res.lockedDoor) {
        this.lockedDoor.set(true);
      } else if (res.stoppedEarly) {
        // Never what stopped it: the player did not see it.
        this.moveNote.set('Você parou antes: algo bloqueou o caminho.');
      }
      return true;
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'mover'));
      await this.refreshAfter(err);
      return false;
    } finally {
      this.busy.set(false);
    }
  }

  /** "Desengajar (gasta a ação)" on the "Mover" page: the warning goes once it is taken. */
  protected disengage(): Promise<boolean> {
    const who = this.mover();
    return who ? this.takeActionFor(who.id, 'standard:disengage') : Promise.resolve(false);
  }

  protected closeFullPage(): void {
    this.lockedDoor.set(false);
    this.moverId.set('');
    this.dropStart.set(null);
    this.state().moving.set(false);
    this.state().mapOpen.set(false);
  }

  /** "Gastar movimento" (a combat without a map): the sheet that asks how far. For the player's character, or for one of their creatures. */
  protected openSpend(id?: string): void {
    const e = this.encounter();
    const who = id ?? this.own()?.id;
    if (!e || !who) {
      return;
    }
    const data: SpendSheetData = {
      campaignId: this.campaignId(),
      encounterId: e.id,
      combatantId: who,
      state: this.state(),
    };
    openSheet<SpendSheet, SpendSheetData, boolean>(this.dialog, this.bottomSheet, SpendSheet, {
      data,
      ariaLabel: 'Gastar movimento',
      labelledBy: 'sheet-t',
    }).subscribe(() => this.moveNote.set(''));
  }

  protected openMove(): void {
    if (this.theatre()) {
      this.openSpend();
      return;
    }
    this.lockedDoor.set(false);
    this.moverId.set('');
    this.dropStart.set(null);
    this.moveError.set('');
    this.state().moving.set(true);
  }

  /** "Mover o Lobo atroz 1": the same page, for the creature. */
  protected openCreatureMove(id: string): void {
    if (this.theatre()) {
      this.openSpend(id);
      return;
    }
    this.moverName = this.encounter()?.combatants.find((c) => c.id === id)?.label ?? '';
    this.moverId.set(id);
    this.lockedDoor.set(false);
    this.dropStart.set(null);
    this.moveError.set('');
    this.state().moving.set(true);
  }

  /** The first free square nearest the middle of the map: where "Colocar no
   * mapa" puts a combatant that came without a token. */
  protected async place(id: string): Promise<void> {
    const e = this.encounter();
    if (!e) {
      return;
    }
    const taken = new Set(e.combatants.filter((c) => c.placed).map((c) => `${c.col},${c.row}`));
    const middle: Square = { col: Math.floor(e.gridColumns / 2), row: Math.floor(e.gridRows / 2) };
    let best: Square | null = null;
    for (let row = 0; row < e.gridRows; row++) {
      for (let col = 0; col < e.gridColumns; col++) {
        const d = Math.max(Math.abs(col - middle.col), Math.abs(row - middle.row));
        const dBest = best
          ? Math.max(Math.abs(best.col - middle.col), Math.abs(best.row - middle.row))
          : Infinity;
        if (!taken.has(`${col},${row}`) && d < dBest) {
          best = { col, row };
        }
      }
    }
    if (best) {
      const spot = best;
      await this.run(
        async (current) =>
          (await this.api.move(this.campaignId(), current.id, id, spot.col, spot.row)).encounter,
      );
    }
  }

  protected leave(): void {
    this.state().dismissEnded();
  }
}
