/**
 * The campaign document's Markdown subset (MR-018), parsed to a token tree.
 *
 * The body is whatever the master typed, so it is never trusted: this parser
 * only ever produces the tokens below, and `MarkdownView` renders them with
 * Angular templates (text interpolation and property bindings, never
 * `innerHTML`). Anything the subset does not know (raw HTML, other link
 * schemes such as `javascript:`, external images, nested lists, tables)
 * stays as plain text, exactly as typed.
 *
 * What it understands (campaign_document.proto, README-B):
 * - headings `#`, `##` and `###` (`#` and `##` are both a level-2 section:
 *   the page's own title is the h1);
 * - paragraphs (consecutive lines join with a space);
 * - `**bold**` and `*italic*`, nested up to {@link MAX_INLINE_DEPTH} deep;
 * - unordered lists (`- ` or `* `) and ordered lists (`1. `), one level;
 * - `[text](https://...)`, an external link (https only);
 * - `[text](map:<uuid>)` and `[text](character:<uuid>)`, the app's own links;
 * - `![caption](image:<uuid>)` alone on its line, a gallery image.
 * - `\` before a punctuation mark writes it literally.
 *
 * The parser is linear for any input: link text and URLs have a length
 * limit, and a work budget turns the rest of a pathological text into plain
 * text instead of letting it run long.
 */

export type RefKind = 'map' | 'character';

export type Inline =
  | { readonly type: 'text'; readonly text: string }
  | { readonly type: 'bold'; readonly children: readonly Inline[] }
  | { readonly type: 'italic'; readonly children: readonly Inline[] }
  | { readonly type: 'link'; readonly href: string; readonly text: string }
  | { readonly type: 'ref'; readonly kind: RefKind; readonly id: string; readonly text: string };

export type Block =
  | { readonly type: 'heading'; readonly level: 2 | 3; readonly children: readonly Inline[] }
  | { readonly type: 'paragraph'; readonly children: readonly Inline[] }
  | {
      readonly type: 'list';
      readonly ordered: boolean;
      readonly start: number;
      readonly items: readonly (readonly Inline[])[];
    }
  | { readonly type: 'image'; readonly id: string; readonly caption: string };

/** How deep `**` and `*` nest; deeper markers stay as text. */
export const MAX_INLINE_DEPTH = 6;
/** The longest link text and URL the parser looks at. */
const MAX_LINK_TEXT = 500;
const MAX_LINK_URL = 2048;
/** Characters the inline scanner may look at in one document. */
const WORK_BUDGET = 4_000_000;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const PUNCTUATION = '\\`*_{}[]()#+-.!<>|~';

interface Budget {
  left: number;
}

/** Parses a document body into blocks. */
export function parseMarkdown(source: string): Block[] {
  const lines = source.replace(/\r\n?/g, '\n').split('\n');
  const budget: Budget = { left: WORK_BUDGET };
  const blocks: Block[] = [];
  let paragraph: string[] = [];
  let list: { ordered: boolean; start: number; items: Inline[][] } | null = null;

  const flushParagraph = () => {
    if (paragraph.length > 0) {
      blocks.push({
        type: 'paragraph',
        children: parseInline(paragraph.join(' '), budget),
      });
      paragraph = [];
    }
  };
  const flushList = () => {
    if (list) {
      blocks.push({ type: 'list', ordered: list.ordered, start: list.start, items: list.items });
      list = null;
    }
  };

  for (const raw of lines) {
    const line = raw.trimEnd();
    if (line.trim() === '') {
      flushParagraph();
      flushList();
      continue;
    }

    const heading = /^ {0,3}(#{1,3}) +(\S.*)$/.exec(line);
    if (heading) {
      flushParagraph();
      flushList();
      blocks.push({
        type: 'heading',
        level: heading[1].length === 3 ? 3 : 2,
        children: parseInline(heading[2].trim(), budget),
      });
      continue;
    }

    const image = /^ {0,3}!\[([^\]\n]{0,500})\]\(image:([^)\s]{1,64})\)$/.exec(line.trim());
    if (image && UUID.test(image[2])) {
      flushParagraph();
      flushList();
      blocks.push({ type: 'image', id: image[2].toLowerCase(), caption: image[1].trim() });
      continue;
    }

    const bullet = /^ {0,3}[-*] +(\S.*)$/.exec(line);
    const numbered = bullet ? null : /^ {0,3}(\d{1,9})[.)] +(\S.*)$/.exec(line);
    if (bullet || numbered) {
      flushParagraph();
      const ordered = numbered !== null;
      if (list && list.ordered !== ordered) {
        flushList();
      }
      list ??= { ordered, start: numbered ? Number(numbered[1]) : 1, items: [] };
      list.items.push(parseInline((bullet ? bullet[1] : numbered![2]).trim(), budget));
      continue;
    }

    // A plain line ends a list unless it is indented: then it continues the
    // last item.
    if (list && /^\s/.test(raw)) {
      // Appended in place: copying the item's tokens for every line would be
      // quadratic in the number of lines.
      const item = list.items[list.items.length - 1];
      for (const token of parseInline(' ' + line.trim(), budget)) {
        item.push(token);
      }
      continue;
    }
    flushList();
    paragraph.push(line.trim());
  }
  flushParagraph();
  flushList();
  return blocks;
}

