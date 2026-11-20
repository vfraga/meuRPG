import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { cwd } from 'node:process';

import { describeInviteErrorCode } from '../campaigns/invite-errors';
import { violationText } from '../content/content-violations';
import { BODY_REASON_TEXT } from './body-reasons';

/**
 * The closed list of error reasons (proto/error-reasons.json) against the
 * Portuguese sentences. The Go test in backend/internal/archtest keeps the list
 * equal to what the server can send; this spec keeps the web equal to the list:
 * a listed reason with no sentence fails, and a sentence for a reason that is
 * not listed fails. docs/architecture/api.md#error-reasons says how to add one.
 */

interface EnumField {
  field: string;
  enum: string;
  reasons: string[];
  unsent: string[];
}
interface ReasonList {
  details: { message: string; fields: EnumField[] }[];
  body: { reason: string }[];
  violations: { reason: string }[];
  redirects: { reason: string }[];
}

// The tests run from web/ (plain and under coverage); import.meta.url points elsewhere once coverage bundles the specs.
const WEB = cwd();
const APP = join(WEB, 'src', 'app');
const LIST: ReasonList = JSON.parse(
  readFileSync(join(WEB, '..', 'proto', 'error-reasons.json'), 'utf8'),
);

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const full = join(dir, e.name);
    if (e.isDirectory()) {
      return sources(full);
    }
    const code = e.name.endsWith('.ts') && !e.name.endsWith('.spec.ts');
    return code && !e.name.includes('testing') ? [readFileSync(full, 'utf8')] : [];
  });
}
const CODE = sources(APP).join('\n');

/**
 * The values of an enum the sources name (`Enum.VALUE`), under its own name and the ones it is imported as. The
 * sources are read once for each enum, not once for each reason: the list of reasons grows with every feature.
 */
const NAMED = new Map<string, Set<string>>();
function namedValues(enumName: string): Set<string> {
  let named = NAMED.get(enumName);
  if (!named) {
    const aliases = [...CODE.matchAll(new RegExp(`\\b${enumName} as (\\w+)`, 'g'))].map(
      (m) => m[1],
    );
    const names = [enumName, ...aliases].join('|');
    named = new Set(
      [...CODE.matchAll(new RegExp(`\\b(?:${names})\\.(\\w+)`, 'g'))].map((m) => m[1]),
    );
    NAMED.set(enumName, named);
  }
  return named;
}

/** Whether a source names the enum value (`Enum.VALUE`, or `Enum.PREFIX_VALUE` for the enums whose generated names keep the prefix). */
function isHandled(enumName: string, value: string): boolean {
  const named = namedValues(enumName);
  if (named.has(value)) {
    return true;
  }
  const suffix = `_${value}`;
  return [...named].some((n) => n.endsWith(suffix) && /^[A-Z_]*$/.test(n.slice(0, -suffix.length)));
}

describe('error reasons: a typed detail', () => {
  const pairs = LIST.details.flatMap((d) =>
    d.fields.flatMap((f) => f.reasons.map((r) => ({ message: d.message, enumName: f.enum, r }))),
  );

  it('lists reasons', () => {
    expect(pairs.length).toBeGreaterThan(200);
  });

  it('has a place in the web for every reason the server sends', () => {
    const missing = pairs
      .filter((p) => !isHandled(p.enumName, p.r))
      .map((p) => `${p.message}: ${p.enumName}.${p.r}`);
    expect(missing).toEqual([]);
  });
});

describe('error reasons: the body of an HTTP route error', () => {
  const listed = [...new Set(LIST.body.map((b) => b.reason))].sort();

  it('has a Portuguese sentence for every listed reason', () => {
    for (const reason of listed) {
      const text = (BODY_REASON_TEXT as Record<string, string | undefined>)[reason];
      expect(text, reason).toMatch(/^[A-ZÀ-Ý].{12,}[.!]$/);
    }
  });

  it('has no sentence for a reason that is not listed', () => {
    expect(Object.keys(BODY_REASON_TEXT).sort()).toEqual(listed);
  });
});

describe('error reasons: a violation of the table content', () => {
  const listed = LIST.violations.map((v) => v.reason);
  /** Fields that say what the reason means in their kind of place: the entry, a name, an unknown path and the rows that have their own words. */
  const PROBES = [
    '',
    'table_race.name_pt',
    'table_x.unknown',
    'table_class.levels',
    'table_class.casting.kind',
    'table_race.traits[0].effects[0].value',
  ];
  const ctx = { aOne: 'uma magia' };
  const textOf = (field: string, reason: string) => violationText({ field, reason }, ctx);

  it('has a sentence of its own for every listed reason', () => {
    const generic = 'reason-nobody-sends';
    const bare = listed.filter(
      (reason) => !PROBES.some((field) => textOf(field, reason) !== textOf(field, generic)),
    );
    expect(bare).toEqual([]);
  });
});

describe('error reasons: the reason in the query of an error page', () => {
  it('has a sentence of its own for every listed reason of /invite/error', () => {
    const generic = describeInviteErrorCode(null);
    const bare = LIST.redirects
      .map((r) => r.reason)
      .filter((reason) => describeInviteErrorCode(reason) === generic);
    expect(bare).toEqual([]);
    expect(LIST.redirects.length).toBeGreaterThan(0);
  });
});
