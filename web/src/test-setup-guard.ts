/**
 * `scrollIntoView` is read from `Element.prototype`, where the beforeEach of `test-setup.ts` puts a fresh mock. A spec that
 * assigns it on `HTMLElement.prototype` (or restores it there from a value it read) leaves an own property that shadows every
 * later mock in the worker: the next files see 0 calls, or "scrollIntoView is not a function". The test that leaves it
 * fails, by name. The property is removed first, so the failure stays with the test that left it.
 */
export function noShadowedScrollIntoView(): void {
  if (Object.hasOwn(HTMLElement.prototype, 'scrollIntoView')) {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>)['scrollIntoView'];
    throw new Error(
      'This test left HTMLElement.prototype.scrollIntoView behind: it shadows the mock of Element.prototype for every later file of the worker. Change the mock of Element.prototype instead.',
    );
  }
}
