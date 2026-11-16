import { DestroyRef, Injectable, computed, inject, signal } from '@angular/core';

import { sessionClosed } from '../combat/combat-errors';
import { newKey } from '../connect/idempotency';
import {
  FavorClient,
  type FavorPassVm,
  type FavorReadVm,
  type FavorRefundVm,
} from './favor-client';
import { favorBlocked, favorBlockedMessage } from './favor-errors';
import {
  FAVOR_ANSWERED_ELSEWHERE,
  FAVOR_REFUSED_TEXT,
  FAVOR_RETURNED_TEXT,
  type FavorLine,
  type FavorWork,
  askOutcome,
  cancelOutcome,
  favorAcceptedText,
  favorCanceledText,
  passAnswerOutcome,
  refundAnswerOutcome,
} from './favor-pass';

/** What the master's grant says when the session is over: the only thing the board words for it. */
export const FAVOR_NO_SESSION = 'Só numa sessão aberta.';
/** A grant that failed for a reason the server does not name (the network, an error). */
export const FAVOR_GRANT_FAILED = 'Não foi possível dar a Inspiração. Tente de novo.';
/** The reason under a dashed "Dar Inspiração": "Pensantus já tem Inspiração." */
export function favorHoldsText(name: string): string {
  return `${name} já tem Inspiração.`;
}

/** How long the line after a grant stays on the row before it is emptied, in ms. */
export const FAVOR_NOTE_MS = 5_000;

/** The most lines of the notices region at a time: the oldest goes when a new one comes. */
const MAX_LINES = 4;

/** A line about one character: what the grant answered. */
export interface FavorNote {
  readonly characterId: string;
  readonly text: string;
  readonly refused: boolean;
}

/**
 * Who has the master's Inspiração in the open session (RN-43), read with `GetFavor` and read again on the hint the server tells:
 * `vitals_changed` (a grant tells nothing else, see "Who hears and who reads" in the favor architecture note), on every
 * `ready` and after a lost hint. The master's read has every holder of the campaign; a player's only their own characters, so the
 * same code shows the master the badges and the player the chip, and never anything about another player (RN-10).
 *
 * A read that fails shows no badge and no chip, and a roll goes on without the Inspiração (`failed` lets its sheet say so). The
 * character sheet provides its own copy and calls `follow`. The live session uses the root copy, so the roll sheets it opens
 * (a dialog or a bottom sheet is built from the root injector, never from the page's) read the same state as the page; it calls
 * `follow` when it opens a session and `stop` when it leaves. `providedIn: 'root'` also keeps a component that is tested alone
 * working, with nobody followed.
 */
@Injectable({ providedIn: 'root' })
export class FavorState {
  private readonly client = inject(FavorClient);

  /** The campaign being followed: empty when there is no open session, and the grant is off with it. */
  readonly campaignId = signal('');
  /** The reader is the master: the asks he reads are the cards he answers, never his own (a player's chip waits, the master's does not). */
  readonly master = signal(false);
  /** The holders the last read gave, by character id, with the character's name. Empty while the read has not come or failed. */
  readonly holders = signal<ReadonlyMap<string, string>>(new Map());
  /** The asks that wait for the master, as the reader may read them: every one for the master, only the asks of their own characters for a
   * player (RN-10). A read that fails keeps the last ones: a card the master is about to answer does not vanish with the network. */
  readonly passes = signal<readonly FavorPassVm[]>([]);
  /** The master's questions about an Inspiração a roll spent for nothing. Only the master reads any. */
  readonly refunds = signal<readonly FavorRefundVm[]>([]);
  /** The calls on their way, by the id of the ask or the question (the character's id for a new ask): the pressed button reads what
   * it does and its pair is dashed (581-07, item 14). */
  readonly working = signal<ReadonlyMap<string, FavorWork>>(new Map());
  /** What the reader is told: the master, that a card closed under him; a player, what the master answered. Polite, dismissable. */
  readonly lines = signal<readonly FavorLine[]>([]);
  /** The last read failed (the network, an error): the roll sheets say so, dashed, instead of hiding the box (board 581-07, item 13).
   * `false` while no read has come yet, and again after a read that works. */
  readonly failed = signal(false);
  /** How many characters have it ("1 com Inspiração" on the master's Grupo tab). */
  readonly count = computed(() => this.holders().size);
  /** The character a player has just received it for, until they dismiss the notice or no longer have it; empty otherwise. */
  readonly gained = signal('');
  /** The characters whose grant is on its way: a second tap does not send another. */
  readonly granting = signal<ReadonlySet<string>>(new Set());
  /** What the last grant answered, for a polite live region on the row of that character. */
  readonly note = signal<FavorNote | null>(null);

