import { NewestRead } from './newest-read';

describe('NewestRead', () => {
  const deferred = () => {
    let done!: (v: boolean) => void;
    const promise = new Promise<boolean>((resolve) => (done = resolve));
    return { promise, done };
  };

  it('answers an overtaken read with the newer read, once it has landed', async () => {
    const reads = new NewestRead();
    const older = deferred();
    const newer = deferred();
    const first = reads.start('c1', (seq) =>
      older.promise.then((v) => v || reads.overtaken('c1', seq)),
    );
    const second = reads.start('c1', () => newer.promise);
    let answered = false;
    void first.then(() => (answered = true));
    older.done(false); // the older read lands first and finds it was overtaken
    await Promise.resolve();
    await Promise.resolve();
    expect(answered).toBe(false);
    newer.done(true);
    expect(await second).toBe(true);
    expect(await first).toBe(true);
  });

  it('answers false when no read started after it, as when a page load took over', async () => {
    const reads = new NewestRead();
    const seq = reads.next('c1');
    expect(reads.holds('c1', seq)).toBe(true);
    reads.next('c1'); // a load takes the screen
    expect(reads.holds('c1', seq)).toBe(false);
    expect(await reads.overtaken('c1', seq)).toBe(false);
  });

  it('follows the newest of three', async () => {
    const reads = new NewestRead();
    const a = reads.start('c1', (seq) =>
      Promise.resolve(false).then(() => reads.overtaken('c1', seq)),
    );
    const b = reads.start('c1', (seq) =>
      Promise.resolve(false).then(() => reads.overtaken('c1', seq)),
    );
    const c = reads.start('c1', () => Promise.resolve(true));
    expect(await Promise.all([a, b, c])).toEqual([true, true, true]);
  });

  it('keeps the numbers of each character apart: a read of another character never overtakes this one', () => {
    const reads = new NewestRead();
    const mine = reads.next('c1');
    reads.next('c2'); // another character is read
    expect(reads.holds('c1', mine)).toBe(true);
    reads.next('c1');
    expect(reads.holds('c1', mine)).toBe(false);
  });

  it('never answers a read with the read of another character, even when that one started later and landed true', async () => {
    const reads = new NewestRead();
    const seq = reads.next('c1');
    const other = reads.start('c2', () => Promise.resolve(true));
    expect(await other).toBe(true);
    expect(reads.holds('c1', seq)).toBe(true);
    expect(await reads.overtaken('c1', seq)).toBe(false);
  });

  it('answers an overtaken read with the newer read of the same character only', async () => {
    const reads = new NewestRead();
    const older = reads.next('c1');
    const another = reads.start('c2', () => Promise.resolve(true));
    expect(await reads.overtaken('c1', older)).toBe(false);
    const newer = reads.start('c1', () => Promise.resolve(true));
    expect(await reads.overtaken('c1', older)).toBe(true);
    expect([await another, await newer]).toEqual([true, true]);
  });
});
