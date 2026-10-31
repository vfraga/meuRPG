import { combatRPC, openLive, step, passTurnsToAndWait, waitForEncounterRevision, type Page } from './support';

export async function clickRightAfter(m: Page): Promise<void> {
  const enc = await combatRPC(m, 'EndTurn');
  await waitForEncounterRevision(m, enc);
  await m.getByRole('button').click();
}

export async function clickInOuterBlock(m: Page, ok: boolean): Promise<void> {
  let enc = await combatRPC(m, 'Start');
  if (ok) {
    enc = await combatRPC(m, 'EndTurn');
  }
  await waitForEncounterRevision(m, enc);
  await m.getByRole('button').click();
}

// Several calls in a row need one wait, after the last.
export async function waitTooLate(m: Page): Promise<void> {
  await combatRPC(m, 'Start');
  const enc = await combatRPC(m, 'EndTurn');
  await waitForEncounterRevision(m, enc);
  await m.getByRole('button').click();
}

// The page the next action uses is the page waited on; when both act, both are waited on.
export async function waitsOnEachPage(m: Page, p: Page): Promise<void> {
  const enc = await combatRPC(m, 'EndTurn');
  await waitForEncounterRevision([m, p], enc);
  await m.getByRole('button').click();
  await p.getByRole('button').click();
}

// Not an action: `clear` of a Set is not a Playwright action.
export async function setClear(m: Page): Promise<void> {
  const seen = new Set<string>();
  const enc = await combatRPC(m, 'EndTurn');
  seen.clear();
  await waitForEncounterRevision(m, enc);
}

// A page load after the call reads the state itself.
export async function loadsTheLivePage(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn');
  await openLive(m, '/session');
  await m.getByRole('button').click();
}

// The wait after the callback covers the call that ends it.
export async function waitsAfterACallback(m: Page): Promise<void> {
  let enc = await combatRPC(m, 'Start');
  await step('end the turn', async () => {
    enc = await combatRPC(m, 'EndTurn');
  });
  await waitForEncounterRevision(m, enc);
  await m.getByRole('button').click();
}

// The helper waits on the pages in its last argument.
export async function passTurnsWaitsOnBothPages(m: Page, p: Page): Promise<void> {
  await combatRPC(m, 'EndTurn');
  await passTurnsToAndWait(m, 'c', 'Sombra', [m, p]);
  await m.getByRole('button').click();
  await p.getByRole('button').click();
}