/** Parses one run of text into inline tokens. */
export function parseInline(
  text: string,
  budget: Budget = { left: WORK_BUDGET },
  depth = 0,
): Inline[] {
  const out: Inline[] = [];
  let buffer = '';
  const pushText = (s: string) => {
    buffer += s;
  };
  const flush = () => {
    if (buffer !== '') {
      out.push({ type: 'text', text: buffer });
      buffer = '';
    }
  };

  let i = 0;
  while (i < text.length) {
    if (budget.left <= 0) {
      pushText(text.slice(i));
      break;
    }
    budget.left -= 1;
    const ch = text[i];

    if (ch === '\\' && i + 1 < text.length && PUNCTUATION.includes(text[i + 1])) {
      pushText(text[i + 1]);
      i += 2;
      continue;
    }

    // An image that is not a gallery image on its own line (an external
    // image, one inside a sentence) stays as text, "!" and all.
    if (ch === '!' && text[i + 1] === '[') {
      const image = readLink(text, i + 1, budget);
      if (image) {
        pushText(text.slice(i, image.end));
        i = image.end;
        continue;
      }
    }

    if (ch === '[') {
      const link = readLink(text, i, budget);
      if (link) {
        flush();
        out.push(link.token);
        i = link.end;
        continue;
      }
    }

    if (ch === '*' && depth < MAX_INLINE_DEPTH) {
      const bold = text.startsWith('**', i);
      const marker = bold ? '**' : '*';
      const close = findClosing(text, i + marker.length, marker, budget);
      if (close > i + marker.length) {
        const inner = text.slice(i + marker.length, close);
        // Spaces right inside the markers mean it is not emphasis ("2 * 3 *").
        if (inner.trim() === inner) {
          flush();
          const children = parseInline(inner, budget, depth + 1);
          out.push({ type: bold ? 'bold' : 'italic', children });
          i = close + marker.length;
          continue;
        }
      }
      pushText(marker);
      i += marker.length;
      continue;
    }

    pushText(ch);
    i += 1;
  }
  flush();
  return out;
}

/** The position of the next unescaped closing `marker` at or after `from`,
 * or -1. A single `*` skips over `**` pairs. */
function findClosing(text: string, from: number, marker: string, budget: Budget): number {
  for (let j = from; j < text.length; j += 1) {
    budget.left -= 1;
    if (budget.left <= 0) {
      return -1;
    }
    const c = text[j];
    if (c === '\\') {
      j += 1;
    } else if (c === '[') {
      // Do not close inside a link's brackets: skip to its "]" if close.
      const end = text.indexOf(']', j);
      if (end !== -1 && end - j <= MAX_LINK_TEXT) {
        j = end;
      }
    } else if (c === '*') {
      if (marker === '**') {
        if (text[j + 1] === '*') {
          return j;
        }
      } else if (text[j + 1] === '*') {
        j += 1; // a `**` pair inside: skip it whole
      } else {
        return j;
      }
    }
  }
  return -1;
}

