import { spawn } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

import { expect, type Browser, type BrowserContext, type Page, type TestInfo } from '@playwright/test';

import { PageDiagnostics, PROBE_DEADLINE_MS, test } from './diagnostics';
import { newSignedInContext } from './support';

// The support that explains a test stuck on a blank page (diagnostics.ts). These tests never touch the app: the pages
// are made here, with content set by hand, and made to crash, hang or fail on purpose.

// A page on an origin of its own, so the script can fetch with relative addresses; nothing leaves the browser.
async function serve(page: Page, html: string): Promise<void> {
  await page.route('http://diagnostics.test/', (route) => route.fulfill({ contentType: 'text/html; charset=utf-8', body: html }));
  await page.goto('http://diagnostics.test/');
}

async function openWatched(browser: Browser, diagnostics: PageDiagnostics) {
  const restore = diagnostics.watchBrowser(browser);
  const context = await browser.newContext();
  restore();
  return context;
}

test('uma página que responde aparece no relatório como respondendo', async ({ browser }) => {
  const diagnostics = new PageDiagnostics();
  const context = await openWatched(browser, diagnostics);
  try {
    const page = await context.newPage();
    await page.setContent('<p>pronto</p>');
    await diagnostics.probe();
    expect(diagnostics.report()).toMatch(/context 1, page 1 .*\n {2}state: answers \(readyState complete\)/);
  } finally {
    await context.close();
  }
});

test('uma página que travou diz que não responde e lista o pedido que ficou aberto', async ({ browser }) => {
  const diagnostics = new PageDiagnostics();
  const context = await openWatched(browser, diagnostics);
  try {
    const page = await context.newPage();
    await page.route('**/nunca-responde', () => {});
    await serve(page, '<script>fetch("/nunca-responde").catch(() => {});</script>');
    await expect.poll(() => diagnostics.report() ?? '').toContain('/nunca-responde');
    // The loop never ends: the evaluation below never settles, and only the probe's deadline ends the wait.
    let busyStarted!: () => void;
    const busy = new Promise<void>((resolve) => (busyStarted = resolve));
    await page.exposeFunction('busyStarted', () => busyStarted());
    void page.evaluate(() => {
      void (window as unknown as { busyStarted(): void }).busyStarted();
      for (;;) {
        // busy
      }
    }).catch(() => undefined);
    await busy;
    const started = Date.now();
    await diagnostics.probe();
    expect(Date.now() - started).toBeLessThan(PROBE_DEADLINE_MS * 4);
    const report = diagnostics.report() ?? '';
    expect(report).toContain('state: does not answer: no reply to a probe within');
    expect(report).toMatch(/requests still open \(\d+\):\n(?: {4}.*\n)* {4}GET \S+\/nunca-responde \(\d+ ms\)/);
  } finally {
    await context.close();
  }
});

test('uma página que caiu aparece como caiu, com a idade dos pedidos que ficaram abertos', async ({ browser }) => {
  const diagnostics = new PageDiagnostics();
  const context = await openWatched(browser, diagnostics);
  try {
    const page = await context.newPage();
    await page.route('**/nunca-responde', () => {});
    await serve(page, '<script>fetch("/nunca-responde").catch(() => {});</script>');
    await expect.poll(() => diagnostics.report() ?? '').toContain('/nunca-responde');
    const crashed = page.waitForEvent('crash');
    await page.goto('chrome://crash').catch(() => undefined);
    await crashed;
    await diagnostics.probe();
    const report = diagnostics.report() ?? '';
    expect(report).toContain('state: crashed');
    expect(report).toMatch(/GET \S+\/nunca-responde \(\d+ ms\)/);
  } finally {
    await context.close();
  }
});

test('o erro da página, o erro do console e o pedido que falhou entram no relatório', async ({ browser }) => {
  const diagnostics = new PageDiagnostics();
  const context = await openWatched(browser, diagnostics);
  try {
    const page = await context.newPage();
    await page.route('**/cai', (route) => route.abort());
    await serve(page, '<script>console.error("falhou no console"); setTimeout(() => { throw new Error("falhou na página"); }); fetch("/cai").catch(() => {});</script>');
    await expect.poll(() => diagnostics.report() ?? '').toContain('request failed: GET');
    await expect.poll(() => diagnostics.report() ?? '').toContain('pageerror: falhou na página');
    expect(diagnostics.report()).toContain('console.error: falhou no console');
  } finally {
    await context.close();
  }
});

