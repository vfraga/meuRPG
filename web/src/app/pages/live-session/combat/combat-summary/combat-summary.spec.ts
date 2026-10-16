import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';

import { XpMode } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantState,
  EncounterStatus,
  GetCombatHighlightsResponseSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
  ListXPAwardsResponseSchema,
} from '../../../../../gen/meurpg/progression/v1/progression_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { RosterClient } from '../../../../core/maps/roster-client';
import { ProgressionClient } from '../../../../core/progression/progression-client';
import { CombatSummary } from './combat-summary';
import { nextTurn } from '../../../../../test-settle';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const nbsp = ' ';
const combatants = [
  combatant({ id: 'p', label: 'Pensantus', kind: CombatantKind.PLAYER, characterId: 'cp' }),
  combatant({
    id: 'g1',
    label: 'Goblin 1',
    defeated: true,
    state: CombatantState.DEFEATED,
    xpValue: 50,
  }),
  combatant({
    id: 'g2',
    label: 'Goblin 2',
    defeated: true,
    state: CombatantState.DEFEATED,
    xpValue: 50,
  }),
];

describe('CombatSummary and the XP (E7-06)', () => {
  const experience = vi.fn();
  const highlights = vi.fn();

  beforeEach(() => {
    highlights.mockReset().mockResolvedValue(create(GetCombatHighlightsResponseSchema, {}));
    experience.mockReset().mockResolvedValue(
      create(GetCampaignExperienceResponseSchema, {
        xpMode: XpMode.ENEMIES,
        characters: [
          create(CharacterExperienceSchema, {
            characterId: 'cp',
            name: 'Pensantus',
            level: 3,
            experiencePoints: 0,
            nextLevelXp: 300,
          }),
        ],
      }),
    );
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ProgressionClient,
          useValue: {
            experience,
            listAwards: () => Promise.resolve(create(ListXPAwardsResponseSchema, {})),
            award: vi.fn(),
          },
        },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: CombatClient, useValue: fakeCombatClient({ highlights }) },
        { provide: MatDialog, useValue: { open: vi.fn() } },
        { provide: MatBottomSheet, useValue: { open: vi.fn() } },
      ],
    });
  });

  function setup(master: boolean, list = combatants) {
    const fixture = TestBed.createComponent(CombatSummary);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ id: 'enc', status: EncounterStatus.ENDED, combatants: list }),
    );
    fixture.componentRef.setInput('isMaster', master);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('sessionNumber', 5);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  async function ready(fixture: ReturnType<typeof setup>['fixture']) {
    for (let i = 0; i < 3; i++) {
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const leave = (el: HTMLElement) => el.querySelector<HTMLButtonElement>('.end__leave')!;
  const filled = (b: HTMLButtonElement) => b.classList.contains('mat-mdc-unelevated-button');

  it('keeps "Voltar à sessão" outlined while the XP is still to give: the block\'s button is the one filled', async () => {
    const { fixture, el } = setup(true);
    await ready(fixture);
    expect(el.querySelector('app-combat-xp')).not.toBeNull();
    expect(filled(leave(el))).toBe(false);
    expect(leave(el).classList.contains('mat-mdc-outlined-button')).toBe(true);
    // Exactly one filled button on the screen.
    expect(Array.from(el.querySelectorAll('button')).filter(filled)).toHaveLength(1);
    expect(el.querySelector('app-combat-xp .primary')?.textContent?.trim()).toBe(
      `Dar 100${nbsp}XP a cada um`,
    );
  });

  it('turns it filled once the XP is left for later', async () => {
    const { fixture, el } = setup(true);
    await ready(fixture);
    el.querySelector<HTMLButtonElement>('app-combat-xp .secondary')!.click();
    await ready(fixture);

    expect(filled(leave(el))).toBe(true);
    expect(Array.from(el.querySelectorAll('button')).filter(filled)).toHaveLength(1);
  });

  it('is filled at once when the campaign has no XP for enemies (and no block takes room)', async () => {
    experience.mockResolvedValue(
      create(GetCampaignExperienceResponseSchema, { xpMode: XpMode.MILESTONES, characters: [] }),
    );
    const { fixture, el } = setup(true);
    await ready(fixture);
    expect(filled(leave(el))).toBe(true);
    expect(el.querySelector('.cols--xp')).toBeNull();
    // What the defeated were worth is not shown where it counts for nothing.
    expect(el.querySelector('.line__xp')).toBeNull();
  });

  it("keeps a player's defeated creature out of the enemies: not counted, not listed", async () => {
    const withFamiliar = [
      ...combatants,
      combatant({
        id: 'fam',
        label: 'Urso Pardo',
        kind: CombatantKind.CREATURE,
        defeated: true,
        state: CombatantState.DEFEATED,
        xpValue: 0,
      }),
    ];
    const master = setup(true, withFamiliar);
    await ready(master.fixture);
    const text = master.el.textContent!.replace(/\s+/g, ' ');
    expect(text).toContain('2 de 2');
    expect(text).not.toContain('Urso Pardo');
    expect(master.el.querySelectorAll('.line__xp').length).toBe(2);
  });

  it('shows the master what each defeated NPC is worth, and not a player', async () => {
    const master = setup(true);
    await ready(master.fixture);
    expect(Array.from(master.el.querySelectorAll('.line__xp')).map((x) => x.textContent)).toEqual([
      `50${nbsp}XP`,
      `50${nbsp}XP`,
    ]);

    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const player = setup(false);
    expect(player.el.querySelector('.line__xp')).toBeNull();
    expect(player.el.querySelector('app-combat-xp')).toBeNull();
    expect(filled(leave(player.el))).toBe(true);
    expect(experience).toHaveBeenCalledTimes(1); // only the master's block read it
  });

  it('puts the master\'s "Destaques do combate" between the summary tiles and the XP block, and asks for them by combat', async () => {
    const { fixture, el } = setup(true);
    await ready(fixture);
    const panel = el.querySelector('app-combat-highlights');
    expect(panel).not.toBeNull();
    expect(highlights).toHaveBeenCalledWith('camp-1', 'enc');
    const order = Array.from(el.children).map((c) => c.tagName.toLowerCase());
    expect(order.indexOf('app-combat-highlights')).toBeGreaterThan(order.indexOf('section'));
    expect(order.indexOf('app-combat-highlights')).toBeLessThan(order.indexOf('div'));
  });

  it('has no highlights panel for a player: theirs is the card on the session page', async () => {
    const { fixture, el } = setup(false);
    await ready(fixture);
    expect(el.querySelector('app-combat-highlights')).toBeNull();
    expect(highlights).not.toHaveBeenCalled();
  });

  it('does not say a dead character needs healing, and still says it of one who is only down', async () => {
    const down = (id: string, label: string, state: CombatantState) =>
      combatant({ id, label, kind: CombatantKind.PLAYER, characterId: `c-${id}`, state });
    const fixture = TestBed.createComponent(CombatSummary);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        id: 'enc',
        status: EncounterStatus.ENDED,
        combatants: [
          down('a', 'Brisa', CombatantState.DEAD),
          down('b', 'Toren', CombatantState.DOWN),
        ],
      }),
    );
    fixture.componentRef.setInput('isMaster', true);
    const zero = (characterId: string) =>
      ({
        characterId,
        hitPointsCurrent: 0,
        hitPointsMax: 10,
        spellSlots: [],
      }) as never;
    fixture.componentRef.setInput('vitals', [zero('c-a'), zero('c-b')]);
    fixture.detectChanges();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Toren está caída');
    expect(text).not.toContain('Brisa está caída');
  });

  it('counts the dead out of the players standing, and lists them among those who fell', async () => {
    const fixture = TestBed.createComponent(CombatSummary);
    const player = (id: string, label: string, state: CombatantState, extra = {}) =>
      combatant({ id, label, kind: CombatantKind.PLAYER, characterId: `c-${id}`, state, ...extra });
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        id: 'enc',
        status: EncounterStatus.ENDED,
        combatants: [
          player('a', 'Brisa', CombatantState.DEAD, { defeated: true }),
          player('b', 'Toren', CombatantState.UNHURT, { hitPointsCurrent: 12 }),
        ],
      }),
    );
    fixture.componentRef.setInput('isMaster', true);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const text = (el.textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toMatch(/Jogadores em pé\s*1 de 2/);
    expect(text).not.toContain('Ninguém caiu neste combate');
    expect(text).toContain('Brisa');
    expect(text).toContain('Morta');
  });

  it('has no "Voltar à sessão" of its own while the highlights card above it has the one "Fechar"', async () => {
    const { fixture, el } = setup(false);
    fixture.componentRef.setInput('quietTitle', true);
    fixture.detectChanges();
    expect(el.querySelector('.end__leave')).toBeNull();
  });
});
