import {
  type Block,
  type Inline,
  collectReferences,
  inlineText,
  outline,
  parseInline,
  parseMarkdown,
  safeHttps,
} from './markdown';

const MAP = '11111111-2222-4333-8444-555555555555';
const CHAR = 'aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee';
const IMG = '99999999-8888-4777-8666-555555555555';

const text = (t: string): Inline => ({ type: 'text', text: t });

describe('parseMarkdown', () => {
  it('reads headings, paragraphs and joins consecutive lines', () => {
    const blocks = parseMarkdown('# A\n## B\n### C\n\nPrimeira\nsegunda\n\nOutra');
    expect(blocks).toEqual([
      { type: 'heading', level: 2, children: [text('A')] },
      { type: 'heading', level: 2, children: [text('B')] },
      { type: 'heading', level: 3, children: [text('C')] },
      { type: 'paragraph', children: [text('Primeira segunda')] },
      { type: 'paragraph', children: [text('Outra')] },
    ]);
  });

  it('does not make a heading of #### or of a # without a space', () => {
    expect(parseMarkdown('#### Quatro')[0].type).toBe('paragraph');
    expect(parseMarkdown('#semespaço')[0].type).toBe('paragraph');
  });

  it('reads bold, italic and nesting', () => {
    expect(parseInline('a **b *c* d** e *f*')).toEqual([
      text('a '),
      {
        type: 'bold',
        children: [text('b '), { type: 'italic', children: [text('c')] }, text(' d')],
      },
      text(' e '),
      { type: 'italic', children: [text('f')] },
    ]);
  });

  it('keeps unbalanced and spaced markers as text', () => {
    expect(inlineText(parseInline('**sem fim'))).toBe('**sem fim');
    expect(inlineText(parseInline('2 * 3 * 4'))).toBe('2 * 3 * 4');
    expect(inlineText(parseInline('*a'))).toBe('*a');
    expect(parseInline('** a **')).toEqual([text('** a **')]);
  });

  it('writes an escaped marker literally', () => {
    expect(parseInline('\\*não\\* \\[x\\](https://a.b)')).toEqual([text('*não* [x](https://a.b)')]);
  });

  it('reads unordered and ordered lists, with the first number', () => {
    const blocks = parseMarkdown('- um\n- dois **forte**\n\n3. três\n4. quatro');
    expect(blocks).toEqual([
      {
        type: 'list',
        ordered: false,
        start: 1,
        items: [[text('um')], [text('dois '), { type: 'bold', children: [text('forte')] }]],
      },
      { type: 'list', ordered: true, start: 3, items: [[text('três')], [text('quatro')]] },
    ]);
  });

  it('continues a list item on an indented line and ends it on a plain one', () => {
    const blocks = parseMarkdown('- um\n  continua\nfim');
    expect(blocks).toHaveLength(2);
    expect(inlineText((blocks[0] as Extract<Block, { type: 'list' }>).items[0])).toBe(
      'um continua',
    );
    expect(blocks[1].type).toBe('paragraph');
  });

  it('reads the three links of the app and https links', () => {
    const [p] = parseMarkdown(
      `Veja [Mirathel](map:${MAP}), [Capitão](character:${CHAR}) e [o SRD](https://example.com/srd?a=1).`,
    );
    const kinds = (p as Extract<Block, { type: 'paragraph' }>).children.map((c) => c.type);
    expect(kinds).toEqual(['text', 'ref', 'text', 'ref', 'text', 'link', 'text']);
    const refs = (p as Extract<Block, { type: 'paragraph' }>).children.filter(
      (c) => c.type === 'ref',
    );
    expect(refs).toEqual([
      { type: 'ref', kind: 'map', id: MAP, text: 'Mirathel' },
      { type: 'ref', kind: 'character', id: CHAR, text: 'Capitão' },
    ]);
  });

  it('reads an image alone on its line, with its caption', () => {
    const [img] = parseMarkdown(`![A Taverna, onde Odra espera.](image:${IMG})`);
    expect(img).toEqual({ type: 'image', id: IMG, caption: 'A Taverna, onde Odra espera.' });
  });

  it('keeps an image inside a sentence, a bad id and an external image as text', () => {
    for (const src of [
      `Veja ![x](image:${IMG}) aqui`,
      '![x](image:nao-e-uuid)',
      '![x](https://example.com/a.png)',
    ]) {
      const [b] = parseMarkdown(src);
      expect(b.type).toBe('paragraph');
      expect(JSON.stringify(b)).not.toContain('"image"');
      expect(inlineText((b as Extract<Block, { type: 'paragraph' }>).children)).toBe(src);
    }
  });

  it('counts the bytes of nothing: an empty or blank body has no blocks', () => {
    expect(parseMarkdown('')).toEqual([]);
    expect(parseMarkdown('\n \n\t\n')).toEqual([]);
  });

  it('accepts Windows and old Mac line breaks', () => {
    expect(parseMarkdown('# A\r\n\r\nB\rC')).toHaveLength(2);
  });
});

