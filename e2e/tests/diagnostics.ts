import { test as base, type Browser, type BrowserContext, type ConsoleMessage, type Page, type Request, type TestInfo } from '@playwright/test';

/**
 * The `test` every spec imports (the lint rejects `test` from '@playwright/test'). It adds an automatic fixture that
 * watches every page of every context the test opens (the `page` and `context` fixtures, `browser.newContext()`,
 * `newSignedInContext` and the other helpers that call it) and, when the test fails, attaches `page-diagnostics`: what
 * each page reported (a crash, `pageerror`, console errors, failed requests), whether it still answers, and which
 * requests the browser still has open (docs/testing.md, "A page that hangs").
 *
 * The probe is an evaluation, which Playwright records as a step of the test and which can wait for a stuck page: it
 * runs only after the test failed, in the `context` fixture, which ends before Playwright closes the contexts (a stuck
 * page makes that close overrun the teardown, and the attachment would be lost). A context the test closed itself has
 * no page left to ask; for its pages the report keeps what the browser had seen and the requests open at the close.
 * A page in a CPU loop is out of reach: Playwright's own screenshot and trace of the failed test wait for it, and the
 * test ends without the attachment.
 */

/** How long a page gets to answer the probe; the probe never keeps a test or its teardown waiting longer. */
export const PROBE_DEADLINE_MS = 1_500;

// A page that fails in a loop must not fill the report.
const MAX_LINES = 40;
const MAX_TEXT = 400;

type OpenRequest = { description: string; since: number };

type Probe = { answers: string; openRequests: string[] };

type Watched = {
  label: string;
  /** The requests open when the test called `context.close()`, kept without asking the page anything. */
  closedWith?: string[];
  page: Page;
  crashed: boolean;
  errors: string[];
  failedRequests: string[];
  open: Map<Request, OpenRequest>;
  probe?: Probe;
};

function clip(text: string): string {
  return text.length > MAX_TEXT ? `${text.slice(0, MAX_TEXT)}…` : text;
}

/** One line for each request still open, with how long it has been open. */
function openRequestLines(open: Iterable<OpenRequest>, now: number): string[] {
  return [...open].map((request) => `${request.description} (${now - request.since} ms)`);
}

function firstLine(error: unknown): string {
  const text = error instanceof Error ? error.message : String(error);
  return clip(text.split('\n')[0] ?? '');
}

/** What the probe of one page found, one block of the report. */
function describe(watched: Watched, now: number): string[] {
  const { page, label, probe } = watched;
  const lines = [`${label}  ${page.url() || '(no url yet)'}`];
  // A close the test asked for may not have finished: a page that is still open was probed and says what it answered.
  const closed = page.isClosed();
  const state = watched.crashed ? 'crashed' : probe ? probe.answers : closed ? 'closed by the test, not probed' : 'not probed';
  lines.push(`  state: ${state}`);
  for (const error of watched.errors) lines.push(`  ${error}`);
  for (const failed of watched.failedRequests) lines.push(`  ${failed}`);
  const open = probe?.openRequests ?? (closed ? watched.closedWith : undefined) ?? openRequestLines(watched.open.values(), now);
  if (open.length > 0) {
    lines.push(`  requests still open (${open.length}):`);
    for (const request of open.slice(0, MAX_LINES)) lines.push(`    ${request}`);
  }
  return lines;
}

export class PageDiagnostics {
  private readonly pages: Watched[] = [];
  private readonly contexts = new WeakSet<BrowserContext>();
  private attached = false;
  private contextCount = 0;

  /** Watches the pages the context has and the ones it opens later, and probes its pages before it closes. */
  watchContext(context: BrowserContext): void {
    if (this.contexts.has(context)) return;
    this.contexts.add(context);
    const name = `context ${++this.contextCount}`;
    let pageCount = 0;
    const watch = (page: Page) => this.watchPage(page, `${name}, page ${++pageCount}`);
    context.pages().forEach(watch);
    context.on('page', watch);
    // The test body usually closes its contexts itself, in a `finally`, before anything knows that it failed.
    // Nothing is asked of the pages there: only what the browser already told is kept.
    const close = context.close.bind(context);
    context.close = (options) => {
      const now = Date.now();
      for (const watched of this.pages) {
        if (watched.page.context() === context && !watched.page.isClosed()) {
          watched.closedWith = openRequestLines(watched.open.values(), now);
        }
      }
      return close(options);
    };
  }

