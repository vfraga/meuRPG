import { signal } from '@angular/core';

import type { ChoicesPreviewVm } from './character-editor.types';

/** How long the draft rests before the server is asked: a run of keystrokes (a favored enemy's race) asks once. */
const PAUSE_MS = 300;

/**
 * Whether two answers say the same: a signal set with an equal answer would still notify, and an effect that reads the
 * draft the answer feeds (the spell sections, the hit points) would ask again, for ever, with nothing changed.
 */
export function sameAnswer<T>(a: T | null, b: T): boolean {
  return a !== null && JSON.stringify(a) === JSON.stringify(b);
}

/**
 * The choices the server reads in the draft on screen (`PreviewChoices`, PM-05). The browser knows no rule: which
 * choices a race and a class ask, what each option stores and what is still open are the server's answer. Each
 * change of the draft asks once, after a pause; only the newest answer counts; while one is on the way the last
 * stays; a draft the server cannot read yet (no race or class) shows none; a call that fails keeps the last answer
 * (the save is still checked by the server, which refuses what is open). The same asking serves any answer the server
 * gives about the draft (`T`).
 */
export class ServerChoices<T = ChoicesPreviewVm> {
  /** The newest answer, `null` while the draft has none to show. */
  readonly answer = signal<T | null>(null);

  private timer: ReturnType<typeof setTimeout> | null = null;
  private version = 0;

  constructor(private readonly ask: () => Promise<T>) {}

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
            if (mine === this.version && !sameAnswer(this.answer(), res)) {
              this.answer.set(res);
            }
          },
          () => {
            // Keeps the last answer: a blip must not take the step away from under the person.
          },
        );
    }, PAUSE_MS);
  }

  /** The page is gone: nothing more is asked, and no late answer lands. */
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
