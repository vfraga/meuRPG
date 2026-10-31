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

  it("takes the server's earlier refusal off the screen before the client's errors", async () => {
    const saver = new EntrySaver({ aOne: 'uma raça' }, 'a raça');
    await saver.run(
      () => Promise.reject(refusal('x.y')),
      () => true,
    );
    expect(saver.issues('x.y').length).toBeGreaterThan(0);
    expect(saver.summary()).not.toBe('');
    saver.blockOn(['a.b'], () => undefined);
    expect(saver.issues('x.y')).toEqual([]);
    expect(saver.summary()).toBe('');
    expect(saver.placement().fields).toEqual([]);
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
