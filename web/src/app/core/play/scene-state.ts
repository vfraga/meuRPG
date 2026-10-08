import { computed, signal } from '@angular/core';

import type { SceneClue } from '../../../gen/meurpg/maps/v1/maps_pb';
import type { OpenSceneInfo, StageNpc } from '../../../gen/meurpg/play/v1/scene_pb';
import { attemptsAnnouncement, rollAnnouncement } from './scene-view';
import { stageAnnouncement } from './stage-view';

/**
 * The scene open in the session, as this person sees it (MR-015): a signal
 * the master's blocks and the player's block read, and the words a live
 * region says when it changes. Pure TypeScript, so the rules are tested
 * without a DOM:
 *
 * - the first read says nothing (the page was just opened);
 * - a player hears "O mestre abriu uma cena: …" when one opens, and "O mestre
 *   fechou a cena." when it closes;
 * - the master hears each new roll ("Toren: Seguir os rastros dos goblins, 7,
 *   não passou");
 * - a player hears when the master gives another attempt ("O mestre deu mais
 *   uma tentativa em Resistir ao cheiro de fumaça.", MR-015);
 * - a player also hears the stage move ("Mira entrou na cena.", "Aldo fala.",
 *   MR-031);
 * - a stale answer never overwrites a newer one.
 */
export class SceneState {
  readonly scene = signal<OpenSceneInfo | null>(null);
  /** The NPCs on the scene's stage, in the order they came in (empty with no
   * scene). Part of the scene: the server clears it when the scene closes. */
  readonly stage = computed<readonly StageNpc[]>(() => this.scene()?.stage ?? []);
  /** What the live region says now; changes with each news. */
  readonly notice = signal('');
  /**
   * Where focus goes next, because the master's own action replaced what had it
   * (E7-02): "title" after a scene opens or is swapped (the open scene's title),
   * "open" after it closes (the "Abrir cena" button of the panel that comes
   * back). Whoever is drawn for it takes the focus and sets this back to
   * `null`; a scene that was already open when the page loaded never sets it.
   */
  readonly focusNext = signal<'title' | 'open' | null>(null);

  private generation = 0;
  /** Moves with each local answer: a read that began before it may lack what the answer carries (or carry less
   * than the answer), so it is dropped and the scene is read once more, not lost. */
  private edits = 0;
  private loaded = false;

  constructor(
    private readonly load: () => Promise<OpenSceneInfo | null>,
    private readonly isMaster: () => boolean,
  ) {}

  /** Reads the scene again (`ready`, `scene_changed`, `scene_check_rolled`).
   * A failed read keeps the copy on screen: the next event reads it again. */
  async refresh(): Promise<void> {
    const generation = ++this.generation;
    const edits = this.edits;
    try {
      const next = await this.load();
      if (generation !== this.generation) {
        return;
      }
      if (edits !== this.edits) {
        return await this.refresh();
      }
      this.show(next);
    } catch {
      // The stream's next event, or reconnection, reads it again.
    }
  }

  /** The scene as an answer of the master's own call (open, swap, close). */
  apply(next: OpenSceneInfo | null): void {
    this.edits++;
    this.show(next);
  }

  private show(next: OpenSceneInfo | null): void {
    const prev = this.scene();
    this.scene.set(next);
    if (this.loaded) {
      this.say(prev, next);
    }
    this.loaded = true;
  }

  /** A clue the master just revealed here: the open scene carries it with the
   * players who have it now (the stream's `scene_changed` reads the same). */
  clueRevealed(clue: SceneClue): void {
    const scene = this.scene();
    if (!scene) {
      return;
    }
    this.edits++;
    this.scene.set({ ...scene, clues: scene.clues.map((c) => (c.id === clue.id ? clue : c)) });
  }

  /** The master's own stage call answered: the stage as it is now. Applied in
   * place, so the cards change at once; a read still on its way is dropped and made again. */
  setStage(stage: readonly StageNpc[]): void {
    const scene = this.scene();
    if (!scene) {
      return;
    }
    this.edits++;
    this.scene.set({ ...scene, stage: [...stage] });
  }

  /** The master opened (or swapped) the scene here: the title takes focus. */
  openedHere(next: OpenSceneInfo): void {
    this.apply(next);
    this.focusNext.set('title');
  }

  /** The master closed the scene here: "Abrir cena" takes focus. */
  closedHere(): void {
    this.apply(null);
    this.focusNext.set('open');
  }

  /** A new session or page: nothing is open, and the next read is silent. */
  clear(): void {
    this.generation++;
    this.scene.set(null);
    this.notice.set('');
    this.focusNext.set(null);
    this.loaded = false;
  }

  private say(prev: OpenSceneInfo | null, next: OpenSceneInfo | null): void {
    if (this.isMaster()) {
      if (next && prev && sameScene(prev, next)) {
        const known = new Set(prev.rolls.map((r) => r.id));
        const fresh = next.rolls.filter((r) => !known.has(r.id)).reverse();
        if (fresh.length > 0) {
          this.notice.set(fresh.map((r) => rollAnnouncement(next, r)).join('. '));
        }
      }
      return;
    }
    if (next && (!prev || !sameScene(prev, next))) {
      this.notice.set(`O mestre abriu uma cena: ${next.name}.`);
    } else if (!next && prev) {
      this.notice.set('O mestre fechou a cena.');
    } else if (next && prev) {
      const stage = stageAnnouncement(prev.stage, next.stage);
      const attempts = attemptsAnnouncement(prev, next);
      if (attempts) {
        this.notice.set(attempts);
      } else if (stage) {
        this.notice.set(stage);
      }
    }
  }
}

/** The same opening of the same scene (closing and opening it again is another). */
function sameScene(a: OpenSceneInfo, b: OpenSceneInfo): boolean {
  return (
    a.pointId === b.pointId &&
    a.openedAt?.seconds === b.openedAt?.seconds &&
    a.openedAt?.nanos === b.openedAt?.nanos
  );
}
