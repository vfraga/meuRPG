import { computed, signal } from '@angular/core';

import type { EncounterEvaluation } from '../../../gen/meurpg/play/v1/encounters_pb';
import type { CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { MonsterGroupSpec } from '../combat/combat-client';
import { type EncountersClient, type PartyNpcSpec } from './encounters-client';
import { encounterErrorMessage } from './encounter-text';

/** One kind of creature of the draft, as the master put it: the creature and how many. */
export interface DraftEntry {
  readonly creature: CreatureSummary;
  readonly count: number;
}

/** An NPC in the party, with the name the chips show. */
export interface DraftNpc extends PartyNpcSpec {
  /** What the chip says: the NPC's name, or the name typed. */
  readonly label: string;
}

/** The server's limits (encounters.proto): one creature once, 1 to 40 of it, 20 kinds, 10 NPCs. */
export const ENTRY_MAX = 40;
export const KINDS_MAX = 20;
export const PARTY_NPC_MAX = 10;

/** How long the master's typing pauses before the encounter is measured again. */
const PAUSE_MS = 250;

/**
 * The encounter the master is putting together, and what the server says it costs (MR-043, RN-29). The browser does
 * no maths: every change asks `EvaluateEncounter` and the screen draws the answer. The asking is debounced (a run
 * of taps on "+" asks once), one request is in flight at a time (a change that arrives meanwhile asks again when it
 * ends, with the newest draft), and an answer that is no longer for the draft on screen is dropped.
 */
export class EncounterDraft {
  readonly entries = signal<readonly DraftEntry[]>([]);
  readonly npcs = signal<readonly DraftNpc[]>([]);
  /** The server's last answer for the draft on screen; `null` before the first. */
  readonly evaluation = signal<EncounterEvaluation | null>(null);
  readonly state = signal<'idle' | 'measuring' | 'error'>('idle');
  readonly error = signal('');
  /** The seed of the generated encounter this draft came from (shown small), or `null` once it is edited. */
  readonly seed = signal<number | null>(null);

  readonly creatureCount = computed(() => this.entries().reduce((n, e) => n + e.count, 0));
  readonly specs = computed<MonsterGroupSpec[]>(() =>
    this.entries().map((e) => ({ creatureKey: e.creature.key, count: e.count })),
  );
  readonly party = computed<PartyNpcSpec[]>(() =>
    this.npcs().map((n) => ({ characterId: n.characterId, name: n.name, level: n.level })),
  );

  private timer: ReturnType<typeof setTimeout> | null = null;
  /** Grows with every change: an answer is kept only if it is still for the newest one. */
  private version = 0;
  private inFlight = false;
  private again = false;

  constructor(
    private readonly api: Pick<EncountersClient, 'evaluate'>,
    private readonly campaignId: string,
  ) {}

  /** Taps "+" (or the stepper): sets how many of a creature; 0 takes it out. */
  setCount(key: string, count: number): void {
    const next = Math.min(ENTRY_MAX, Math.max(0, count));
    this.change(
      this.entries()
        .map((e) => (e.creature.key === key ? { ...e, count: next } : e))
        .filter((e) => e.count > 0),
    );
  }

  /** Adds a creature (one of it); one already there gets one more. */
  add(creature: CreatureSummary): void {
    const have = this.entries().find((e) => e.creature.key === creature.key);
    if (have) {
      this.setCount(creature.key, have.count + 1);
    } else if (this.entries().length < KINDS_MAX) {
      this.change([...this.entries(), { creature, count: 1 }]);
    }
  }

  remove(key: string): void {
    this.change(this.entries().filter((e) => e.creature.key !== key));
  }

  /** Puts a whole encounter in (a generated one, "Usar este encontro"); the seed shows small until the next edit. */
  replace(entries: readonly DraftEntry[], seed: number | null = null): void {
    this.change(entries);
    this.seed.set(seed);
  }

  addNpc(npc: DraftNpc): void {
    if (this.npcs().length < PARTY_NPC_MAX) {
      this.npcs.set([...this.npcs(), npc]);
      this.touch();
    }
  }

  /**
   * Takes out the NPC a chip of the party stands for: the chips come from the last measure, which lags the draft, so a
   * position in them is not a position in the draft. An NPC of the campaign is told by its `characterId`, one given by
   * name alone by its name. An NPC no longer in the draft (a second click on the same chip) changes nothing.
   */
  removeNpc(member: { readonly characterId: string; readonly name: string }): void {
    const at = this.npcs().findIndex((n) =>
      member.characterId !== ''
        ? n.characterId === member.characterId
        : n.characterId === '' && n.name === member.name,
    );
    if (at < 0) {
      return;
    }
    this.npcs.set(this.npcs().filter((_, i) => i !== at));
    this.touch();
  }

  /** Measures at once (the first look, a retry): no pause. */
  measureNow(): void {
    this.touch(0);
  }

  /** Stops asking: the page is gone. */
  stop(): void {
    this.version++;
    this.again = false;
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }

  private change(entries: readonly DraftEntry[]): void {
    this.entries.set(entries);
    this.seed.set(null);
    this.touch();
  }

  private touch(pause = PAUSE_MS): void {
    this.version++;
    this.state.set('measuring');
    if (this.timer) {
      clearTimeout(this.timer);
    }
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.run();
    }, pause);
  }

  private async run(): Promise<void> {
    if (this.inFlight) {
      this.again = true;
      return;
    }
    this.inFlight = true;
    const sent = this.version;
    try {
      const evaluation = await this.api.evaluate(this.campaignId, this.specs(), this.party());
      if (sent === this.version) {
        this.evaluation.set(evaluation);
        this.error.set('');
        this.state.set('idle');
      }
    } catch (err) {
      if (sent === this.version) {
        this.error.set(encounterErrorMessage(err, 'evaluate'));
        this.state.set('error');
      }
    } finally {
      this.inFlight = false;
      if (this.again) {
        this.again = false;
        void this.run();
      }
    }
  }
}
