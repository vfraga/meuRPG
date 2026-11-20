/** The newest number given for one thing, and the read that holds it. */
interface Newest {
  count: number;
  seq: number;
  run: Promise<boolean>;
}

/**
 * The reads of one thing that overtake each other (a screen that reads a character again each time something changes).
 * Every read, and whatever else takes the screen over (a first load), takes a number from `next(key)`, where the key is what is
 * read (the character's id); only the last number of a key holds. The keys are apart: a read of another character never
 * overtakes this one's, and never answers for it.
 * A read that a newer one of the same key overtook answers with the newer read's result, once it has landed, so whoever waits for
 * "what the screen shows is current" never gets an early yes (the old data) or a no (the newer read had it).
 */
export class NewestRead {
  private readonly keys = new Map<string, Newest>();

  private of(key: string): Newest {
    let newest = this.keys.get(key);
    if (!newest) {
      newest = { count: 0, seq: 0, run: Promise.resolve(false) };
      this.keys.set(key, newest);
    }
    return newest;
  }

  /** A new number for `key`: whatever holds an older one of the same key is overtaken. */
  next(key: string): number {
    return ++this.of(key).count;
  }

  /** Whether nothing took a number of `key` after `seq`. */
  holds(key: string, seq: number): boolean {
    return seq === this.of(key).count;
  }

  /** Starts a read of `key` with a new number and keeps it as the newest read of that key. */
  start(key: string, read: (seq: number) => Promise<boolean>): Promise<boolean> {
    const seq = this.next(key);
    const run = read(seq);
    const newest = this.of(key);
    newest.seq = seq;
    newest.run = run;
    return run;
  }

  /** The answer of the read of `key` numbered `seq` that found it was overtaken: the newest read's, or `false` when no read of the key started after it. */
  overtaken(key: string, seq: number): Promise<boolean> {
    const newest = this.of(key);
    return newest.seq > seq ? newest.run : Promise.resolve(false);
  }
}
