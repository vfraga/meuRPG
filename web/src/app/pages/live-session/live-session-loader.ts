import { casterOfRevival } from '../../core/revivify/revivify-flow';
import { applySnapshot } from '../../core/play/vitals';
import type { CampaignInfoVm } from '../../core/play/live-session.types';
import type { LiveSessionParts } from './live-session-parts';
import { COMBAT_REREADS, COMBAT_RETRY_MS, mageArmorClassOf } from './live-session-pure';
import type { LiveSessionServices } from './live-session-services';
import type { LiveSessionState } from './live-session-state';

/**
 * The reads of the session page: the campaign, the snapshot, the combat, the sheet and the lists. Each answer carries the
 * generation it was asked in, and is dropped when the page has loaded another campaign since.
 */
export class LiveSessionLoader {
  constructor(
    private readonly services: LiveSessionServices,
    private readonly state: LiveSessionState,
    private readonly parts: LiveSessionParts,
    /** The stream's news the snapshot waits on: it sets the vision's read going. */
    private readonly scheduleVision: () => void,
  ) {}

  /** Starts over for a campaign: clears what the page shows, starts the campaign's read, and returns the new generation. */
  begin(campaignId: string): number {
    const { state, parts } = this;
    const generation = ++state.generation;
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
    parts.toasts.clear();
    state.loadedSheetFor = null;
    state.partyInfoIds = new Set();

    // The campaign (its name, and whether the person is its master) and the
    // stream start together: at the table, every round trip is a wait.
    state.campaignLoad = this.services.source.getCampaign(campaignId).then(
      (campaign) => this.campaignRead(campaign, generation),
      (err: unknown) => {
        if (generation === state.generation) {
          this.closeStream();
          this.fail(err);
        }
        return null;
      },
    );
    return generation;
  }

  private campaignRead(campaign: CampaignInfoVm, generation: number): CampaignInfoVm | null {
    if (generation !== this.state.generation) {
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

  async readSnapshot(campaignId: string, generation: number): Promise<void> {
    const { state, parts } = this;
    const mapEvents = state.mapEvents;
    const imageEvents = state.imageEvents;
    try {
      const [snapshot, campaign] = await Promise.all([
        this.services.source.getLiveSession(campaignId),
        state.campaignLoad,
      ]);
      if (generation !== state.generation || !campaign) {
        return;
      }
      state.session.set(snapshot.session);
      state.vitals.update((list) => applySnapshot(list, snapshot.vitals));
      // News that came while the snapshot was on its way is newer than it: the snapshot only fills in what no event told.
      const mapIsNews = mapEvents === state.mapEvents;
      if (mapIsNews) {
        state.currentMapId.set(snapshot.currentMapId);
      }
      if (imageEvents === state.imageEvents) {
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
      }
      state.phase.set('live');
      state.stream()?.confirmHealthy();
      // Here already: the notice about this session has nothing to add.
      this.services.openSessions.dismiss(snapshot.session.sessionId);
      this.loadDetails(campaignId, generation);
    } catch (err) {
      if (generation !== state.generation) {
        return;
      }
      this.snapshotFailed(err);
    }
  }

  /** Every panel that reads on a tick reads again: the creatures, the Revivify casts, the spell casts, the group check. */
  private bumpTicks(): void {
    const { state } = this;
    state.creaturesTick.update((n) => n + 1);
    state.revivifyTick.update((n) => n + 1);
    state.castsTick.update((n) => n + 1);
    state.groupCheckTick.update((n) => n + 1);
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
      if (generation === state.generation) {
        state.revivedCaster.set(casterOfRevival(log.rounds, characterId));
      }
    } catch {
      // The notice stands without the caster's name.
    }
  }

  /** The vitals again, from the snapshot, without reading the rest of the page. */
  async readVitals(campaignId: string, generation: number): Promise<void> {
    try {
      const snapshot = await this.services.source.getLiveSession(campaignId);
      if (generation === this.state.generation) {
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
        generation === state.generation &&
        !parts.combat.applyRead(ticket, encounter) &&
        parts.combat.changedSince(ticket) &&
        attempt < COMBAT_REREADS
      ) {
        void this.loadCombat(generation, attempt + 1);
      }
    } catch {
      // One more try a moment later; after that the stream's next event, or reconnection, reads it again.
      if (generation === state.generation && attempt === 0) {
        setTimeout(() => {
          if (generation === state.generation) {
            void this.loadCombat(generation, COMBAT_REREADS);
          }
        }, COMBAT_RETRY_MS);
      }
    }
  }

  /** The campaign again, to pick up the dice choice the player just saved. */
  async refreshCampaign(): Promise<void> {
    const generation = this.state.generation;
    try {
      const campaign = await this.services.source.getCampaign(this.state.campaignId());
      if (generation === this.state.generation) {
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
      if (ids.every((id) => state.partyInfoIds.has(id))) {
        return;
      }
      ids.forEach((id) => state.partyInfoIds.add(id));
      this.services.source.getPartyInfo(campaignId).then(
        (info) => generation === state.generation && state.partyInfo.set(info),
        () => undefined,
      );
      return;
    }
    const own = state.ownVitals();
    if (!own || state.loadedSheetFor === own.characterId) {
      return;
    }
    state.loadedSheetFor = own.characterId;
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
      (sheet) => generation === state.generation && state.playerSheet.set(sheet),
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
        if (generation === state.generation) {
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
    const generation = state.generation;
    // Only the latest read lands: replies come in any order, and an older one holds an older list.
    const seq = ++state.leftSeq;
    try {
      const images = await this.services.source.listLeftImages(state.campaignId());
      if (generation === state.generation && seq === state.leftSeq) {
        state.leftImages.set(images);
      }
    } catch {
      // Best effort.
    }
  }

  /** The master's select and the "Fundo de mapa escondido" tags read the list. */
  async reloadMaps(): Promise<void> {
    const generation = this.state.generation;
    try {
      const maps = await this.services.mapsApi.list(this.state.campaignId());
      if (generation === this.state.generation) {
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