  private seq = 0;
  private reading = false;
  private again = false;
  /** The last read that worked, to tell a new holder from one the player already had; `null` until the first one. */
  private lastGood: ReadonlySet<string> | null = null;
  private noteTimer: ReturnType<typeof setTimeout> | undefined;
  /** The asks and questions this screen closed itself: their leaving the list is no news. */
  private readonly handled = new Set<string>();
  /** The characters that lost the Inspiração without a pass: one that comes back after a roll spent it was given back, not granted. */
  private readonly spentIds = new Set<string>();
  private lastPasses: readonly FavorPassVm[] = [];
  private lastRefunds: readonly FavorRefundVm[] = [];
  private lineSeq = 0;

  constructor() {
    inject(DestroyRef).onDestroy(() => clearTimeout(this.noteTimer));
  }

  /** Whether `characterId` has it, as far as the reader may know. */
  has(characterId: string): boolean {
    return this.holders().has(characterId);
  }

  /** The ask of a character that waits for the master, as the reader may read it (the giver's own; the master reads them as cards). */
  passOf(characterId: string): FavorPassVm | undefined {
    return this.passes().find((p) => p.fromId === characterId);
  }

  /** The character's own ask waits for the master, and the reader is not the master: the chip and the spend box say so (581-07, item 4). */
  waiting(characterId: string): boolean {
    return !this.master() && this.passOf(characterId) !== undefined;
  }

  /** Follows the open session of `campaignId` and reads who has it; the same campaign again just reads again. `master` says who reads. */
  follow(campaignId: string, master = false): void {
    this.master.set(master);
    if (campaignId === this.campaignId()) {
      void this.refresh();
      return;
    }
    this.reset();
    this.campaignId.set(campaignId);
    void this.refresh();
  }

  /** The session is over, or the page left: nothing is known and nothing is offered. */
  stop(): void {
    this.reset();
    this.campaignId.set('');
  }

  /** Reads who has it. A read already on its way runs once more when it ends, so a burst of hints is one read behind the last one. */
  async refresh(): Promise<void> {
    const campaignId = this.campaignId();
    if (campaignId === '') {
      return;
    }
    if (this.reading) {
      this.again = true;
      return;
    }
    this.reading = true;
    const seq = this.seq;
    try {
      const read = await this.client.read(campaignId);
      if (seq === this.seq) {
        this.take(read);
        this.failed.set(false);
      }
    } catch {
      // A read that fails shows no chip and no badge, and a roll goes on without the Inspiração: its sheet says "Não deu para
      // ler a Inspiração agora" (the box, dashed). The next hint reads again.
      if (seq === this.seq) {
        this.holders.set(new Map());
        this.failed.set(true);
      }
    } finally {
      if (seq === this.seq) {
        this.reading = false;
      }
    }
    if (this.again && seq === this.seq) {
      this.again = false;
      await this.refresh();
    }
  }

  /** The master's grant. Never throws: the answer is in `note`. */
  async grant(characterId: string, name: string): Promise<void> {
    const campaignId = this.campaignId();
    if (campaignId === '') {
      this.say(characterId, FAVOR_NO_SESSION, true);
      return;
    }
    if (this.granting().has(characterId)) {
      return;
    }
    this.granting.update((s) => new Set(s).add(characterId));
    try {
      await this.client.grant(campaignId, characterId, crypto.randomUUID());
      this.say(characterId, `${name} ganhou Inspiração.`, false);
    } catch (err) {
      this.say(characterId, this.refusal(err), true);
    } finally {
      this.granting.update((s) => {
        const next = new Set(s);
        next.delete(characterId);
        return next;
      });
    }
    // A grant, or a refusal because it was already there: either way the master's read is what is true now.
    await this.refresh();
  }

  /** The player closed the notice. */
  dismissGained(): void {
    this.gained.set('');
  }

