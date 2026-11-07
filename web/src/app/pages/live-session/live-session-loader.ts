import {
  CastEffect,
  type OutsideCast,
  OutsideCastStatus,
} from '../../../gen/meurpg/play/v1/casting_pb';
import { casterOfRevival } from '../../core/revivify/revivify-flow';
import { applySnapshot } from '../../core/play/vitals';
import type { CampaignInfoVm, ShownImageVm } from '../../core/play/live-session.types';
import type { LiveSessionParts, LiveSessionServices } from './live-session-deps';
import type { LiveSessionState } from './live-session-state';

const MAGE_ARMOR_KEY = 'spell:mage-armor';

/** How many times the combat is read again when a read was dropped for a change that came while it was out. */
const COMBAT_REREADS = 3;

/** How long the page waits before trying a failed combat read once more, in milliseconds. */
const COMBAT_RETRY_MS = 1500;

/** How long a burst of vision hints waits to become one read, in milliseconds. */
const VISION_DEBOUNCE_MS = 120;

/** The armor class the character's lasting Armadura Arcana gives, the best one; 0 without it. */
export function mageArmorClassOf(casts: readonly OutsideCast[], characterId: string): number {
  let best = 0;
  for (const cast of casts) {
    if (cast.spellKey !== MAGE_ARMOR_KEY || cast.status !== OutsideCastStatus.ACTIVE) {
      continue;
    }
    for (const t of cast.targets) {
      if (t.characterId === characterId && t.effect === CastEffect.ARMOR_CLASS) {
        best = Math.max(best, t.armorClass);
      }
    }
  }
  return best;
}

/** How many of the stream's news of the current map and of the shown image have come: a snapshot that began before one
 * of them does not overwrite it. */
export interface NewsCounts {
  readonly map: number;
  readonly image: number;
}

/**
 * The reads of the session page: the campaign, the snapshot, the combat, the sheet and the lists. Each answer carries the
 * generation it was asked in, and is dropped when the page has loaded another campaign since.
 */
