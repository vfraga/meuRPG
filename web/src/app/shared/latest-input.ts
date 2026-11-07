import { type Signal, effect, untracked } from '@angular/core';

/**
 * The value a component edits, as the last edit left it. An input only shows what the parent rendered, and the parent
 * renders after the event, so two edits before that render (a pick and a field right after it) would each build on the
 * old value and the second would drop the first. `current()` is the last value this component sent, until the input
 * brings a new one; call it in an injection context (a field initializer or the constructor).
 */
export function latestInput<T>(input: Signal<T>): { current(): T; sent(value: T): void } {
  let last: { value: T } | null = null;
  effect(() => {
    input();
    last = null;
  });
  return {
    current: () => (last ? last.value : untracked(input)),
    sent: (value) => {
      last = { value };
    },
  };
}
