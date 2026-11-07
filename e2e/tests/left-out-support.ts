import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

import { type Locator, type Page, type Route } from '@playwright/test';

// Setup for the "left out" specs (issue #736): the engine refuses an entry only when the data changes under it (a newer
// rule set), and the API refuses to save one it would refuse, so no call can make a refused entry. The test says it the
// way the server does: the answer of ListTableEntries gets the `leftOut` it would have (a TableContentViolation each), for
// the entries the master has not yet archived, which is what the server does too. The rest of the real answer is kept.

export interface FakeViolation {
  key: string;
  field: string;
  reason: string;
  message?: string;
}

/** An entry as the API lists it: the few fields the helper reads, and the rest of the body as it comes. */
export interface StoredEntry {
  key: string;
  revision: number;
  archived?: boolean;
  [field: string]: unknown;
}

/** Sets the first trait's first effect target of a stored table race: the code the rules no longer have. */
export function targetLeftTheMenu(entry: StoredEntry, target = 'alvo_antigo'): void {
  const race = entry.tableRace as { traits: { effects: { target: string }[] }[] };
  race.traits[0].effects[0].target = target;
}

/** From now on this page's ListTableEntries says these entries are refused, until the entry is archived or fixed (saved). It returns
 * the end of it: a test calls it before it closes the page, so a read the page started is answered first. */
export async function refuseEntries(
  page: Page,
  violations: readonly FakeViolation[],
  /** What the stored entry says, by key: the data that the engine refuses (a target that left the menu), as the API gives it. */
  stored: Readonly<Record<string, (entry: StoredEntry) => void>> = {},
): Promise<() => Promise<void>> {
  // The revision each entry had when it was first listed: a save moves it, and the server would read the entry again (and,
  // fixed, leave it in), so the refusal lifts with it.
  const seen = new Map<string, number>();
  await page.route('**/meurpg.rules.v1.TableContentService/ListTableEntries', async (route: Route) => {
    const real = await route.fetch();
    const body = (await real.json()) as Record<string, unknown>;
    const entries = (body.entries ?? []) as StoredEntry[];
    for (const e of entries) {
      seen.set(e.key, seen.get(e.key) ?? e.revision);
    }
    const archived = new Set(entries.filter((e) => e.archived).map((e) => e.key));
    const fixed = new Set(entries.filter((e) => e.revision !== seen.get(e.key)).map((e) => e.key));
    const live = violations.filter((v) => !archived.has(v.key) && !fixed.has(v.key));
    const headers = { ...real.headers() };
    delete headers['content-length'];
    delete headers['content-encoding'];
    for (const entry of entries) {
      if (live.some((v) => v.key === entry.key)) {
        stored[entry.key]?.(entry);
      }
    }
    await route.fulfill({ status: real.status(), headers, body: JSON.stringify({ ...body, leftOut: live.map((v) => ({ ...v, message: v.message ?? '' })) }) });
  });
  return () => page.unrouteAll({ behavior: 'wait' });
}

/** The folder the screenshots go to when `E2E_SHOTS_DIR` names one; otherwise nothing is written. */
const shotsDir = process.env.E2E_SHOTS_DIR;

/** The device scale of a context that takes screenshots: 2 when they are asked for, else the browser's own. */
export const leftOutShotScale = shotsDir ? { deviceScaleFactor: 2 } : {};

/** The viewport of each width: a phone, a tablet and a laptop. The screens are judged at the fold, never full page. */
export const LEFT_OUT_VIEWPORTS: Record<number, number> = { 390: 844, 768: 1024, 1280: 800 };

/** Saves the viewport as `736-<screen>-<width>-<claro|escuro>.png` when `E2E_SHOTS_DIR` is set; a normal run writes nothing. */
export async function shootLeftOut(page: Page, screen: string, width: number, scheme: 'light' | 'dark', at?: Locator): Promise<void> {
  if (!shotsDir) {
    return;
  }
  mkdirSync(shotsDir, { recursive: true });
  await page.setViewportSize({ width, height: LEFT_OUT_VIEWPORTS[width] ?? 800 });
  if (at) {
    await at.evaluate((el) => {
      el.scrollIntoView({ block: el.getBoundingClientRect().height < innerHeight * 0.8 ? 'center' : 'start' });
    });
  } else {
    await page.evaluate(() => window.scrollTo(0, 0));
  }
  await page.screenshot({ path: join(shotsDir, `736-${screen}-${width}-${scheme === 'light' ? 'claro' : 'escuro'}.png`) });
}
