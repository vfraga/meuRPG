import fs from 'node:fs';
import path from 'node:path';

import { expect } from '@playwright/test';
import { test } from './diagnostics';

// CC-BY-4.0 compliance: the repo root's NOTICE carries the exact SRD 5.1
// attribution text (ADR-0008, private), and the public /credits page
// (web/src/app/pages/credits) shows the same text, hardcoded from the same
// source. This test reads NOTICE at runtime instead of hardcoding the
// attribution itself, so it can never drift from what NOTICE actually says,
// and fails if NOTICE is ever missing: a skipped test would hide it.
const noticePath = path.resolve(__dirname, '..', '..', 'NOTICE');

/**
 * NOTICE's attribution paragraph: the exact CC-BY block ADR-0008 fixes, one
 * paragraph starting with "This work includes material...". NOTICE's own
 * section heading ("System Reference Document 5.1\n----...") also mentions
 * the SRD by name, so this matches on the attribution's own opening words
 * instead, which only the real paragraph has.
 */
function readAttributionParagraph(): string {
  const text = fs.readFileSync(noticePath, 'utf-8');
  const paragraph = text.split(/\n\s*\n/).find((p) => p.includes('This work includes material'));
  if (!paragraph) {
    throw new Error(`NOTICE exists but no paragraph starts with the CC-BY attribution: ${noticePath}`);
  }
  return paragraph.replace(/\s+/g, ' ').trim();
}

test('a página de créditos mostra a atribuição do SRD 5.1', { tag: '@licenca' }, async ({ page }) => {
  const attribution = readAttributionParagraph();

  await page.goto('/credits');
  // A web-first assertion, which waits for the page to render the text (and
  // collapses whitespace, like readAttributionParagraph): the app renders
  // /credits after `goto` returns, so a one-shot read of the body could
  // catch only the header (seen locally, 30/09/2026).
  await expect(page.locator('body')).toContainText(attribution);
});

// The SRD 5.2.1 line (CC BY 4.0): the three tables of the 2024 rules the app uses are credited with the exact
// paragraph of NOTICE, which this test reads at runtime, like the 5.1 one above.
test('a página de créditos mostra a atribuição do SRD 5.2.1 e o rótulo das regras de 2024', { tag: '@licenca' }, async ({ page }) => {
  const text = fs.readFileSync(noticePath, 'utf-8');
  const paragraph = text.split(/\n\s*\n/).find((p) => p.includes('This work includes material from the System Reference Document 5.2.1'));
  if (!paragraph) {
    throw new Error(`NOTICE has no SRD 5.2.1 attribution paragraph: ${noticePath}`);
  }
  await page.goto('/credits');
  await expect(page.locator('body')).toContainText(paragraph.replace(/\s+/g, ' ').trim());
  await expect(page.locator('body')).toContainText('SRD 5.2.1 (regras de 2024)');
});

// The 2024 table of the encounter builder (MR-043) is named on the page, with its page of the SRD 5.2.1 (10.17b).
test('a página de créditos nomeia a tabela de orçamento de XP dos encontros (p. 201 do SRD 5.2.1)', { tag: ['@licenca', '@MR-043'] }, async ({ page }) => {
  await page.goto('/credits');
  await expect(page.locator('body')).toContainText('“XP Budget per Character” (p. 201)');
  await expect(page.locator('body')).toContainText('o orçamento de XP de cada dificuldade nos encontros');
});
