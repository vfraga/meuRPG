import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import {
  TableContentKind,
  TableContentViolationSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { settle } from '../../../../test-settle';
import { ContentSummary, contentCounts, leftOutSentence } from './content-summary';

const entry = (kind: TableContentKind, archived = false) => ({ kind, archived }) as TableEntry;

describe('"Conteúdo da mesa" on the "Regras" tab', () => {
  it('counts what the table made, by kind, in a sentence (board C6-03)', () => {
    const entries = [
      entry(TableContentKind.CLASS),
      entry(TableContentKind.CLASS),
      entry(TableContentKind.RACE),
      entry(TableContentKind.BACKGROUND),
      ...Array.from({ length: 5 }, () => entry(TableContentKind.SPELL)),
    ];
    expect(contentCounts(entries)).toBe('2 classes, 1 raça, 1 antecedente e 5 magias da mesa.');
  });

  it('leaves out the archived entries, and says nothing when there are none', () => {
    expect(contentCounts([entry(TableContentKind.CLASS, true)])).toBe('');
    expect(contentCounts([])).toBe('');
  });

  it('shows the sentence and the way to the content page', async () => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: TableContentClient,
          useValue: {
            list: () =>
              Promise.resolve({
                entries: [entry(TableContentKind.CLASS)],
                tableRevision: 1,
                contentVersion: '',
                leftOut: [],
              }),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(ContentSummary);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    await settle(fixture, 2);
    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('h2')?.textContent?.trim()).toBe('Conteúdo da mesa');
    expect(el.textContent).toContain('1 classe da mesa. O SRD continua valendo.');
    expect(el.querySelector('a')?.getAttribute('href')).toBe('/campaigns/camp-1/content');
    expect(el.querySelector('a')?.textContent?.trim()).toBe('Abrir o conteúdo');
  });

  async function open(leftOut: number) {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: TableContentClient,
          useValue: {
            list: () =>
              Promise.resolve({
                entries: [entry(TableContentKind.CLASS)],
                tableRevision: 1,
                contentVersion: '',
                leftOut: Array.from({ length: leftOut }, (_, i) =>
                  create(TableContentViolationSchema, { key: `class:c${i}@mesa` }),
                ),
              }),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(ContentSummary);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    await settle(fixture, 2);
    return fixture.nativeElement as HTMLElement;
  }

  it('says in words how many entries are out, one and many', () => {
    expect(leftOutSentence(1)).toBe('1 entrada do conteúdo da mesa está fora da campanha.');
    expect(leftOutSentence(3)).toBe('3 entradas do conteúdo da mesa estão fora da campanha.');
  });

  it('warns the master with a way to the content when the rules left entries out', async () => {
    const el = await open(3);
    const notice = el.querySelector('.mr-notice--danger')!;
    expect(notice.querySelector('p')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      '3 entradas do conteúdo da mesa estão fora da campanha. Ver o conteúdo da mesa',
    );
    const link = notice.querySelector('a')!;
    expect(link.textContent?.trim()).toBe('Ver o conteúdo da mesa');
    expect(link.getAttribute('href')).toBe('/campaigns/camp-1/content');
  });

  it('has no warning when nothing is out, as a player never has', async () => {
    const el = await open(0);
    expect(el.querySelector('.mr-notice')).toBeNull();
    expect(el.textContent).toContain('1 classe da mesa.');
  });
});