  watchPage(page: Page, label: string): void {
    const watched: Watched = { label, page, crashed: false, errors: [], failedRequests: [], open: new Map() };
    this.pages.push(watched);
    const note = (list: string[], line: string) => {
      if (list.length < MAX_LINES) list.push(clip(line));
    };
    page.on('crash', () => {
      watched.crashed = true;
    });
    page.on('pageerror', (error) => note(watched.errors, `pageerror: ${firstLine(error)}`));
    page.on('console', (message: ConsoleMessage) => {
      if (message.type() === 'error') note(watched.errors, `console.error: ${message.text()} (${message.location().url}:${message.location().lineNumber})`);
      // Playwright hands every message's arguments to Node as handles that stay alive until they are released; only
      // the text of an error is kept, so none of them is needed.
      for (const arg of message.args()) void arg.dispose().catch(() => undefined);
    });
    page.on('close', () => {
      // A page that closed has no request left to wait for, so none is listed (their ages would only grow until the
      // report). A context closed by the test already kept its list, and a page that crashed keeps what it left open.
      watched.closedWith ??= watched.crashed ? openRequestLines(watched.open.values(), Date.now()) : [];
      watched.open.clear();
    });
    page.on('request', (request) => watched.open.set(request, { description: `${request.method()} ${request.url()}`, since: Date.now() }));
    page.on('requestfinished', (request) => watched.open.delete(request));
    page.on('requestfailed', (request) => {
      watched.open.delete(request);
      note(watched.failedRequests, `request failed: ${request.method()} ${request.url()}: ${request.failure()?.errorText ?? 'unknown'}`);
    });
  }

  /** Watches what the browser opens from now on, until the returned function puts it back. */
  watchBrowser(browser: Browser): () => void {
    const newContext = browser.newContext.bind(browser);
    const newPage = browser.newPage.bind(browser);
    browser.newContext = async (options) => {
      const context = await newContext(options);
      this.watchContext(context);
      return context;
    };
    browser.newPage = async (options) => {
      const page = await newPage(options);
      this.watchContext(page.context());
      return page;
    };
    // The browser is the worker's, shared by its tests: the own properties go, and the prototype's methods are back.
    return () => {
      delete (browser as Partial<Browser>).newContext;
      delete (browser as Partial<Browser>).newPage;
    };
  }

  /**
   * Asks every open page that was not probed yet whether it answers. Each probe has its own deadline and they run
   * together, so this never takes longer than the deadline; a probe that never returns is abandoned.
   */
  async probe(watched: Watched[] = this.pages): Promise<void> {
    await Promise.all(
      watched.map(async (entry) => {
        if (entry.probe || entry.page.isClosed()) return;
        // A crashed page has nothing to answer with.
        if (entry.crashed) {
          entry.probe = { answers: 'crashed', openRequests: openRequestLines(entry.open.values(), Date.now()) };
          return;
        }
        const started = Date.now();
        entry.probe = { answers: 'probe running', openRequests: [] };
        let timer: NodeJS.Timeout | undefined;
        const deadline = new Promise<string>((resolve) => {
          timer = setTimeout(() => resolve(`does not answer: no reply to a probe within ${PROBE_DEADLINE_MS} ms`), PROBE_DEADLINE_MS);
        });
        // The evaluation of a page that is stuck never settles: it is left behind, and its late rejection is read here.
        const answer = entry.page.evaluate(() => document.readyState).then(
          (readyState) => `answers (readyState ${readyState}) in ${Date.now() - started} ms`,
          (error: unknown) => `does not answer: ${firstLine(error)}`,
        );
        const answers = await Promise.race([answer, deadline]);
        clearTimeout(timer);
        entry.probe = { answers, openRequests: openRequestLines(entry.open.values(), Date.now()) };
      }),
    );
  }

  /** The text of the attachment, or undefined when the test opened no page. */
  report(): string | undefined {
    if (this.pages.length === 0) return undefined;
    const now = Date.now();
    const blocks = this.pages.map((watched) => describe(watched, now).join('\n'));
    return ['Pages the test opened, as the browser saw them when it failed.', '', ...blocks].join('\n') + '\n';
  }

  /** Probes what is still open and attaches the report when the test failed. */
  async attachIfFailed(testInfo: TestInfo): Promise<void> {
    if (this.attached || (testInfo.status !== 'failed' && testInfo.status !== 'timedOut')) return;
    this.attached = true;
    await this.probe();
    const body = this.report();
    if (body) await testInfo.attach('page-diagnostics', { body, contentType: 'text/plain' });
  }
}

export const test = base.extend<{ pageDiagnostics: PageDiagnostics }>({
  // Ends before Playwright closes the context, which is where the report has to be made (see the top of this file).
  context: async ({ context, pageDiagnostics }, use, testInfo) => {
    // Playwright builds this context without `browser.newContext()` when it reuses contexts (UI mode, `reuseContext`).
    pageDiagnostics.watchContext(context);
    await use(context);
    await pageDiagnostics.attachIfFailed(testInfo);
  },
  pageDiagnostics: [
    async ({ browser }, use, testInfo) => {
      const diagnostics = new PageDiagnostics();
      const restore = diagnostics.watchBrowser(browser);
      try {
        await use(diagnostics);
      } finally {
        restore();
        // The test used no `context` or `page` fixture: its own contexts are all that was watched.
        await diagnostics.attachIfFailed(testInfo);
      }
    },
    { auto: true },
  ],
});
