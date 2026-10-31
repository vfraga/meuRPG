import type { Page } from './support';
import type { Locator } from './playwright-core/types';

export async function exactLabel(m: Page): Promise<void> {
  await m.getByLabel('Nome', { exact: true }).click();
}

export async function exactLink(m: Page): Promise<void> {
  await m.getByRole('link', { name: 'legenda', exact: true }).click();
}

// A string no campaign name holds.
export async function otherText(m: Page): Promise<void> {
  await m.getByLabel('Sua resposta').click();
}

// A locator scoped to an element cannot see the app bar.
export async function scoped(dialog: Locator): Promise<void> {
  await dialog.getByLabel('Nome').click();
}

// Other roles carry no campaign name.
export async function button(m: Page): Promise<void> {
  await m.getByRole('button', { name: 'Nome' }).click();
}
