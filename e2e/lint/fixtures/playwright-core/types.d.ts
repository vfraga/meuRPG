// Stands for playwright-core's own types. The rule reads where the type of a receiver is declared (a path with
// `playwright-core` in it), not what the type is called: an app type named `Page` is no Page.
export interface Locator {
  getByLabel(text: string, options?: { exact?: boolean }): Locator;
  click(): Promise<void>;
  clear(): Promise<void>;
  dispatchEvent(type: string): Promise<void>;
  selectText(): Promise<void>;
}
export interface ElementHandle {
  click(): Promise<void>;
}
export interface Frame {
  click(selector: string): Promise<void>;
}
export interface Keyboard {
  press(key: string): Promise<void>;
  down(key: string): Promise<void>;
  up(key: string): Promise<void>;
  insertText(text: string): Promise<void>;
}
export interface Mouse {
  click(x: number, y: number): Promise<void>;
  down(): Promise<void>;
  up(): Promise<void>;
  move(x: number, y: number): Promise<void>;
  wheel(dx: number, dy: number): Promise<void>;
}
export interface Page {
  getByRole(role: string, options?: { name?: string; exact?: boolean }): Locator;
  goto(url: string): Promise<void>;
  getByLabel(text: string, options?: { exact?: boolean }): Locator;
  getByText(text: string, options?: { exact?: boolean }): Locator;
  dragAndDrop(source: string, target: string): Promise<void>;
  frame(): Frame;
  keyboard: Keyboard;
  mouse: Mouse;
  route(url: string, handler: () => Promise<void>): Promise<void>;
  exposeFunction(name: string, callback: () => Promise<void>): Promise<void>;
  waitForEvent(name: string, predicate?: () => boolean): Promise<void>;
  on(event: string, handler: () => Promise<void>): void;
}