/** `[text](target)` at `start`, if it is a link the subset knows. */
function readLink(
  text: string,
  start: number,
  budget: Budget,
): { token: Inline; end: number } | null {
  let j = start + 1;
  let label = '';
  for (; j < text.length && j - start <= MAX_LINK_TEXT + 1; j += 1) {
    budget.left -= 1;
    const c = text[j];
    if (c === '\\' && j + 1 < text.length && PUNCTUATION.includes(text[j + 1])) {
      label += text[j + 1];
      j += 1;
    } else if (c === ']') {
      break;
    } else if (c === '[') {
      return null;
    } else {
      label += c;
    }
  }
  if (text[j] !== ']' || text[j + 1] !== '(' || label.trim() === '') {
    return null;
  }
  const urlStart = j + 2;
  let k = urlStart;
  for (; k < text.length && k - urlStart <= MAX_LINK_URL; k += 1) {
    budget.left -= 1;
    const c = text[k];
    if (c === ')') {
      break;
    }
    if (/\s/.test(c) || c === '(') {
      return null;
    }
  }
  if (text[k] !== ')') {
    return null;
  }
  const target = text.slice(urlStart, k);
  const token = linkToken(label.trim(), target);
  return token ? { token, end: k + 1 } : null;
}

function linkToken(label: string, target: string): Inline | null {
  const colon = target.indexOf(':');
  const scheme = colon === -1 ? '' : target.slice(0, colon).toLowerCase();
  if (scheme === 'map' || scheme === 'character') {
    const id = target.slice(colon + 1);
    return UUID.test(id) ? { type: 'ref', kind: scheme, id: id.toLowerCase(), text: label } : null;
  }
  if (scheme === 'https') {
    const href = safeHttps(target);
    return href ? { type: 'link', href, text: label } : null;
  }
  return null;
}

/** The URL normalised by the browser, only if it is a plain https URL. */
export function safeHttps(target: string): string | null {
  try {
    const url = new URL(target);
    if (
      url.protocol !== 'https:' ||
      url.username !== '' ||
      url.password !== '' ||
      url.host === ''
    ) {
      return null;
    }
    return url.href;
  } catch {
    return null;
  }
}

/** A heading's plain text, for the table of contents. */
export function inlineText(children: readonly Inline[]): string {
  return children
    .map((c) => {
      switch (c.type) {
        case 'text':
          return c.text;
        case 'bold':
        case 'italic':
          return inlineText(c.children);
        default:
          return c.text;
      }
    })
    .join('');
}

/** The ids of the maps, sheets and images a document points to. */
export function collectReferences(blocks: readonly Block[]): {
  maps: Set<string>;
  characters: Set<string>;
  images: Set<string>;
} {
  const refs = {
    maps: new Set<string>(),
    characters: new Set<string>(),
    images: new Set<string>(),
  };
  const visit = (children: readonly Inline[]) => {
    for (const c of children) {
      if (c.type === 'ref') {
        (c.kind === 'map' ? refs.maps : refs.characters).add(c.id);
      } else if (c.type === 'bold' || c.type === 'italic') {
        visit(c.children);
      }
    }
  };
  for (const b of blocks) {
    if (b.type === 'image') {
      refs.images.add(b.id);
    } else if (b.type === 'list') {
      b.items.forEach(visit);
    } else {
      visit(b.children);
    }
  }
  return refs;
}

/** The headings the table of contents lists: the level-2 sections. */
export function outline(blocks: readonly Block[]): { index: number; text: string }[] {
  const out: { index: number; text: string }[] = [];
  blocks.forEach((b, index) => {
    if (b.type === 'heading' && b.level === 2) {
      const text = inlineText(b.children).trim();
      if (text !== '') {
        out.push({ index, text });
      }
    }
  });
  return out;
}
