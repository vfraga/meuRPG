import { signal } from '@angular/core';

import type { CharacterPreviewVm } from './character-editor.types';

/** How long the draft rests before the server is asked: a run of keystrokes asks once. */
const PAUSE_MS = 300;

/**
 * What the server derives for the draft on screen (`PreviewCharacter`): the hit points and the spell numbers of each
 * casting class. The browser has no rules engine, so the "Pontos de vida" box adds the server's
 * `hit_points_from_effects` (Dwarven Toughness, a table's own effect) to the rows it writes itself, and the "Magias"
 * step counts its picks against the server's numbers. Each change of the draft asks once, after a pause; only the newest
 * answer counts (a slow one overtaken by a newer one is dropped); while an answer is on the way the last one
 * stays; a draft the server cannot take yet (no race or class) asks nothing and shows none, and a call that
 * fails shows none, so the box falls back to its own arithmetic.
 */
export class ServerHitPoints {
  /** The server's answer for the newest draft that got one; `null` when there is none to show. */
  readonly answer = signal<CharacterPreviewVm | null>(null);

  private timer: ReturnType<typeof setTimeout> | null = null;
  /** Grows with every change: an answer is kept only if it is still for the newest one. */
  private version = 0;

  constructor(private readonly ask: () => Promise<CharacterPreviewVm>) {}

  /** The draft changed. `ready` is false while it lacks what the server needs (a race and a class). */
  draftChanged(ready: boolean): void {
    const mine = ++this.version;
    this.clearTimer();
    if (!ready) {
      this.answer.set(null);
      return;
    }
    this.timer = setTimeout(() => {
      this.timer = null;
      Promise.resolve()
        .then(() => this.ask())
        .then(
          (res) => {
            if (mine === this.version) {
              this.answer.set(res);
            }
          },
          () => {
            if (mine === this.version) {
              this.answer.set(null);
            }
          },
        );
    }, PAUSE_MS);
  }

  /** The box left the screen, or the page is gone: nothing more is asked, and no late answer lands. */
  stop(): void {
    this.version++;
    this.clearTimer();
    this.answer.set(null);
  }

  private clearTimer(): void {
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }
}