test('o relatório só vai para o teste que falhou', async ({ browser }) => {
  const diagnostics = new PageDiagnostics();
  const context = await openWatched(browser, diagnostics);
  try {
    await (await context.newPage()).setContent('<p>pronto</p>');
    const attached: string[] = [];
    const testInfo = (status: string) => ({ status, attach: (name: string) => void attached.push(`${status}:${name}`) }) as unknown as TestInfo;
    for (const status of ['passed', 'interrupted', 'skipped']) await diagnostics.attachIfFailed(testInfo(status));
    expect(attached).toEqual([]);
    await diagnostics.attachIfFailed(testInfo('failed'));
    // Once per test, whichever fixture gets there first.
    await diagnostics.attachIfFailed(testInfo('failed'));
    expect(attached).toEqual(['failed:page-diagnostics']);
  } finally {
    await context.close();
  }
});

test('o teste vê as páginas do fixture, de browser.newContext e de newSignedInContext sem pedir nada', async ({ browser, page, pageDiagnostics }) => {
  await page.setContent('<p>do fixture</p>');
  const manual = await browser.newContext();
  const signedIn = await newSignedInContext(browser, 'Mestre Teste');
  try {
    await (await manual.newPage()).setContent('<p>manual</p>');
    await (await signedIn.newPage()).setContent('<p>com sessão</p>');
    const report = pageDiagnostics.report() ?? '';
    expect(report).toContain('context 1, page 1');
    expect(report).toContain('context 2, page 1');
    expect(report).toContain('context 3, page 1');
  } finally {
    await manual.close();
    await signedIn.close();
  }
});

// A page and a context that do what the report needs, so that a close that never finishes can be staged.
function fakeContext(pageClosed: () => boolean) {
  const page = { on: () => page, url: () => 'http://diagnostics.test/', isClosed: pageClosed, evaluate: () => Promise.resolve('complete'), context: () => context } as unknown as Page;
  const context = { pages: () => [page], on: () => context, close: () => new Promise<void>(() => {}) } as unknown as BrowserContext;
  return context;
}

test('um fechamento que não terminou: a página ainda aberta é sondada e responde, não "fechada pelo teste"', async () => {
  const diagnostics = new PageDiagnostics();
  const context = fakeContext(() => false);
  diagnostics.watchContext(context);
  void context.close();
  await diagnostics.probe();
  const report = diagnostics.report() ?? '';
  expect(report).toContain('state: answers (readyState complete)');
  expect(report).not.toContain('closed by the test');
});

test('uma página que de fato fechou aparece como fechada pelo teste, sem sonda', async () => {
  const diagnostics = new PageDiagnostics();
  let closed = false;
  const context = fakeContext(() => closed);
  diagnostics.watchContext(context);
  void context.close();
  closed = true;
  await diagnostics.probe();
  expect(diagnostics.report()).toContain('state: closed by the test, not probed');
});

test('uma página fechada com page.close() aparece como fechada, sem pedidos abertos', async ({ browser }) => {
  const diagnostics = new PageDiagnostics();
  const context = await openWatched(browser, diagnostics);
  try {
    const page = await context.newPage();
    await page.route('**/nunca-responde', () => {});
    await serve(page, '<script>fetch("/nunca-responde").catch(() => {});</script>');
    await expect.poll(() => diagnostics.report() ?? '').toContain('/nunca-responde');
    await page.close();
    await diagnostics.probe();
    const report = diagnostics.report() ?? '';
    expect(report).toContain('state: closed by the test, not probed');
    expect(report).not.toContain('requests still open');
    expect(report).not.toContain('/nunca-responde');
  } finally {
    await context.close();
  }
});

test('o console entrega os argumentos de cada mensagem: nenhum fica retido', async () => {
  // A fake page: the handler of 'console' must release the arguments of an error and of any other message.
  const handlers = new Map<string, (arg: unknown) => void>();
  const page = { on: (event: string, handler: (arg: unknown) => void) => handlers.set(event, handler) } as unknown as Page;
  new PageDiagnostics().watchPage(page, 'fake');
  const released: string[] = [];
  const handle = (name: string) => ({ dispose: () => (released.push(name), Promise.resolve()) });
  const message = (type: string, args: string[]) => ({ type: () => type, text: () => 'texto', location: () => ({ url: 'u', lineNumber: 1 }), args: () => args.map(handle) });
  handlers.get('console')?.(message('log', ['a', 'b']));
  handlers.get('console')?.(message('error', ['c']));
  expect(released).toEqual(['a', 'b', 'c']);
});

