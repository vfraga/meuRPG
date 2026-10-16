import { nextTurn, settle } from './test-settle';

describe('test-settle', () => {
  afterEach(() => vi.useRealTimers());

  it('waits one real turn when the clock is real', async () => {
    let ran = false;
    setTimeout(() => (ran = true), 0);
    await nextTurn();
    expect(ran).toBe(true);
  });

  it('does not hang on a fake clock a spec left on: it advances that clock', async () => {
    vi.useFakeTimers();
    let ran = false;
    setTimeout(() => (ran = true), 0);
    await nextTurn();
    expect(ran).toBe(true);
  });

  it('does not hang when only Date is faked, as the puzzle specs do', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    await nextTurn();
  });

  it('settles a fixture: one render before and one after the turns', async () => {
    const fixture = { detectChanges: vi.fn(), whenStable: vi.fn().mockResolvedValue(undefined) };
    await settle(fixture, 2);
    expect(fixture.detectChanges).toHaveBeenCalledTimes(3);
    expect(fixture.whenStable).toHaveBeenCalledTimes(2);
  });
});
