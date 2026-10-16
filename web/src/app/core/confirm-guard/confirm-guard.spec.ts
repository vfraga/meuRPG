import { TestBed } from '@angular/core/testing';

import { CONFIRM_GUARD_MS, ConfirmGuard } from './confirm-guard';

// With --coverage test-setup.ts only hooks the first file of a worker, so the fake clock this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.useRealTimers();
});

describe('ConfirmGuard, a double-click never confirms a destructive inline action', () => {
  let host: HTMLElement;

  beforeEach(() => {
    vi.useFakeTimers();
    host = document.createElement('div');
    document.body.append(host);
    TestBed.inject(ConfirmGuard).start();
  });

  afterEach(() => {
    host.remove();
  });

  /** Opens a confirmation the way the app does: a container added with its buttons. */
  async function open() {
    const ask = document.createElement('div');
    ask.className = 'ask';
    ask.setAttribute('role', 'alertdialog');
    ask.setAttribute('data-confirm', '');
    ask.innerHTML = '<button class="danger">Descartar</button><button class="safe">Voltar</button>';
    host.append(ask);
    await vi.advanceTimersByTimeAsync(0);
    const danger = ask.querySelector<HTMLButtonElement>('.danger')!;
    const onDanger = vi.fn();
    danger.addEventListener('click', onDanger);
    return { ask, danger, onDanger };
  }

  it('ignores a click on the destructive button right after the confirmation opened', async () => {
    const { danger, onDanger } = await open();
    danger.click();
    await vi.advanceTimersByTimeAsync(300);
    danger.click();
    expect(onDanger).not.toHaveBeenCalled();
  });

  it('marks the buttons aria-disabled during the guard, so a test waits for them, and clears it after', async () => {
    const { danger } = await open();
    expect(danger.getAttribute('aria-disabled')).toBe('true');
    await vi.advanceTimersByTimeAsync(CONFIRM_GUARD_MS + 1);
    expect(danger.hasAttribute('aria-disabled')).toBe(false);
  });

  it('works after the guard window', async () => {
    const { danger, onDanger } = await open();
    await vi.advanceTimersByTimeAsync(CONFIRM_GUARD_MS + 1);
    danger.click();
    expect(onDanger).toHaveBeenCalledTimes(1);
  });

  it('does not touch a field inside the confirmation', async () => {
    const { ask } = await open();
    const input = document.createElement('input');
    input.type = 'checkbox';
    ask.append(input);
    input.click();
    expect(input.checked).toBe(true);
  });

  it('leaves a button outside any confirmation alone', async () => {
    const plain = document.createElement('button');
    host.append(plain);
    await vi.advanceTimersByTimeAsync(0);
    const onClick = vi.fn();
    plain.addEventListener('click', onClick);
    plain.click();
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