export class LiveSessionLoader {
  /** The campaign of the current load; `null` once it turned out the page can't show it (no access, a pending member, an error). */
  private campaignLoad: Promise<CampaignInfoVm | null> = Promise.resolve(null);
  private loadedSheetFor: string | null = null;
  private partyInfoIds = new Set<string>();
  /** Moves with each load: an answer that comes for an older one is dropped. */
  private generation = 0;
  /** Moves with each read of the images left with the players, and with each one taken back here. */
  private leftSeq = 0;
  private visionTimer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private readonly services: LiveSessionServices,
    private readonly state: LiveSessionState,
    private readonly parts: LiveSessionParts,
  ) {}

  /** Stops what the page left waiting: the stream and the vision's debounce. */
  dispose(): void {
    clearTimeout(this.visionTimer);
    this.closeStream();
  }

  /** What a player sees may have changed (a hint, a move, a turn): one read of the vision and its layers for a burst of them. The master's "Ver como" and his light warning read again too. */
  scheduleVision(): void {
    if (this.visionTimer !== undefined) {
      return;
    }
    this.visionTimer = setTimeout(() => {
      this.visionTimer = undefined;
      void this.parts.fog.refresh();
      // This hint is the only trigger for a Darkness appearing or ending: neither changes the map or its tokens.
      // Opening the page and a reconnection read here too.
      // The watch folds a burst into one read after the one in the air.
      void this.parts.dark.refresh();
      this.state.visionTick.update((n) => n + 1);
    }, VISION_DEBOUNCE_MS);
  }

  /** The master took an image back: it leaves the list now, and a read that began before this answer is dropped
   * (it may still carry the image). */
  takenBack(image: ShownImageVm): void {
    this.leftSeq++;
    this.state.leftImages.update((list) => list.filter((i) => i.id !== image.id));
  }

  /** "Voltar aos seus olhos" worked: the band goes at once; the stream's newer vitals confirm it. */
  familiarStopped(characterId: string): void {
    this.state.vitals.update((list) =>
      list.map((v) => (v.characterId === characterId ? { ...v, familiarSight: null } : v)),
    );
    void this.parts.mapState.refresh();
    this.scheduleVision();
  }

  /** Starts over for a campaign: clears what the page shows, starts the campaign's read, and returns the new generation. */
  begin(campaignId: string): number {
    const { state, parts } = this;
    const generation = ++this.generation;
    this.closeStream();
    state.campaignId.set(campaignId);
    state.phase.set('loading');
    state.campaign.set(null);
    state.session.set(null);
    state.vitals.set([]);
    state.playerSheet.set(null);
    state.mageArmorAc.set(0);
    state.partyInfo.set(new Map());
    state.currentMapId.set(null);
    state.shownImage.set(null);
    state.shownKeep.set(false);
    state.leftImages.set([]);
    state.shownNotice.set('');
    state.campaignMaps.set([]);
    void parts.mapState.open(null);
    parts.combat.clear();
    state.highlightsClosed.set(null);
    parts.scene.clear();
    parts.notes.clear();
    parts.trapBoard.clear();
    parts.puzzles.clear();
    parts.freeDice.clear();
    parts.toasts.clear();
    this.loadedSheetFor = null;
    this.partyInfoIds = new Set();

    // The campaign (its name, and whether the person is its master) and the
    // stream start together: at the table, every round trip is a wait.
    this.campaignLoad = this.services.source.getCampaign(campaignId).then(
      (campaign) => this.campaignRead(campaign, generation),
      (err: unknown) => {
        if (generation === this.generation) {
          this.closeStream();
          this.fail(err);
        }
        return null;
      },
    );
    return generation;
  }

  private campaignRead(campaign: CampaignInfoVm, generation: number): CampaignInfoVm | null {
    if (generation !== this.generation) {
      return null;
    }
    if (campaign.awaitingApproval) {
      // A pending member isn't a member yet (RN-15): same page as a
      // stranger, without the campaign's name.
      this.closeStream();
      this.state.phase.set('no-access');
      return null;
    }
    this.state.campaign.set(campaign);
    return campaign;
  }

  closeStream(): void {
    this.state.stream()?.stop();
    this.state.stream.set(null);
  }

  async readSnapshot(
    campaignId: string,
    generation: number,
    news: () => NewsCounts,
  ): Promise<void> {
    const { state, parts } = this;
    const before = news();
    try {
      const [snapshot, campaign] = await Promise.all([
        this.services.source.getLiveSession(campaignId),
        this.campaignLoad,
      ]);
      if (generation !== this.generation || !campaign) {
        return;
      }
      // Another session opened while the stream was down: its rolls start empty.
      if (state.session() && state.session()?.sessionId !== snapshot.session.sessionId) {
        parts.freeDice.clear();
      }
      state.session.set(snapshot.session);
      void parts.freeDice.refresh();
      state.vitals.update((list) => applySnapshot(list, snapshot.vitals));
      // News that came while the snapshot was on its way is newer than it: the snapshot only fills in what no event told.
      const after = news();
      const mapIsNews = before.map === after.map;
      if (mapIsNews) {
        state.currentMapId.set(snapshot.currentMapId);
      }
      if (before.image === after.image) {
        state.shownImage.set(snapshot.shownImage);
        state.shownKeep.set(snapshot.shownImageKeep);
      }
      void this.reloadLeftImages();
      void this.loadCombat(generation);
      void parts.scene.refresh();
      void parts.puzzles.refresh();
      if (!state.isMaster()) {
        // A clue that arrived while the stream was down counts as new too.
        void parts.notes.refresh(true);
      }
      parts.combat.touchLog(); // the log is read again too, after a reconnection
      // Each `ready` (a reconnection too) reads the map again: a missed event never leaves it stale.
      if (mapIsNews) {
        void parts.mapState.open(snapshot.currentMapId);
      }
      // What a player sees on a fog map, and the creatures, follow the server's current state too.
      this.scheduleVision();
      this.bumpTicks();
      if (state.isMaster()) {
        void this.reloadMaps();
        // After the campaign is read: only then is it known the person is the master, and the bar's button needs the TV's state.
        void parts.tableScreen.load();
      }
      state.phase.set('live');
      state.stream()?.confirmHealthy();
      // Here already: the notice about this session has nothing to add.
      this.services.openSessions.dismiss(snapshot.session.sessionId);
      this.loadDetails(campaignId, generation);
    } catch (err) {
      if (generation !== this.generation) {
        return;
      }
      this.snapshotFailed(err);
    }
  }

  /** Every panel that reads on a tick reads again: the creatures, the Revivify casts, the spell casts, the group check, the held rolls' questions. */
  private bumpTicks(): void {
    const { state } = this;
    state.creaturesTick.update((n) => n + 1);
    state.revivifyTick.update((n) => n + 1);
    state.castsTick.update((n) => n + 1);
    state.groupCheckTick.update((n) => n + 1);
    state.heldRollsTick.update((n) => n + 1);
  }

  /** The player's own character lives again: the notice, with who cast it when the combat's log says (the master's "Reviver" has no caster). */
  async characterRevived(
    campaignId: string,
    generation: number,
    characterId: string,
  ): Promise<void> {
    const { state, parts } = this;
    if (state.isMaster()) {
      return;
    }
    state.revivedCaster.set('');
    state.revivedId.set(characterId);
    const e = parts.combat.encounter();
    if (!e) {
      return;
    }
    try {
      const log = await this.services.combatApi.turn.log(campaignId, e.id);
      if (generation === this.generation) {
        state.revivedCaster.set(casterOfRevival(log.rounds, characterId));
      }
    } catch {
      // The notice stands without the caster's name.
    }
  }

  /** The vitals on the page again, for a screen that found them changed. */
  rereadVitals(): Promise<void> {
    return this.readVitals(this.state.campaignId(), this.generation);
  }

  /** The vitals again, from the snapshot, without reading the rest of the page. */
  async readVitals(campaignId: string, generation: number): Promise<void> {
    try {
      const snapshot = await this.services.source.getLiveSession(campaignId);
      if (generation === this.generation) {
        this.state.vitals.update((list) => applySnapshot(list, snapshot.vitals));
      }
    } catch {
      // Keep what is on screen: the vitals event of the character, or the next connection, brings it.
    }
  }

  /** What a snapshot that could not be read means: some answers end the page, only the others are worth asking again. */
  private snapshotFailed(err: unknown): void {
    switch (this.services.source.classifyError(err)) {
      case 'no-session':
        this.ended();
        return;
      case 'no-access':
      case 'forbidden':
        // The server will not give this person the session, however many times it is asked.
        this.closeStream();
        this.state.phase.set('no-access');
        return;
      case 'signed-out':
        this.closeStream();
        this.services.auth.signIn(this.services.router.url);
        return;
      case 'invalid':
        // The request itself is wrong: asking again changes nothing, "Tentar de novo" starts over.
        this.closeStream();
        this.state.phase.set('error');
        return;
      default:
        // Try the whole thing again: the next `ready` reads a new one.
        this.state.stream()?.restart();
    }
  }

  /** The session's combat, as this person may see it (best effort: the
   * screen keeps the copy it has until the next event or `ready`). */
  async loadCombat(generation: number, attempt = 0): Promise<void> {
    const { state, parts } = this;
    const ticket = parts.combat.beginRead();
    try {
      const encounter = await this.services.combatApi.turn.get(state.campaignId());
      // Dropped for something that replaced the combat while it was out (an event, an answer to a call): that may be
      // older than what this read was asked for, so read again. Bounded: a busy table never loops.
      if (
        generation === this.generation &&
        !parts.combat.applyRead(ticket, encounter) &&
        parts.combat.changedSince(ticket) &&
        attempt < COMBAT_REREADS
      ) {
        void this.loadCombat(generation, attempt + 1);
      }
    } catch {
      // One more try a moment later; after that the stream's next event, or reconnection, reads it again.
      if (generation === this.generation && attempt === 0) {
        setTimeout(() => {
          if (generation === this.generation) {
            void this.loadCombat(generation, COMBAT_REREADS);
          }
        }, COMBAT_RETRY_MS);
      }
    }
  }

  /** The campaign again, to pick up the dice choice the player just saved. */
  async refreshCampaign(): Promise<void> {
    const generation = this.generation;
    try {
      const campaign = await this.services.source.getCampaign(this.state.campaignId());
      if (generation === this.generation) {
        this.state.campaign.set(campaign);
      }
    } catch {
      // Best effort: the screen keeps the choice it had.
    }
  }

  /** What the vitals come with: the player's CA and class line, or the
   * master's class and player names per character. Best effort: the live
   * numbers don't wait for these. */
  private loadDetails(campaignId: string, generation: number): void {
    const { state } = this;
    if (state.isMaster()) {
      const ids = state.vitals().map((v) => v.characterId);
      if (ids.every((id) => this.partyInfoIds.has(id))) {
        return;
      }
      ids.forEach((id) => this.partyInfoIds.add(id));
      this.services.source.getPartyInfo(campaignId).then(
        (info) => generation === this.generation && state.partyInfo.set(info),
        () => undefined,
      );
      return;
    }
    const own = state.ownVitals();
    if (!own || this.loadedSheetFor === own.characterId) {
      return;
    }
    this.loadedSheetFor = own.characterId;
    this.loadPlayerSheet(campaignId, generation);
  }

  /** Reads the player's own sheet again; best effort, the page keeps the one it has when this fails. */
  loadPlayerSheet(campaignId: string, generation: number): void {
    const { state } = this;
    const own = state.ownVitals();
    if (state.isMaster() || !own) {
      return;
    }
    this.services.source.getPlayerSheet(campaignId, own.characterId).then(
      (sheet) => generation === this.generation && state.playerSheet.set(sheet),
      () => undefined,
    );
    this.loadMageArmor(campaignId, generation);
  }

  /** Reads the Armadura Arcana that lasts on the player's character: every place that shows the armor class must agree with the combat's. Best effort. */
  loadMageArmor(campaignId: string, generation: number): void {
    const { state } = this;
    const own = state.ownVitals();
    if (state.isMaster() || !own) {
      return;
    }
    this.services.castingApi.list(campaignId, own.characterId).then(
      (res) => {
        if (generation === this.generation) {
          state.mageArmorAc.set(mageArmorClassOf(res.active, own.characterId));
        }
      },
      () => undefined,
    );
  }

  /** The images left with the players, read again (best effort: the list
   * keeps what it had). */
  async reloadLeftImages(): Promise<void> {
    const { state } = this;
    const generation = this.generation;
    // Only the latest read lands: replies come in any order, and an older one holds an older list.
    const seq = ++this.leftSeq;
    try {
      const images = await this.services.source.listLeftImages(state.campaignId());
      if (generation === this.generation && seq === this.leftSeq) {
        state.leftImages.set(images);
      }
    } catch {
      // Best effort.
    }
  }

  /** The master's select and the "Fundo de mapa escondido" tags read the list. */
  async reloadMaps(): Promise<void> {
    const generation = this.generation;
    try {
      const maps = await this.services.mapsApi.list(this.state.campaignId());
      if (generation === this.generation) {
        this.state.campaignMaps.set(maps);
      }
    } catch {
      // Best effort: the select keeps what it had.
    }
  }

  ended(): void {
    this.closeStream();
    // Without a snapshot, there was never a session on this page.
    this.state.phase.set(this.state.session() ? 'ended' : 'no-session');
    void this.services.openSessions.refresh();
  }

  private fail(err: unknown): void {
    switch (this.services.source.classifyError(err)) {
      case 'no-access':
        this.state.phase.set('no-access');
        return;
      case 'signed-out':
        this.services.auth.signIn(this.services.router.url);
        return;
      default:
        this.state.phase.set('error');
    }
  }
}
