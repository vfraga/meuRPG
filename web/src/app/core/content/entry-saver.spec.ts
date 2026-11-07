import { ChangeDetectionStrategy, Component, Injector } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableContentBlockedReason,
  TableContentBlockedSchema,
  TableContentRefusalSchema,
  TableContentViolationSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { EntrySaver } from './entry-saver';

function refusal(field: string): ConnectError {
  return new ConnectError('refused', Code.InvalidArgument, undefined, [
    {
      desc: TableContentRefusalSchema,
      value: create(TableContentRefusalSchema, {
        violations: [create(TableContentViolationSchema, { field, reason: 'bad_formula' })],
      }),
    },
  ]);
}

function staleError(): ConnectError {
  return new ConnectError('x', Code.Aborted, undefined, [
    {
      desc: TableContentBlockedSchema,
      value: create(TableContentBlockedSchema, {
        reason: TableContentBlockedReason.STALE,
        key: 'race:corujeiro@mesa',
      }),
    },
  ]);
}

describe('EntrySaver.blockOn', () => {
  it('stops at the first path it is given, shows it, and does not touch the write', () => {
    const saver = new EntrySaver({ aOne: 'uma raça' }, 'a raça');
    const shown: string[] = [];
    expect(saver.blockOn(['a.b', 'c.d'], (p) => shown.push(p))).toBe(true);
    expect(shown).toEqual(['a.b']);
  });

  it('lets the save go on when there is nothing to block on, and leaves a refusal alone', async () => {
    const saver = new EntrySaver({ aOne: 'uma raça' }, 'a raça');
    await saver.run(
      () => Promise.reject(refusal('x.y')),
      () => true,
    );
    expect(saver.summary()).not.toBe('');
    expect(saver.blockOn([], () => undefined)).toBe(false);
    expect(saver.summary()).not.toBe('');
  });

  it("takes the server's earlier refusal of a field off the screen when the client has its own error there", async () => {
    const saver = new EntrySaver({ aOne: 'uma raça' }, 'a raça');
    await saver.run(
      () => Promise.reject(refusal('x.y')),
      () => true,
    );
    expect(saver.issues('x.y').length).toBeGreaterThan(0);
    expect(saver.summary()).not.toBe('');
    saver.blockOn(['x.y'], () => undefined);
    expect(saver.issues('x.y')).toEqual([]);
    expect(saver.summary()).toBe('');
    expect(saver.placement().fields).toEqual([]);
  });

  it("keeps the server's refusal of a field the client has nothing to say about, and counts it", async () => {
    const saver = new EntrySaver({ aOne: 'uma raça' }, 'a raça');
    await saver.run(
      () => Promise.reject(refusal('x.y')),
      () => true,
    );
    saver.blockOn(['a.b'], () => undefined);
    expect(saver.issues('x.y').length).toBeGreaterThan(0);
    expect(saver.placement().fields).toEqual(['x.y']);
    expect(saver.summary()).toContain('1 campo');
  });
});

describe('EntrySaver.blockOn and a stale entry', () => {
  it('keeps the stale banner: the entry stays out of date until it is reloaded', async () => {
    const saver = new EntrySaver({ aOne: 'uma raça' }, 'a raça');
    await saver.run(
      () => Promise.reject(staleError()),
      () => true,
    );
    expect(saver.stale()).toBe(true);
    expect(saver.blockOn(['a.b'], () => undefined)).toBe(true);
    expect(saver.stale()).toBe(true);
  });
});

describe('EntrySaver.focusRefusal', () => {
  it('focuses the first field the refusal landed on, once it is drawn', async () => {
    @Component({
      template: '<input data-field="x.y" />',
      changeDetection: ChangeDetectionStrategy.OnPush,
    })
    class Host {}
    TestBed.configureTestingModule({});
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const saver = new EntrySaver({ aOne: 'uma raça' }, 'a raça');
    await saver.run(
      () => Promise.reject(refusal('x.y')),
      () => true,
    );
    saver.focusRefusal(fixture.nativeElement as HTMLElement, TestBed.inject(Injector));
    await fixture.whenStable();
    TestBed.tick();
    expect(document.activeElement).toBe(
      (fixture.nativeElement as HTMLElement).querySelector('input'),
    );
  });
});

describe('EntrySaver.seedLeftOut', () => {
  const left = (field: string, reason = 'bad_value') =>
    create(TableContentViolationSchema, { key: 'class:guardiao@mesa', field, reason });

  it('puts the violation of a left-out entry on its field, with the entry key cleared, and says no summary', () => {
    const saver = new EntrySaver({ aOne: 'uma classe' }, 'a classe');
    const at = 'table_class.levels[0].features[0].effects[0].target';
    expect(saver.seedLeftOut(left(at), (p) => p === at)).toBe(at);
    expect(saver.issues(at)).toEqual([
      'Este alvo não está mais no menu de efeitos. Escolha outro.',
    ]);
    expect(saver.summary()).toBe('');
    expect(saver.placement().top).toEqual([]);
  });

  it('marks the nearest input the editor draws when the field itself has none', () => {
    const saver = new EntrySaver({ aOne: 'uma classe' }, 'a classe');
    const feature = 'table_class.levels[0].features[0]';
    expect(saver.seedLeftOut(left(`${feature}.effects[0].target`), (p) => p === feature)).toBe(
      feature,
    );
  });

  it('keeps a violation with no field for the page to say, and marks no input', () => {
    const saver = new EntrySaver({ aOne: 'uma classe' }, 'a classe');
    expect(saver.seedLeftOut(left('', 'overlay'), () => true)).toBe('');
    expect(saver.placement().fields).toEqual([]);
    expect(saver.placement().top).toHaveLength(1);
  });

  it('does nothing for an entry that is not left out, and a save clears it', async () => {
    const saver = new EntrySaver({ aOne: 'uma classe' }, 'a classe');
    expect(saver.seedLeftOut(null, () => true)).toBe('');
    expect(saver.placement().count).toBe(0);
    saver.seedLeftOut(left('table_class.hit_die'), () => true);
    await saver.run(
      () => Promise.resolve(1),
      () => true,
    );
    expect(saver.issues('table_class.hit_die')).toEqual([]);
  });
});
