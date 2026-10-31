import type { Page } from './support';

// One error per line marked `// error`: a plain string that is part of a campaign name ("Nomes e legenda", "Mesa de teste").
export async function labelOfTheForm(m: Page): Promise<void> {
  await m.getByLabel('Nome').click(); // error
}

export async function nameGivenToAHelper(m: Page): Promise<void> {
  await m.getByLabel('Pontes').click(); // error
}

export async function linkOfTheBar(m: Page): Promise<void> {
  await m.getByRole('link', { name: 'legenda' }).click(); // error
}

export async function labelInUpperCase(m: Page): Promise<void> {
  await m.getByLabel('NOMES').click(); // error
}

export async function aPageOrNothing(m: Page | undefined): Promise<void> {
  await m!.getByLabel('Nome').click(); // error
}
