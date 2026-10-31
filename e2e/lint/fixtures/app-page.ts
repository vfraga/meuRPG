// An app's own type that happens to be called `Page` (a page of results): it has `click`, and is no Playwright page.
export interface Page {
  click(): Promise<void>;
}
