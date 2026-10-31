import { type Injector, afterNextRender, signal } from '@angular/core';

import {
  type Placement,
  NO_PLACEMENT,
  type ViolationContext,
  placeViolations,
  refusalSummary,
} from './content-violations';
import { ActionKey } from '../connect/idempotency';
import { contentErrorText, isStale, refusalOf } from './content-client';

/**
 * What an editor does around a write (MR-025, E10-01 states 5): runs it, and on a refusal puts every violation back on
 * its field, counts them for the summary at the top, and keeps what the person typed. `stale` is "Esta entrada mudou
 * enquanto você editava" (the page offers to reload); any other failure is a sentence in `error`. A write that works
 * clears all of it.
 */
export class EntrySaver {
  readonly saving = signal(false);
  readonly placement = signal<Placement>(NO_PLACEMENT);
  readonly error = signal('');
  readonly stale = signal(false);
  /** The summary of a refusal, for the alert at the top. */
  readonly summary = signal('');

  constructor(
    private readonly ctx: ViolationContext,
    /** "a magia", "a raça": the summary says "Não foi possível salvar a magia." */
    private readonly what: string,
  ) {}

  /** The idempotency key of a new entry: the same body again (a retry) keeps it, another body or a write that worked renews it. */
  private readonly createKey = new ActionKey();

  keyFor(body: unknown): string {
    return this.createKey.keyFor(body);
  }

  /** The messages of one input, by its path. */
  issues = (path: string): readonly string[] =>
    (this.placement().byField.get(path) ?? []).map((p) => p.text);

  /** Stops a save the client already knows is bad (`paths` are the fields it cannot send, in the order to show them): an older refusal
   * leaves the screen, so it does not sit beside the client's errors (the stale banner stays: the entry is out of date until it is reloaded), and `show` takes the person to the first path. Returns whether it stopped. */
  blockOn(paths: readonly string[], show: (path: string) => void): boolean {
    if (paths.length === 0) {
      return false;
    }
    this.placement.set(NO_PLACEMENT);
    this.summary.set('');
    show(paths[0]);
    return true;
  }

  /** After a refusal, focuses the first field it landed on once the render that shows the refusals is done. */
  focusRefusal(root: ParentNode, injector: Injector): void {
    afterNextRender(
      () => {
        const first = this.placement().fields[0];
        if (first) {
          focusField(root, first);
        }
      },
      { injector },
    );
  }

  clear(): void {
    this.placement.set(NO_PLACEMENT);
    this.error.set('');
    this.stale.set(false);
    this.summary.set('');
  }

  /** Runs the write. Returns its result, or `null` after it failed (the reason is in the signals). `isKnown` says which
   * paths the editor draws an input for at this moment. */
  async run<R>(write: () => Promise<R>, isKnown: (path: string) => boolean): Promise<R | null> {
    this.saving.set(true);
    this.clear();
    try {
      const result = await write();
      this.createKey.renew();
      return result;
    } catch (err) {
      const violations = refusalOf(err);
      if (violations) {
        const placement = placeViolations(violations, isKnown, this.ctx);
        this.placement.set(placement);
        this.summary.set(refusalSummary(this.what, placement.count));
      } else if (isStale(err)) {
        this.stale.set(true);
      } else {
        this.error.set(contentErrorText(err, `salvar ${this.what}`));
      }
      return null;
    } finally {
      this.saving.set(false);
    }
  }
}

/** Moves the focus to the input of a path (or the first control inside a group that carries it) and scrolls it into view. */
export function focusField(root: ParentNode, path: string): boolean {
  const escaped =
    typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(path) : path.replace(/["\\]/g, '\\$&');
  const el = root.querySelector<HTMLElement>(`[data-field="${escaped}"]`);
  if (!el) {
    return false;
  }
  const target = el.matches('input, select, textarea, button')
    ? el
    : el.querySelector<HTMLElement>('input, select, textarea, button');
  (target ?? el).focus({ preventScroll: true });
  (target ?? el).scrollIntoView?.({ block: 'center', behavior: 'auto' });
  return true;
}

/** Focuses the input of a path once the render that draws it is done: the open of a feature goes before, so the input exists. */
export function focusAfterRender(root: ParentNode, injector: Injector, path: string): void {
  afterNextRender(
    () => {
      focusField(root, path);
    },
    { injector },
  );
}
