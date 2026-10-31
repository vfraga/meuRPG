// Just the shapes the rule reads: a helper that answers with a revisioned Encounter, one that answers with nothing.
export interface Encounter {
  id: string;
  revision: number;
  combatants: unknown[];
}
interface Locator {
  click(): Promise<void>;
}
export interface Page {
  getByRole(role: string): Locator;
  goto(url: string): Promise<void>;
}
export declare function combatRPC(page: Page, method: string): Promise<Encounter>;
export declare function getEncounterRPC(page: Page): Promise<Encounter>;
export declare function rollSceneRPC(page: Page): Promise<void>;
export declare function waitForEncounterRevision(pages: Page | Page[], expected: Encounter): Promise<void>;
export declare function passTurnsToAndWait(master: Page, campaignId: string, label: string, pages: Page | Page[]): Promise<Encounter>;
export declare function openLive(page: Page, route: string): Promise<void>;
export declare function step(name: string, body: () => Promise<void>): Promise<void>;