describe('hostile input', () => {
  const allText = (blocks: Block[]) =>
    blocks.every((b) => b.type === 'paragraph' && b.children.every((c) => c.type === 'text'));

  it('leaves raw HTML as plain text, tags and all', () => {
    const src = '<script>alert(1)</script> <img src=x onerror=alert(1)> <b>x</b>';
    const blocks = parseMarkdown(src);
    expect(allText(blocks)).toBe(true);
    expect(inlineText((blocks[0] as Extract<Block, { type: 'paragraph' }>).children)).toBe(src);
  });

  it('leaves links with other schemes as their literal source', () => {
    for (const target of [
      'javascript:alert(1)',
      'JaVaScRiPt:alert(1)',
      'data:text/html;base64,AAAA',
      'vbscript:x',
      'http://example.com',
      'ftp://example.com',
      'file:///etc/passwd',
      '//example.com',
      '/relative',
      'map:nao-uuid',
      'character:',
      'https://user:pass@example.com',
      'https://',
    ]) {
      const src = `[clique](${target})`;
      const [b] = parseMarkdown(src);
      expect(b.type).toBe('paragraph');
      const children = (b as Extract<Block, { type: 'paragraph' }>).children;
      expect(
        children.every((c) => c.type === 'text'),
        target,
      ).toBe(true);
      expect(inlineText(children)).toBe(src);
    }
  });

  it('never produces an href that is not https', () => {
    for (const href of ['https://a.b/c', 'https://a.b/"onmouseover="x']) {
      const out = parseInline(`[t](${href})`);
      for (const c of out) {
        if (c.type === 'link') {
          expect(c.href.startsWith('https://')).toBe(true);
        }
      }
    }
    expect(safeHttps('javascript:alert(1)')).toBeNull();
    expect(safeHttps('https://a.b/x y')).not.toBeNull(); // the browser encodes the space
  });

  it('survives nested and unbalanced markers', () => {
    const src = '*'.repeat(200) + 'x' + '*'.repeat(201) + ' **a *b **c *d ' + '[['.repeat(100);
    expect(() => parseMarkdown(src)).not.toThrow();
    const nested = '**a *b **c *d '.repeat(50) + 'z' + ' d* c** b* a**'.repeat(50);
    expect(() => parseMarkdown(nested)).not.toThrow();
  });

  it('limits how deep it nests', () => {
    const src = '*a '.repeat(0) + '**'.repeat(1) + '*x*'.repeat(1);
    expect(parseInline(src).length).toBeGreaterThan(0);
    let deep = 'x';
    for (let i = 0; i < 2000; i += 1) {
      deep = i % 2 === 0 ? `**${deep}**` : `*${deep}*`;
    }
    const out = parseInline(deep);
    let depth = 0;
    let cur: readonly Inline[] = out;
    while (cur.length === 1 && (cur[0].type === 'bold' || cur[0].type === 'italic')) {
      depth += 1;
      cur = cur[0].children;
    }
    expect(depth).toBeLessThanOrEqual(7);
  });

  it('parses 200 KB of pathological text quickly', () => {
    const cases = [
      '['.repeat(204_800),
      '[a]('.repeat(51_200),
      '*'.repeat(204_800),
      '**a '.repeat(51_200),
      '*a **b '.repeat(29_000),
      '[x](https://a.b/' + 'a'.repeat(204_000),
      ('![x](image:' + 'a'.repeat(30) + ')\n').repeat(6_000),
      '- '.repeat(100_000),
      ('# ' + 'h'.repeat(60) + '\n').repeat(3_000),
    ];
    for (const src of cases) {
      const t0 = performance.now();
      parseMarkdown(src);
      expect(performance.now() - t0).toBeLessThan(1500);
    }
  });

  it('keeps a very long line in one piece', () => {
    const long = 'palavra '.repeat(30_000);
    const [b] = parseMarkdown(long);
    expect(inlineText((b as Extract<Block, { type: 'paragraph' }>).children)).toBe(long.trimEnd());
  });
});

describe('collectReferences and outline', () => {
  it('lists the maps, sheets and images a document points to, once each', () => {
    const blocks = parseMarkdown(
      `# Título\n\n[a](map:${MAP}) e **[b](character:${CHAR})** de novo [a](map:${MAP}).\n\n- [c](map:${MAP})\n\n![x](image:${IMG})`,
    );
    const refs = collectReferences(blocks);
    expect([...refs.maps]).toEqual([MAP]);
    expect([...refs.characters]).toEqual([CHAR]);
    expect([...refs.images]).toEqual([IMG]);
  });

  it('builds the outline from the level-2 headings, by block index', () => {
    const blocks = parseMarkdown('# Um\n\ntexto\n\n### Sub\n\n## Dois **forte**\n\n##   \n');
    expect(outline(blocks)).toEqual([
      { index: 0, text: 'Um' },
      { index: 3, text: 'Dois forte' },
    ]);
  });
});

function timeParse(lines: number): number {
  const body = '- a\n' + ' b\n'.repeat(lines);
  const start = performance.now();
  parseMarkdown(body);
  return performance.now() - start;
}

describe('indented list continuation lines', () => {
  it('parse 30k continuation lines (about 90KB, under the 200KB limit) quickly', () => {
    timeParse(1000); // warm up
    const ms = timeParse(30_000);
    // Linear parsing takes tens of ms; the quadratic copy takes seconds.
    expect(ms).toBeLessThan(1500);
  }, 120_000);

  it('take linear time: doubling them does not quadruple the time', () => {
    timeParse(1000);
    const t1 = timeParse(15_000);
    const t2 = timeParse(30_000);
    expect(t2).toBeLessThan(Math.max(t1, 50) * 3);
  }, 120_000);
});
