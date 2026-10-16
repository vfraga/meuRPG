import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { provideRouter } from '@angular/router';

import { XpMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
  ListMilestonesResponseSchema,
  MilestoneSchema,
  XPAwardSchema,
} from '../../../../gen/meurpg/progression/v1/progression_pb';
import { AuthService } from '../../../core/auth/auth.service';
import { RosterClient } from '../../../core/maps/roster-client';
import { ExperienceStore } from '../../../core/progression/experience-store';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { MilestonesPanel } from './milestones-panel';
import { nextTurn } from '../../../../test-settle';

const reachedVale = create(MilestoneSchema, {
  id: 'm1',
  text: 'Chegar ao Vale Seco',
  reached: true,
  reachedAt: timestampFromDate(new Date(2026, 9, 3, 22, 5)),
  marks: [
    create(XPAwardSchema, {
      id: 'a1',
      givenByDisplayName: 'Samuel',
      canUndo: true,
      createdAt: timestampFromDate(new Date(2026, 9, 3, 22, 5)),
      shares: [
        { characterId: 'p', characterName: 'Pensantus' },
        { characterId: 't', characterName: 'Toren' },
      ],
    }),
  ],
});

describe('MilestonesPanel (E8-14)', () => {
  const api = { experience: vi.fn(), listAwards: vi.fn(), listMilestones: vi.fn() };

  async function setup(isMaster: boolean, milestones: unknown[]) {
    api.experience.mockResolvedValue(
      create(GetCampaignExperienceResponseSchema, {
        xpMode: XpMode.MILESTONES,
        characters: [
          create(CharacterExperienceSchema, {
            characterId: 'p',
            name: 'Pensantus',
            playerUserId: 'me',
            level: 3,
            canLevelUp: milestones.length > 0,
          }),
          create(CharacterExperienceSchema, {
            characterId: 't',
            name: 'Toren',
            playerUserId: 'other',
            level: 3,
            canLevelUp: milestones.length > 0,
          }),
          create(CharacterExperienceSchema, {
            characterId: 'b',
            name: 'Brisa',
            playerUserId: 'third',
            level: 3,
          }),
        ],
      }),
    );
    api.listMilestones.mockResolvedValue(
      create(ListMilestonesResponseSchema, { milestones: milestones as never }),
    );
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        ExperienceStore,
        { provide: ProgressionClient, useValue: api },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        {
          provide: AuthService,
          useValue: {
            state: () => ({ status: 'signed-in', user: { id: 'me', displayName: null } }),
          },
        },
        { provide: MatDialog, useValue: { open: vi.fn() } },
        { provide: MatBottomSheet, useValue: { open: vi.fn() } },
      ],
    });
    await TestBed.inject(ExperienceStore).load('c1', false);
    const fixture = TestBed.createComponent(MilestonesPanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('campaignName', 'Mirathel');
    fixture.componentRef.setInput('isMaster', isMaster);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const text = (el: HTMLElement) => el.textContent!.replace(/\s+/g, ' ');
  const headings = (el: HTMLElement) =>
    Array.from(el.querySelectorAll('h3')).map((h) => h.textContent!.trim());

  beforeEach(() => {
    Object.values(api).forEach((f) => f.mockReset());
    api.listAwards.mockResolvedValue({ awards: [], nextPageToken: '' });
  });

  it('master before any milestone: the planned list first, an empty "alcançados", the off-list action', async () => {
    const planned = [create(MilestoneSchema, { id: 'x', text: 'Salvar o mercador' })];
    const { el } = await setup(true, planned);
    expect(headings(el)).toEqual(['Marcos planejados', 'Marcos alcançados']);
    expect(text(el)).toContain('Nenhum marco alcançado ainda. Quando você marcar um');
    expect(text(el)).not.toContain('Personagens'); // nothing to say about anyone yet
    expect(text(el)).toContain('Registrar um marco fora da lista');
  });

  it('master with nothing planned and nothing reached: only the invitation', async () => {
    const { el } = await setup(true, []);
    expect(headings(el)).toEqual(['Marcos planejados']);
    expect(text(el)).toContain('Nenhum marco planejado');
    expect(text(el)).toContain('Registrar um marco fora da lista');
  });

  it('master after a milestone: reached on top, then the group, then the planned ones', async () => {
    const { el } = await setup(true, [
      reachedVale,
      create(MilestoneSchema, { id: 'x', text: 'Salvar o mercador' }),
    ]);
    expect(headings(el)).toEqual(['Marcos alcançados', 'Personagens', 'Marcos planejados']);
    expect(text(el)).toContain('Sem marco novo'); // Brisa
  });

  it('a player reads only the reached milestones, and never a hint of the planned ones', async () => {
    const { el } = await setup(false, [reachedVale]);
    expect(headings(el)).toEqual(['Marcos alcançados', 'Seu personagem']);
    expect(text(el)).toContain('Subiram de nível: Pensantus e Toren');
    expect(text(el)).not.toMatch(/planejad/i);
    expect(el.querySelectorAll('app-milestone-characters li')).toHaveLength(1); // only their own
    expect(text(el)).toContain('Pensantus');
    expect(text(el)).not.toContain('Brisa');
    const open = el.querySelector<HTMLAnchorElement>('app-milestone-characters a')!;
    expect(open.textContent?.trim()).toBe('Abrir a ficha');
    expect(open.getAttribute('href')).toBe('/campaigns/c1/characters/p');
  });

  it('a player before the first milestone: the empty state, no character row, no hint', async () => {
    const { el } = await setup(false, []);
    expect(headings(el)).toEqual(['Marcos alcançados']);
    expect(text(el)).toContain(
      'Nenhum marco alcançado ainda. Quando o grupo cumprir um, ele aparece aqui.',
    );
    expect(el.querySelector('app-milestone-characters')).toBeNull();
    expect(text(el)).not.toMatch(/planejad/i);
    expect(el.querySelector('app-planned-milestones')).toBeNull();
  });

  it('says to a player which milestone appeared since the last read, naming only their character', async () => {
    const { fixture, el } = await setup(false, []);
    api.listMilestones.mockResolvedValue(
      create(ListMilestonesResponseSchema, { milestones: [reachedVale] }),
    );
    document.dispatchEvent(new Event('visibilitychange'));
    await fixture.whenStable();
    await nextTurn();
    fixture.detectChanges();
    expect(el.querySelector('.live')?.textContent).toBe(
      'O mestre marcou Chegar ao Vale Seco. Pensantus pode subir de nível.',
    );
  });
});
