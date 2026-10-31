import { combatRPC, openLive, step, waitForEncounterRevision, type Page } from './support';
import type { Page as AppPage } from './app-page';

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

// An awaited callback that runs after the call waits for the page itself.
export async function waitsInALaterStep(m: Page): Promise<void> {
  const enc = await combatRPC(m, 'EndTurn');
  await step('click', async () => {
    await waitForEncounterRevision(m, enc);
    await m.getByRole('button').click();
  });
}

// Not Playwright's `Page`: an app type with the same name is not a Playwright action.
export async function appTypeNamedPage(m: Page, results: AppPage): Promise<void> {
  await combatRPC(m, 'EndTurn');
  await results.click();
}

// A handler runs later, when the page calls it: what is inside it is not what follows the call that registers it.
export async function registersAHandler(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn');
  await m.route('**/rpc', async () => {
    await m.getByRole('button').click();
  });
  await m.waitForEvent('response', () => {
    void m.getByRole('button').click();
    return true;
  });
}

// A handler's own calls are a different moment from the test body.
export async function callsInsideAHandler(m: Page): Promise<void> {
  await m.route('**/rpc', async () => {
    await combatRPC(m, 'EndTurn');
  });
  await m.getByRole('button').click();
}

// A listener is not awaited: it does not run before what follows it.
export async function listens(m: Page): Promise<void> {
  m.on('response', async () => {
    await combatRPC(m, 'EndTurn');
  });
  await m.getByRole('button').click();
}

export async function exposesAFunction(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn');
  await m.exposeFunction('report', async () => {
    await m.getByRole('button').click();
  });
}

// A step nobody awaits does not run before what follows it.
export async function startsAStepWithoutAwaiting(m: Page): Promise<void> {
  void step('later', async () => {
    await combatRPC(m, 'EndTurn');
  });
  await m.getByRole('button').click();
}
