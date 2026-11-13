import type { RollDie } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  ElementRef,
  Injector,
  Directive,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatUndone } from '../../../core/combat/combat-undone';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import type { FogView } from '../../../core/maps/fog-view';
import { MapsClient } from '../../../core/maps/maps-client';
import { type SearchSkills } from '../../../core/traps/trap-search';
import type { TrapBoard } from '../../../core/traps/trap-board';
import type { CombatState } from '../../../core/combat/combat-state';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { RollAnimator } from '../../../shared/roll-overlay/roll-animator';
import type { MapState } from '../../../core/maps/map-state';
import { RosterClient } from '../../../core/maps/roster-client';
import { TurnAlert } from '../../../core/combat/turn-alert';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import type { PartyMemberInfoVm, VitalsVm } from '../../../core/play/live-session.types';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { EncounterStatus } from '../../../../gen/meurpg/play/v1/combat_pb';
import { isTheatre } from '../../../core/combat/theatre';
import { CreatureOptionsState } from '../../../core/combat/creature-options-state';
import { CHARACTER_TAB, type MineTab, mineTabs } from '../../../core/combat/mine';
import { offersToAnswer } from '../../../core/combat/opportunity';
import { LayersState } from '../../../core/maps/layers-state';
import { CombatLogState } from '../../../core/combat/combat-log-state';
import { TurnOptionsState } from '../../../core/combat/turn-options-state';
import { currentCombatant, isDown, isPlayer, ownCombatant } from '../../../core/combat/combat-view';
import { acts, jointTurn, masterActor } from '../../../core/combat/joint-turn';
import { MasterAutoPass } from '../../../core/combat/auto-pass';
import { CombatCalls } from '../../../core/combat/combat-calls';
import { CombatReads } from './combat-reads';

/**
 * What the combat screen is handed (its inputs and outputs), the services it builds on and the core of what it shows: the
 * combat, who the player is in it, whose turn it is and the services' state the parts read (`CombatCalls`, `CombatReads`,
 * `MasterAutoPass`). The base of the layers that make up `CombatView` (`combat-view-derived.ts`, `combat-view-parts.ts`,
 * `combat-view.ts`), so the template and the specs keep reading every member from the one component.
 */
@Directive()
export abstract class CombatViewStateDirective {
  protected readonly api = inject(CombatClient);
  protected readonly animator = inject(RollAnimator);
  protected readonly undone = inject(CombatUndone);
  protected readonly roster = inject(RosterClient);
  protected readonly maps = inject(MapsClient);
  protected readonly catalog = inject(SpellCatalog);
  protected readonly dialog = inject(MatDialog);
  protected readonly creaturesApi = inject(CreaturesClient);
  protected readonly tableRules = inject(TableRulesClient);
  protected readonly bottomSheet = inject(MatBottomSheet);
  protected readonly turnAlert = inject(TurnAlert);
  protected readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly injector = inject(Injector);
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
  /** The table dice the player's character may add to a save or the initiative (`DerivedSheet.roll_dice`). */
  readonly rollDice = input<readonly RollDie[]>([]);
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

  /** The master is painting the sun on the map (#704): "Próximo turno" waits until "Pronto". */
  protected readonly sunPainting = signal(false);

  /** The calls that answer with the combat: one at a time, the answer applied, a refusal said (`CombatCalls`). */
  protected readonly calls = new CombatCalls({
    api: this.api,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    state: () => this.state(),
  });
  protected readonly busy = this.calls.busy;
  protected readonly error = this.calls.error;

  /** What the screen reads from the services around the combat (`CombatReads`). */
  protected readonly reads = new CombatReads({
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

  /** The master's screen passes by itself the reactions a player leaves unanswered (`MasterAutoPass`). */
  protected readonly autoPass = new MasterAutoPass({
    api: this.api,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    isMaster: () => this.isMaster(),
    state: () => this.state(),
  });
  protected readonly autoLeft = this.autoPass.windowsLeft;
  protected readonly offerLeft = this.autoPass.offersLeft;
  /** What the combatant in the spotlight can do: the player's own on their
   * turn, the master's current one (`GetTurnOptions`). */
  protected readonly turn = new TurnOptionsState();
  protected readonly log = new CombatLogState();
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
  protected readonly lookingThroughFamiliar = computed(() => !!this.own()?.familiarSightCreatureId);
  /** The player character the master plays for on its turn (C8-01), by its combatant id; `null` while he is only the master. */
  readonly actFor = signal<string | null>(null);
  /** The character the master acts for now: a player character that is on turn and on its feet. It goes with the turn. */
  protected readonly acting = computed(() => {
    const e = this.encounter();
    const id = this.actFor();
    if (!this.isMaster() || !e || e.status !== EncounterStatus.ACTIVE || !id) {
      return null;
    }
    const c = e.combatants.find((x) => x.id === id);
    return c && isPlayer(c) && acts(e, c) && !isDown(c) ? c : null;
  });
  /** The character the screen acts for: the player's own, or, for the master, the one he acts for. */
  protected readonly own = computed(() => {
    const e = this.encounter();
    if (this.isMaster()) {
      return this.acting();
    }
    return e ? ownCombatant(e) : null;
  });
  protected readonly myTurn = computed(() => {
    const e = this.encounter();
    const own = this.own();
    return !!e && !!own && acts(e, own);
  });
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
  /** The player's own hit points, from its vitals (the combatant carries them only for a beast's pool): the turn card and the order read them. */
  protected readonly ownHitPoints = computed(() => {
    const v = this.ownVitals();
    return v ? { current: v.hitPointsCurrent, max: v.hitPointsMax } : null;
  });
  /** The player's own slots, for the rows above the spell list. */
  protected readonly ownSlots = computed(() => {
    const own = this.own();
    const v = own ? this.vitals().find((x) => x.characterId === own.characterId) : undefined;
    return { usage: v?.spellSlots ?? [], pact: v?.pactSlots ?? null };
  });
  /** The map's painted layers (walls, difficult terrain, cover), read again when the map's `layers_revision` changes. */
  protected readonly layersState = new LayersState(
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
  /** The offers the caller answers, and the ones on the player's own mover. */
  protected readonly toAnswer = computed(() => {
    const e = this.encounter();
    return e ? offersToAnswer(e) : [];
  });
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
  /** The player's maximum hit points, for "com 0 de 24 pontos de vida". */
  protected readonly ownMaxHp = computed(
    () =>
      this.vitals().find((v) => v.characterId === this.own()?.characterId)?.hitPointsMax ?? null,
  );
  protected readonly ownDown = computed(() => {
    const own = this.own();
    return !!own && isDown(own);
  });
  protected readonly setup = computed(() => this.encounter()?.status === EncounterStatus.SETUP);
  protected readonly active = computed(() => this.encounter()?.status === EncounterStatus.ACTIVE);
  protected readonly ended = computed(() => this.encounter()?.status === EncounterStatus.ENDED);
  /** Changes only when the turn passes (or the combat ends): a string, so the effects that read it ignore the combat's other updates. */
  protected readonly turnKey = computed(() => {
    const e = this.encounter();
    return e && e.status === EncounterStatus.ACTIVE
      ? `${e.id}:${e.round}:${e.currentCombatantId}`
      : '';
  });
}
