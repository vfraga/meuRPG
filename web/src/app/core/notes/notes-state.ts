import { computed, signal } from '@angular/core';

import type { Note, NoteScene } from '../../../gen/meurpg/notes/v1/notes_pb';
import { ActionKey } from '../connect/idempotency';
import type { NotesClient } from './notes-client';
import { isClue, sortNotes } from './notes-view';

/** The key of the note being written for the first time. */
export const NEW = 'new';

/**
 * The player's notes of one campaign (MR-030), as the session page, its notes
 * sheet and the character sheet's panel read them: a signal for the list
 * (newest first), the scenes the group discovered (the tag picker), and what
 * is new. Pure TypeScript with signals, so the rules are tested without a DOM:
 *
 * - the first read, and the one after each `ready`, say nothing (a clue
 *   received while offline is just in the list);
 * - a clue that arrives through `notes_changed` is new: it counts on the
 *   app bar's button and raises the notice, until the person opens the notes
 *   (`seen`) or dismisses the notice;
 * - one write at a time per entry (`writing`): a second tap on the same note
 *   is ignored while the first is in flight;
 * - a stale read never overwrites a newer one, and a failed read keeps the
 *   copy on screen (the next event reads it again).
 */
export class NotesState {
  readonly notes = signal<readonly Note[]>([]);
  readonly scenes = signal<readonly NoteScene[]>([]);
  /** How many notes (not clues) the player has, and the most they may have. */
  readonly noteCount = signal(0);
  readonly maxNotes = signal(300);
  readonly loaded = signal(false);
  readonly failed = signal(false);
  /** The clues that arrived since the notes were last opened. */
  readonly fresh = signal<readonly Note[]>([]);
  /** The "O mestre revelou uma pista para você" notice is up. */
  readonly notice = signal(false);
  /** The notes that are being written now (the new one is `NEW`). */
  readonly writing = signal<ReadonlySet<string>>(new Set());

  readonly atLimit = computed(() => this.noteCount() >= this.maxNotes());

  private generation = 0;
  /** Moves with `clear()`: a write that began before it belongs to another campaign or session and leaves no trace. */
  private session = 0;
  /** Moves with each write's answer: a read that began before it may lack the written note, so it is dropped and
   * made again (still announcing the clues it brings), not lost. */
  private edits = 0;
  /** The key of the note being written: kept across the retries of one note, new for the next. */
  private readonly createKey = new ActionKey();

  constructor(
    private readonly api: Pick<NotesClient, 'list' | 'scenes' | 'create' | 'update' | 'delete'>,
    private readonly campaignId: () => string,
  ) {}

  /**
   * Reads the list and the discovered scenes again. `announce` is true only
   * for a `notes_changed`: the clues that were not here before are new.
   */
  async refresh(announce = false): Promise<void> {
    const generation = ++this.generation;
    const edits = this.edits;
    const campaignId = this.campaignId();
    try {
      const [list, scenes] = await Promise.all([
        this.api.list(campaignId),
        this.api.scenes(campaignId),
      ]);
      if (generation !== this.generation) {
        return;
      }
      if (edits !== this.edits) {
        return await this.refresh(announce);
      }
      const known = new Set(this.notes().map((n) => n.id));
      const arrived =
        this.loaded() && announce ? list.notes.filter((n) => isClue(n) && !known.has(n.id)) : [];
      this.notes.set(sortNotes(list.notes));
      this.scenes.set(scenes);
      this.noteCount.set(list.noteCount);
      this.maxNotes.set(list.maxNotes || 300);
      this.loaded.set(true);
      this.failed.set(false);
      if (arrived.length > 0) {
        this.fresh.update((now) => [...now, ...arrived]);
        this.notice.set(true);
      }
    } catch {
      if (generation === this.generation && edits === this.edits) {
        this.failed.set(true);
      }
    }
  }

  /** Only the scenes (the tag picker), when a form opens: a scene may have been discovered since. */
  async refreshScenes(): Promise<void> {
    const session = this.session;
    try {
      const scenes = await this.api.scenes(this.campaignId());
      if (session === this.session) {
        this.scenes.set(scenes);
      }
    } catch {
      // The picker keeps the list it had.
    }
  }

  /** The notes were opened: nothing is new any more, and the notice goes. */
  seen(): void {
    this.fresh.set([]);
    this.notice.set(false);
  }

  dismissNotice(): void {
    this.notice.set(false);
  }

  /** A new session or page: nothing is loaded, and the next read is silent. */
  clear(): void {
    this.generation++;
    this.session++;
    this.notes.set([]);
    this.scenes.set([]);
    this.noteCount.set(0);
    this.loaded.set(false);
    this.failed.set(false);
    this.fresh.set([]);
    this.notice.set(false);
    this.writing.set(new Set());
  }

  // ---- writes: the answer goes into the list at once ----

  /** Whether this entry (or the new one, `NEW`) has a write in flight. */
  isWriting(id: string): boolean {
    return this.writing().has(id);
  }

  /**
   * `call` runs the request and returns what to do with the answer; the
   * answer is applied only while the session it began in is still the current
   * one, so a write of campaign A never lands in the list of campaign B.
   */
  private async write<T>(
    id: string,
    call: () => Promise<{ value: T; apply: () => void }>,
  ): Promise<T | null> {
    if (this.isWriting(id)) {
      return null;
    }
    const session = this.session;
    this.writing.update((set) => new Set(set).add(id));
    try {
      const { value, apply } = await call();
      if (session === this.session) {
        apply();
      }
      return value;
    } finally {
      if (session === this.session) {
        this.writing.update((set) => {
          const next = new Set(set);
          next.delete(id);
          return next;
        });
      }
    }
  }

  /** A new note. `null` while another write of it is in flight; a refusal throws. */
  create(text: string, scenePointId: string): Promise<Note | null> {
    return this.write(NEW, async () => {
      // A retry of the same note (a lost answer, a second tap) sends the same key and adds it once.
      const note = await this.api.create(
        this.campaignId(),
        text,
        scenePointId,
        this.createKey.keyFor([text, scenePointId]),
      );
      this.createKey.renew();
      return {
        value: note,
        apply: () => {
          this.edits++;
          this.notes.update((list) => sortNotes([note, ...list]));
          this.noteCount.update((n) => n + 1);
        },
      };
    });
  }

  /** Changes the fields given. With neither there is nothing to send (the server refuses it): the note stays as it is. */
  async update(
    noteId: string,
    changes: { text?: string; scenePointId?: string },
  ): Promise<Note | null> {
    if (changes.text === undefined && changes.scenePointId === undefined) {
      return this.notes().find((n) => n.id === noteId) ?? null;
    }
    return this.write(noteId, async () => {
      const note = await this.api.update(this.campaignId(), noteId, changes);
      return {
        value: note,
        apply: () => {
          this.edits++;
          this.notes.update((list) => sortNotes(list.map((n) => (n.id === noteId ? note : n))));
        },
      };
    });
  }

  /** Deletes one of the player's own notes. `false` while a write of it is in flight. */
  async remove(noteId: string): Promise<boolean> {
    const done = await this.write(noteId, async () => {
      await this.api.delete(this.campaignId(), noteId);
      return {
        value: true,
        apply: () => {
          this.edits++;
          this.notes.update((list) => list.filter((n) => n.id !== noteId));
          this.noteCount.update((n) => Math.max(0, n - 1));
        },
      };
    });
    return done === true;
  }
}
