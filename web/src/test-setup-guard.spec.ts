import { noShadowedScrollIntoView } from './test-setup-guard';

// The guard of src/test-setup.ts runs after every test. These tests call it by hand, and put the prototype back as they
// found it, so the real guard does not fail them.
describe('the scrollIntoView guard of the global test setup', () => {
  afterEach(() => {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>)['scrollIntoView'];
  });

  it('passes when HTMLElement.prototype has no scrollIntoView of its own', () => {
    expect(Object.hasOwn(HTMLElement.prototype, 'scrollIntoView')).toBe(false);
    expect(() => noShadowedScrollIntoView()).not.toThrow();
  });

  it('fails a test that left an own scrollIntoView on HTMLElement.prototype, and says what it left behind', () => {
    HTMLElement.prototype.scrollIntoView = () => undefined;
    expect(() => noShadowedScrollIntoView()).toThrow(
      'This test left HTMLElement.prototype.scrollIntoView behind',
    );
  });

  it('removes the property it reports, so only the test that left it fails', () => {
    HTMLElement.prototype.scrollIntoView = () => undefined;
    expect(() => noShadowedScrollIntoView()).toThrow();
    expect(Object.hasOwn(HTMLElement.prototype, 'scrollIntoView')).toBe(false);
    expect(() => noShadowedScrollIntoView()).not.toThrow();
  });
});