  private take(read: FavorReadVm): void {
    const next: ReadonlyMap<string, string> = new Map(
      read.holders.map((h) => [h.characterId, h.name]),
    );
    const before = this.lastGood;
    this.lastGood = new Set(next.keys());
    this.holders.set(next);
    const prevPasses = this.lastPasses;
    const prevRefunds = this.lastRefunds;
    this.lastPasses = read.passes;
    this.lastRefunds = read.refunds;
    this.passes.set(read.passes);
    this.refunds.set(read.refunds);
    if (before !== null) {
      this.tellClosed(prevPasses, read.passes, prevRefunds, read.refunds, next, before);
      // A holder that was not there at the last read that worked is a grant (not the first read: it had it already), or the
      // Inspiração a roll spent that the master gave back.
      const fresh = [...next.keys()].find((id) => !before.has(id));
      if (fresh !== undefined) {
        this.arrived(fresh);
      }
    }
    if (this.gained() !== '' && !next.has(this.gained())) {
      this.gained.set('');
    }
  }

  /** A character got the Inspiração: given back to the roller that spent it, else a grant (or a pass that reached it). */
  private arrived(characterId: string): void {
    if (!this.master() && this.spentIds.delete(characterId)) {
      this.tell(FAVOR_RETURNED_TEXT, 'success', 'star');
      return;
    }
    this.gained.set(characterId);
  }

  /** What leaving the lists means to the reader: the giver reads the master's answer to their ask; the master reads that a card he did
   * not answer here closed. A call of this screen is no news to this screen (`handled`). */
  private tellClosed(
    prevPasses: readonly FavorPassVm[],
    passes: readonly FavorPassVm[],
    prevRefunds: readonly FavorRefundVm[],
    refunds: readonly FavorRefundVm[],
    holders: ReadonlyMap<string, string>,
    before: ReadonlySet<string>,
  ): void {
    const open = new Set(passes.map((p) => p.id));
    const gone = prevPasses.filter((p) => !open.has(p.id));
    // The Inspiração a pass took from its giver: the accept moved it, so it is not a spent one.
    const moved = new Set(gone.map((p) => p.fromId));
    for (const id of before) {
      if (!holders.has(id) && !moved.has(id)) {
        this.spentIds.add(id);
      }
    }
    for (const p of gone) {
      if (this.handled.delete(p.id)) {
        continue;
      }
      if (this.master()) {
        this.tell(favorCanceledText(p.fromName), 'info', 'keyboard_tab');
      } else if (!holders.has(p.fromId)) {
        this.tell(favorAcceptedText(p.toName), 'success', 'keyboard_tab');
      } else {
        this.tell(FAVOR_REFUSED_TEXT, 'neutral', 'keyboard_tab');
      }
    }
    const openRefunds = new Set(refunds.map((r) => r.id));
    for (const r of prevRefunds.filter((r) => !openRefunds.has(r.id))) {
      if (!this.handled.delete(r.id) && this.master()) {
        this.tell(FAVOR_ANSWERED_ELSEWHERE, 'info', 'info');
      }
    }
  }

  /** A line of the notices region. */
  private tell(text: string, tone: FavorLine['tone'], icon: string): void {
    const line: FavorLine = { id: ++this.lineSeq, text, tone, icon };
    this.lines.update((list) => [...list, line].slice(-MAX_LINES));
  }

  /** The reader closed a line of the notices region. */
  dismissLine(id: number): void {
    this.lines.update((list) => list.filter((l) => l.id !== id));
  }

  // ---- the pass and the refund question ----

  /** Runs a call of the pass or of the refund question under `key`: a second tap of the same card does nothing while one is on its way. */
  private async work<T>(key: string, what: FavorWork, call: () => Promise<T>): Promise<T> {
    this.working.update((m) => new Map(m).set(key, what));
    try {
      return await call();
    } finally {
      this.working.update((m) => {
        const next = new Map(m);
        next.delete(key);
        return next;
      });
    }
  }

  /** What the call of `key` is doing, or `null`. */
  workOf(key: string): FavorWork | null {
    return this.working().get(key) ?? null;
  }

  /** The giver's ask to pass the Inspiração of `fromId` to `toId`. Answers `null` when it worked, else what the dialog says. The key is the
   * dialog's: a retry of the same ask answers the same pass. */
  async ask(fromId: string, toId: string, key: string): Promise<string | null> {
    const campaignId = this.campaignId();
    if (this.workOf(fromId) !== null) {
      return null;
    }
    try {
      const pass = await this.work(fromId, 'ask', () =>
        this.client.pass(campaignId, fromId, toId, key),
      );
      if (pass) {
        this.passes.update((list) => [...list.filter((p) => p.id !== pass.id), pass]);
        this.lastPasses = this.passes();
      }
      void this.refresh();
      return null;
    } catch (err) {
      const out = askOutcome(err);
      if (out.stale) {
        void this.refresh();
      }
      return out.text;
    }
  }

