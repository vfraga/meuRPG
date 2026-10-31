import {
  combatRPC,
  getEncounterRPC,
  rollSceneRPC,
  step,
  waitForEncounterRevision,
  type ElementHandle,
  type Locator,
  type Page,
} from './support';

// Errors, one per function that is not under "Not errors": a click straight after the call, a click in an outer block,
// a wait that comes too late, a wait on the master's page before a click on the player's, a call that ends a callback,
// a click inside a later awaited callback, and a receiver read from its type (frame, handle, intersection, generic, union,
// keyboard, mouse, a page that may be undefined).
export async function clickRightAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.getByRole('button').click();
}

export async function clickInOuterBlock(m: Page, ok: boolean): Promise<void> {
  if (ok) {
    await combatRPC(m, 'EndTurn'); // error
  }
  await m.getByRole('button').click();
}

export async function waitTooLate(m: Page): Promise<void> {
  const enc = await combatRPC(m, 'EndTurn'); // error
  await m.getByRole('button').click();
  await waitForEncounterRevision(m, enc);
}

export async function waitsOnTheOtherPage(m: Page, p: Page): Promise<void> {
  const enc = await combatRPC(m, 'EndTurn'); // error
  await waitForEncounterRevision(m, enc);
  await p.getByRole('button').click();
}

// Not errors: a read, a void call (no revision to wait for), a load after the call.
export async function notTracked(m: Page): Promise<void> {
  await getEncounterRPC(m);
  await m.getByRole('button').click();
  await rollSceneRPC(m);
  await m.getByRole('button').click();
  await combatRPC(m, 'EndTurn');
  await m.goto('/');
  await m.getByRole('button').click();
  // A move is patched into the page with no new revision: there is nothing to wait for.
  await combatRPC(m, 'MoveCombatant');
  await m.getByRole('button').click();
}

// An error: the call ends a callback's body and a click follows the callback.
export async function endsACallback(m: Page): Promise<void> {
  await step('end the turn', async () => {
    await combatRPC(m, 'EndTurn'); // error
  });
  await m.getByRole('button').click();
}

// An error: the click is in an awaited callback that runs right after the call.
export async function actsInALaterStep(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await step('click', async () => {
    await m.getByRole('button').click();
  });
}

// Errors: the receiver is read from its type. A frame, an element handle, an intersection, a generic that extends a
// locator, a union with a locator, the keyboard and the mouse are all page actions.
export async function actsOnAFrame(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.frame().click('button');
}

export async function actsOnAHandle(m: Page, handle: ElementHandle): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await handle.click();
}

export async function actsOnAnIntersection(m: Page, box: Locator & { tag: string }): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await box.click();
}

export async function actsOnAGeneric<T extends Locator>(m: Page, target: T): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await target.click();
}

export async function actsOnAUnion(m: Page, target: Locator | Set<string>): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await target.clear();
}

export async function pressesAKey(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.keyboard.press('Enter');
}

export async function clicksTheMouse(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.mouse.click(1, 2);
}

// An error: the page may be undefined, and a wait on the master's page does not cover it.
export async function waitsOnTheOtherPageOfAUnion(m: Page, p: Page | undefined): Promise<void> {
  const enc = await combatRPC(m, 'EndTurn'); // error
  await waitForEncounterRevision(m, enc);
  await p!.getByRole('button').click();
}

// Errors: each of these is a page action too.
export async function keyboardDownAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.keyboard.down('a');
}

export async function keyboardUpAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.keyboard.up('a');
}

export async function keyboardInsertTextAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.keyboard.insertText('a');
}

export async function mouseDownAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.mouse.down();
}

export async function mouseUpAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.mouse.up();
}

export async function mouseMoveAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.mouse.move(1, 2);
}

export async function mouseWheelAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.mouse.wheel(0, 9);
}

export async function dragAndDropAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.dragAndDrop('a', 'b');
}

export async function dispatchEventAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.getByRole('button').dispatchEvent('click');
}

export async function selectTextAfter(m: Page): Promise<void> {
  await combatRPC(m, 'EndTurn'); // error
  await m.getByRole('button').selectText();
}
