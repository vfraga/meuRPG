// Finding U16-03 in review/unit-16-web-content-campaigns.md
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { type MessageInitShape, create } from '@bufbuild/protobuf';
import { BehaviorSubject } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type OptionSwitchEntry,
  OptionSwitchEntrySchema,
  TableContentKind,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import { SpellsClient } from '../../../core/spells/spells-client';
import { fakeContentWatcher } from '../../../core/content/content-testing';
import { ContentOptions } from './options';

const opt = (
  kind: TableContentKind,
  namePt: string,
  over: MessageInitShape<typeof OptionSwitchEntrySchema> = {},
) =>
  create(OptionSwitchEntrySchema, {
    key: `${TableContentKind[kind].toLowerCase()}:${namePt.toLowerCase().replace(/ /g, '-')}`,
    kind,
    namePt,
    ...over,
  });

const list = (): OptionSwitchEntry[] => [];

describe('Review16 U16-03: bulk off while the class filter loads', () => {
  const originalMatchMedia = window.matchMedia;
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  const switches = vi.fn();
  const setSwitches = vi.fn();
  const getCampaign = vi.fn();
  const spellsList = vi.fn();
  let watcher = fakeContentWatcher();
  let query$ = new BehaviorSubject(convertToParamMap({}));

  async function setup(
    role: Role = Role.MASTER,
    opts: {
      phone?: boolean;
      kind?: string;
      options?: OptionSwitchEntry[];
      stranger?: boolean;
    } = {},
  ) {
    switches.mockReset().mockResolvedValue({ options: opts.options ?? list(), tableRevision: 4 });
    spellsList.mockReset().mockResolvedValue({ spells: [{ key: 'spell:light' }] });
    setSwitches.mockReset().mockResolvedValue({ tableRevision: 5, changed: 1, options: [] });
    getCampaign.mockReset().mockResolvedValue({
      campaign: opts.stranger
        ? undefined
        : { id: 'camp-1', name: 'Mirathel', myRole: role, awaitingApproval: false },
    });
    query$ = new BehaviorSubject(convertToParamMap(opts.kind ? { kind: opts.kind } : {}));
    window.matchMedia = ((q: string) => ({
      matches: !!opts.phone && q.includes('max-width'),
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as never;
    TestBed.resetTestingModule();
    watcher = fakeContentWatcher();
    TestBed.overrideComponent(ContentOptions, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(convertToParamMap({ id: 'camp-1' })),
            queryParamMap: query$,
          },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: TableContentClient, useValue: { switches, setSwitches } },
        { provide: SpellsClient, useValue: { list: spellsList } },
      ],
    });
    const fixture = TestBed.createComponent(ContentOptions);
    await settle(fixture);
    return { fixture, el: fixture.nativeElement as HTMLElement, settle: () => settle(fixture) };
  }

  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 4; i++) {
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
    }
    fixture.detectChanges();
  }

  // The text a person reads: the icons' ligature names ("check") are not words.
  const text = (el: Element) => {
    const copy = el.cloneNode(true) as Element;
    copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
    return (copy.textContent ?? '')
      .replace(/\u00a0/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
  };
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      text(b).startsWith(label),
    )!;

  it('does not switch off every spell while the class list is still loading', async () => {
    const options = [
      opt(TableContentKind.CLASS, 'Mago', { key: 'class:wizard' }),
      opt(TableContentKind.SPELL, 'Luz', { key: 'spell:light', level: 0 }),
      opt(TableContentKind.SPELL, 'Bola de Fogo', { key: 'spell:fireball', level: 3 }),
      opt(TableContentKind.SPELL, 'Cura', { key: 'spell:cure', level: 1 }),
    ];
    const { el, settle } = await setup(Role.MASTER, { kind: 'spells', options });
    spellsList.mockReturnValue(new Promise(() => undefined));
    const select = el.querySelector<HTMLSelectElement>('app-select-field select')!;
    select.value = 'class:wizard';
    select.dispatchEvent(new Event('change'));
    await settle();
    const bulk = button(el, 'Desligar todas');
    if (!bulk.disabled) {
      bulk.click();
      await settle();
    }
    const sent = setSwitches.mock.calls.flatMap((c) => c[1] as { key: string }[]);
    expect(sent.map((x) => x.key)).toEqual([]);
  });
});