test('o browser volta ao protótipo: nenhuma camada de embrulho fica depois de cada teste', async ({ browser }) => {
  const prototype = Object.getPrototypeOf(browser) as { newContext: unknown; newPage: unknown };
  for (let i = 0; i < 3; i++) {
    const restore = new PageDiagnostics().watchBrowser(browser);
    expect(Object.hasOwn(browser, 'newContext')).toBe(true);
    restore();
  }
  expect(Object.hasOwn(browser, 'newContext')).toBe(false);
  expect(Object.hasOwn(browser, 'newPage')).toBe(false);
  expect(browser.newContext).toBe(prototype.newContext);
  expect(browser.newPage).toBe(prototype.newPage);
});

// Runs the Playwright CLI in its own process group and kills the group at the deadline, so a stalled run ends here with a
// message instead of holding the worker (a blocking spawn would also stop the test's own timeout from firing).
function runNested(args: string[], env: NodeJS.ProcessEnv, deadlineMs: number): Promise<{ stdout: string; stderr: string; status: number | null; timedOut: boolean }> {
  const e2e = path.join(__dirname, '..');
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [path.join(e2e, 'node_modules', '@playwright', 'test', 'cli.js'), ...args], { cwd: e2e, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    let timedOut = false;
    child.stdout.on('data', (chunk: Buffer) => (stdout += chunk.toString('utf8')));
    child.stderr.on('data', (chunk: Buffer) => (stderr += chunk.toString('utf8')));
    const timer = setTimeout(() => {
      timedOut = true;
      try {
        if (child.pid) process.kill(-child.pid, 'SIGKILL');
      } catch {
        // The group ended just before the deadline.
      }
    }, deadlineMs);
    child.on('error', (error) => {
      clearTimeout(timer);
      resolve({ stdout, stderr: `${stderr}${error.message}`, status: null, timedOut });
    });
    child.on('close', (status) => {
      clearTimeout(timer);
      resolve({ stdout, stderr, status, timedOut });
    });
  });
}

test('um teste que falha anexa o diagnóstico: o pedido que não voltou no fixture e a página que caiu num contexto fechado pelo teste', async () => {
  // A nested run of diagnostics-fixture, whose tests fail on purpose (the attachment is made after the test body,
  // so only a real failing test shows it). Three failures of a few seconds each (the third runs out two 5 s limits), hence the longer timeout.
  test.setTimeout(90_000);
  const { PLAYWRIGHT_JSON_OUTPUT_NAME: _name, ...env } = process.env;
  // One folder for each run: the repeats of this test run side by side, and a run empties its output folder.
  const output = mkdtempSync(path.join(tmpdir(), 'meurpg-diagnostics-'));
  let run: Awaited<ReturnType<typeof runNested>>;
  try {
    run = await runNested(['test', '-c', 'diagnostics-fixture/playwright.config.ts', '--reporter=json', `--output=${output}`], env, 70_000);
  } finally {
    rmSync(output, { recursive: true, force: true });
  }
  expect(run.timedOut, `the nested run did not end in 70 s: ${run.stderr}`).toBe(false);
  type Result = { status: string; attachments: { name: string; body?: string }[] };
  type Suite = { specs?: { title: string; tests: { results: Result[] }[] }[]; suites?: Suite[] };
  const results = new Map<string, Result>();
  const walk = (suite: Suite) => {
    for (const spec of suite.specs ?? []) results.set(spec.title, spec.tests[0].results[0]);
    suite.suites?.forEach(walk);
  };
  expect(run.stdout, `the nested run printed no report (status ${run.status}): ${run.stderr}`).toContain('"suites"');
  walk({ suites: (JSON.parse(run.stdout) as { suites: Suite[] }).suites });
  const attachment = (title: string) => {
    const result = results.get(title);
    expect(result?.status, `${title} must fail`).toMatch(/failed|timedOut/);
    const found = result?.attachments.find((a) => a.name === 'page-diagnostics');
    expect(found, `${title} must attach page-diagnostics`).toBeDefined();
    return Buffer.from(found?.body ?? '', 'base64').toString('utf8');
  };
  const waiting = attachment('a click whose answer never arrives');
  expect(waiting).toContain('state: answers');
  expect(waiting).toMatch(/requests still open \(1\):\n {4}GET http:\/\/diagnostics.test\/never/);
  expect(attachment('a page that crashed in a context the test closes itself')).toContain('state: crashed');
  // The limit the docs name: Playwright's own screenshot and trace of a failed test wait for a page in a loop until
  // their 5 s run out, and then the fixtures end without the attachment.
  const stuck = results.get('a page stuck in a loop that never ends');
  expect(stuck?.status).toMatch(/failed|timedOut/);
  expect(stuck?.attachments.map((a) => a.name)).not.toContain('page-diagnostics');
});