  /** The giver gives the ask up. Answers `null` when it worked, else what to say. */
  async cancelAsk(pass: FavorPassVm): Promise<string | null> {
    if (this.workOf(pass.id) !== null) {
      return null;
    }
    // The hint of the cancel can bring a read before the answer: the ask leaving the list is this screen's own doing.
    this.handled.add(pass.id);
    try {
      await this.work(pass.id, 'cancel', () =>
        this.client.cancelPass(this.campaignId(), pass.id, newKey()),
      );
      this.passes.update((list) => list.filter((p) => p.id !== pass.id));
      void this.refresh();
      return null;
    } catch (err) {
      const out = cancelOutcome(err);
      if (!out.stale) {
        this.handled.delete(pass.id);
      }
      if (out.stale) {
        void this.refresh();
      }
      return out.text;
    }
  }

  /** The master's answer to an ask. A refusal that closes the card says why in a line; one that leaves it says it in a line too. */
  async answerPass(pass: FavorPassVm, accept: boolean): Promise<void> {
    if (this.workOf(pass.id) !== null || (accept && pass.receiverHas)) {
      return;
    }
    // The hint of the answer can bring a read before the answer itself: the card leaving the list is this screen's own doing.
    this.handled.add(pass.id);
    try {
      await this.work(pass.id, accept ? 'accept' : 'refuse', () =>
        this.client.answerPass(this.campaignId(), pass.id, accept, newKey()),
      );
      this.closeCard(pass.id);
    } catch (err) {
      const out = passAnswerOutcome(err, pass);
      if (out.closes) {
        this.closeCard(pass.id);
      } else {
        this.handled.delete(pass.id);
      }
      this.tell(out.text, out.closes ? 'info' : 'neutral', 'keyboard_tab');
    }
    await this.refresh();
  }

  /** The master's answer to the refund question: "Devolver a Inspiração" or "Manter gasta". */
  async answerRefund(prompt: FavorRefundVm, giveBack: boolean): Promise<void> {
    if (this.workOf(prompt.id) !== null) {
      return;
    }
    this.handled.add(prompt.id);
    try {
      await this.work(prompt.id, giveBack ? 'give' : 'keep', () =>
        this.client.answerRefund(this.campaignId(), prompt.id, giveBack, newKey()),
      );
      this.closeCard(prompt.id);
    } catch (err) {
      const out = refundAnswerOutcome(err, prompt);
      if (out.closes) {
        this.closeCard(prompt.id);
      } else {
        this.handled.delete(prompt.id);
      }
      if (out.keep) {
        // The server leaves the question open after a refused give back: the roller has one, so keeping it spent is what is true.
        await this.keepQuietly(prompt);
      }
      this.tell(out.text, out.closes ? 'info' : 'neutral', out.keep ? 'star' : 'info');
    }
    await this.refresh();
  }

  private async keepQuietly(prompt: FavorRefundVm): Promise<void> {
    try {
      await this.client.answerRefund(this.campaignId(), prompt.id, false, newKey());
    } catch {
      // Closed already, or the network: the card is out of the screen either way, and the server closes it with the session.
    }
  }

  /** The card of an ask or a question leaves the screen at once; the next read confirms it. */
  private closeCard(id: string): void {
    this.passes.update((list) => list.filter((p) => p.id !== id));
    this.refunds.update((list) => list.filter((r) => r.id !== id));
  }

  private refusal(err: unknown): string {
    if (sessionClosed(err)) {
      return FAVOR_NO_SESSION;
    }
    const blocked = favorBlocked(err);
    return blocked ? favorBlockedMessage(blocked.reason) : FAVOR_GRANT_FAILED;
  }

  private say(characterId: string, text: string, refused: boolean): void {
    this.note.set({ characterId, text, refused });
    clearTimeout(this.noteTimer);
    this.noteTimer = setTimeout(() => this.note.set(null), FAVOR_NOTE_MS);
  }

  private reset(): void {
    this.seq++;
    this.reading = false;
    this.again = false;
    this.lastGood = null;
    this.holders.set(new Map());
    this.passes.set([]);
    this.refunds.set([]);
    this.lastPasses = [];
    this.lastRefunds = [];
    this.handled.clear();
    this.spentIds.clear();
    this.working.set(new Map());
    this.lines.set([]);
    this.failed.set(false);
    this.gained.set('');
    this.granting.set(new Set());
    clearTimeout(this.noteTimer);
    this.note.set(null);
  }
}
