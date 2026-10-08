import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import { XpMode } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantState,
  EncounterStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  AwardXPResponseSchema,
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
  ListXPAwardsResponseSchema,
  XPAwardMode,
  XPAwardSchema,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../../../gen/meurpg/progression/v1/progression_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { RosterClient } from '../../../../core/maps/roster-client';
import { ProgressionClient } from '../../../../core/progression/progression-client';
import { XpChanges } from '../../../../core/progression/xp-changes';
import { CombatXp, type CombatXpState } from './combat-xp';

const nbsp = ' ';

const party = [
  combatant({ id: 'p', label: 'Pensantus', kind: CombatantKind.PLAYER, characterId: 'cp' }),
  combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, characterId: 'ct' }),
  combatant({
    id: 'b',
    label: 'Brisa',
    kind: CombatantKind.PLAYER,
    characterId: 'cb',
    state: CombatantState.DOWN,
    hitPointsCurrent: 0,
  }),
];
const goblins = [
  combatant({
    id: 'c',
    label: 'Capitão Goblin',
    defeated: true,
    state: CombatantState.DEFEATED,
    xpValue: 200,
  }),
  ...[1, 2, 3].map((n) =>
    combatant({
      id: `g${n}`,
      label: `Goblin ${n}`,
      defeated: true,
      state: CombatantState.DEFEATED,
      xpValue: 50,
    }),
  ),
];

function experienceOf(xp: Record<string, number>, mode = XpMode.ENEMIES) {
  const names: Record<string, string> = { cp: 'Pensantus', ct: 'Toren', cb: 'Brisa' };
  return create(GetCampaignExperienceResponseSchema, {
    xpMode: mode,
    characters: Object.entries(xp).map(([id, points]) =>
      create(CharacterExperienceSchema, {
        characterId: id,
        name: names[id],
        level: 3,
        experiencePoints: points,
        nextLevelXp: 2700,
        canLevelUp: points >= 2700,
      }),
    ),
  });
}

describe('CombatXp (E7-06)', () => {
  const experience = vi.fn();
  const listAwards = vi.fn();
  const award = vi.fn();
  const dialogOpen = vi.fn();
  const states: CombatXpState[] = [];

  beforeEach(() => {
    states.length = 0;
    experience.mockReset().mockResolvedValue(experienceOf({ cp: 2600, ct: 2250, cb: 1950 }));
    listAwards.mockReset().mockResolvedValue(create(ListXPAwardsResponseSchema, { awards: [] }));
    award.mockReset();
    dialogOpen.mockReset().mockReturnValue({ afterClosed: () => of(undefined) });
  });

  function setup(combatants = [...goblins, ...party]) {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: ProgressionClient, useValue: { experience, listAwards, award } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: MatDialog, useValue: { open: dialogOpen } },
        { provide: MatBottomSheet, useValue: { open: dialogOpen } },
      ],
    });
    const fixture = TestBed.createComponent(CombatXp);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ id: 'enc-1', status: EncounterStatus.ENDED, combatants }),
    );
    fixture.componentInstance.stateChange.subscribe((s) => states.push(s));
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  /** Lets the fake calls answer (they are plain promises, which `whenStable` does not wait for). */
  async function ready(fixture: ReturnType<typeof setup>['fixture']) {
    for (let i = 0; i < 3; i++) {
      await new Promise((resolve) => setTimeout(resolve, 0));
      fixture.detectChanges();
    }
  }

  const text = (el: HTMLElement) => el.textContent!.replace(/[ \t\r\n]+/g, ' ');
  const boxes = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLInputElement>('app-xp-recipients input[type="checkbox"]'));
  const give = (el: HTMLElement) => el.querySelector<HTMLButtonElement>('app-xp-actions .primary')!;
  const notNow = (el: HTMLElement) =>
    el.querySelector<HTMLButtonElement>('app-xp-actions .secondary')!;

  it('waits for the XP to load, then shows the block, and says where it stands', async () => {
    const { fixture, el } = setup();
    expect(text(el)).toContain('Carregando a experiência...');
    expect(states).toEqual(['loading']);
    await ready(fixture);
    expect(text(el)).toContain('Experiência do combate');
    expect(states.at(-1)).toBe('open');
  });

  it('adds up what the defeated are worth and lists who receives', async () => {
    const { fixture, el } = setup();
    await ready(fixture);
    expect(text(el)).toContain('Total dos 4 derrotados');
    expect(text(el)).toContain('200 + 50 + 50 + 50');
    expect(el.querySelector('.total__n')?.textContent).toBe(`350${nbsp}XP`);
    expect(text(el)).toContain(`2.600${nbsp}XP agora`);
  });

  it("counts each monster's ND: three Bandidos are 3 × 25 = 75 XP, 18 each among four, 3 left over (E10-08 state 9)", async () => {
    const bandits = [1, 2, 3].map((n) =>
      combatant({
        id: `b${n}`,
        label: `Bandido ${n}`,
        defeated: true,
        state: CombatantState.DEFEATED,
        xpValue: 25,
        challengeRating: '1/8',
        bestiaryCreatureKey: 'monster:bandit',
      }),
    );
    const four = [
      ...party,
      combatant({ id: 's', label: 'Sálvia', kind: CombatantKind.PLAYER, characterId: 'cs' }),
    ];
    experience.mockResolvedValue(experienceOf({ cp: 2600, ct: 2250, cb: 1950, cs: 2000 }));
    const { fixture, el } = setup([...bandits, ...four]);
    await ready(fixture);
    expect(
      Array.from(el.querySelectorAll('.kind__t')).map((k) => text(k as HTMLElement).trim()),
    ).toEqual([`Bandido 1 a 3 · ND 1/8 · 25${nbsp}XP cada`]);
    expect(Array.from(el.querySelectorAll('.kind__n')).map((k) => k.textContent)).toEqual([
      `75${nbsp}XP`,
    ]);
    expect(el.querySelector('.total__n')?.textContent).toBe(`75${nbsp}XP`);
    expect(text(el)).toContain('Total dos 3 derrotados');
    expect(el.querySelector('app-xp-split .split__big')?.textContent).toBe(`18${nbsp}XP para cada`);
    expect(el.querySelector('app-xp-split .split__sum')?.textContent).toContain('3');
    expect(give(el).textContent?.trim()).toBe(`Dar 18${nbsp}XP a cada um`);
  });

  it('lists no kinds when no monster of the bestiary was defeated (the block is as it always was)', async () => {
    const { fixture, el } = setup();
    await ready(fixture);
    expect(el.querySelector('.kinds')).toBeNull();
  });

  it('checks by default who fought and is alive or down; the dead come unchecked and cannot be checked', async () => {
    const dead = combatant({
      id: 'm',
      label: 'Morto Teste',
      kind: CombatantKind.PLAYER,
      characterId: 'cm',
      state: CombatantState.DEAD,
    });
    const { fixture, el } = setup([...goblins, ...party, dead]);
    await ready(fixture);

    expect(boxes(el).map((b) => b.checked)).toEqual([true, true, true, false]);
    expect(boxes(el)[3].disabled).toBe(true);
    // The one who is down gets a word and the reason she is checked.
    expect(text(el)).toContain('Caída');
    expect(text(el)).toContain('Está viva, então recebe a parte dela.');
    expect(text(el)).toContain('Morreu: não recebe XP.');
    expect(text(el)).toContain('Não recebe');
  });

  it('shows the division live, written out, and moves it when someone is unchecked', async () => {
    const { fixture, el } = setup();
    await ready(fixture);
    expect(el.querySelector('app-xp-split .split__big')?.textContent).toBe(
      `116${nbsp}XP para cada`,
    );
    expect(el.querySelector('app-xp-split .split__sum')?.textContent).toBe(
      `350${nbsp}XP ÷ 3 = 116,67, arredondado para baixo. 2${nbsp}XP se perdem na divisão.`,
    );
    expect(give(el).textContent?.trim()).toBe(`Dar 116${nbsp}XP a cada um`);

    boxes(el)[2].click(); // Brisa
    fixture.detectChanges();
    expect(el.querySelector('app-xp-split .split__big')?.textContent).toBe(
      `175${nbsp}XP para cada`,
    );
    expect(el.querySelector('app-xp-split .split__sum')?.textContent).toContain(
      'Divisão exata, nada se perde.',
    );
    expect(text(el)).toContain(`+175${nbsp}XP`);
    // In its own polite status.
    expect(el.querySelector('app-xp-split [role="status"]')?.textContent).toBe(
      `175${nbsp}XP para cada.`,
    );
  });

  it('waits with nobody checked, says why, and never calls', async () => {
    const { fixture, el } = setup();
    await ready(fixture);
    boxes(el).forEach((b) => b.click());
    fixture.detectChanges();

    expect(el.querySelector('app-xp-actions .reason')?.textContent?.trim()).toBe(
      'Marque pelo menos um personagem',
    );
    expect(give(el).getAttribute('aria-disabled')).toBe('true');
    give(el).click();
    expect(award).not.toHaveBeenCalled();
  });

  it('says so when the defeated are worth nothing, and points to "Dar XP"', async () => {
    const free = goblins.map((g) => ({ ...g, xpValue: 0 }) as typeof g);
    const { fixture, el } = setup([...free, ...party]);
    await ready(fixture);
    expect(el.querySelector('app-xp-actions .reason')?.textContent).toContain(
      'Nenhum derrotado dá XP',
    );
    expect(give(el).getAttribute('aria-disabled')).toBe('true');
  });

  it('the filled button and "Agora não" are the pair, the filled one first', async () => {
    const { fixture, el } = setup();
    await ready(fixture);
    const names = Array.from(el.querySelectorAll('app-xp-actions button')).map((b) =>
      b.textContent?.trim(),
    );
    expect(names).toEqual([`Dar 116${nbsp}XP a cada um`, 'Agora não']);
  });

  describe('giving it', () => {
    function answer() {
      award.mockResolvedValue(
        create(AwardXPResponseSchema, {
          xpEach: 116,
          lostXp: 2,
          award: create(XPAwardSchema, {
            id: 'a1',
            mode: XPAwardMode.XP_AWARD_MODE_ENEMIES,
            encounterId: 'enc-1',
            totalXp: 350,
            shares: [
              { characterId: 'cp', characterName: 'Pensantus', xp: 116 },
              { characterId: 'ct', characterName: 'Toren', xp: 116 },
              { characterId: 'cb', characterName: 'Brisa', xp: 116 },
            ],
          }),
        }),
      );
    }

    /** After it, the server says each one's new XP; Pensantus reached the next level. */
    function afterGiving() {
      experience.mockResolvedValue(experienceOf({ cp: 2716, ct: 2366, cb: 2066 }));
    }

    it("sends the combat's award with the reason and who is checked, once", async () => {
      answer();
      const { fixture, el } = setup();
      await ready(fixture);
      boxes(el)[2].click();
      fixture.detectChanges();
      give(el).click();
      give(el).click();
      await ready(fixture);

      expect(award).toHaveBeenCalledTimes(1);
      expect(award).toHaveBeenCalledWith(
        'camp-1',
        { mode: 'enemies', encounterId: 'enc-1' },
        'Combate: Emboscada na estrada',
        ['cp', 'ct'],
        expect.stringMatching(/^[0-9a-f-]{36}$/),
      );
    });

    it('says what was given, the arithmetic of each one, and who can level up, and moves the focus to it', async () => {
      answer();
      const { fixture, el } = setup();
      await ready(fixture);
      afterGiving();
      give(el).click();
      await ready(fixture);

      const status = el.querySelector<HTMLElement>('.done')!;
      expect(status.getAttribute('role')).toBe('status');
      expect(document.activeElement).toBe(status);
      expect(text(el)).toContain(
        `350${nbsp}XP dados: 116 para cada. 2${nbsp}XP se perderam na divisão.`,
      );
      expect(text(el)).toContain(`2.600 + 116 = 2.716${nbsp}XP`);
      expect(text(el)).toContain(`2.716 de${nbsp}2.700${nbsp}XP`);
      expect(el.querySelectorAll('app-level-up-tag')).toHaveLength(1);
      expect(text(el)).toContain('Pensantus chegou ao XP do próximo nível.');
      expect(text(el)).toContain('Para desfazer este XP, use "Experiência" na página da campanha.');
      expect(el.querySelector('a')?.getAttribute('href')).toBe('/campaigns/camp-1');
      expect(states.at(-1)).toBe('given');
      // The form is gone: nothing left to give twice.
      expect(el.querySelector('app-xp-actions')).toBeNull();
    });

    it('says why in words when the server refuses, and reads again when it was already given', async () => {
      award.mockRejectedValue(
        new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: XPBlockedSchema,
            value: { reason: XPBlockedReason.XP_BLOCKED_REASON_ALREADY_AWARDED },
          },
        ]),
      );
      const { fixture, el } = setup();
      await ready(fixture);
      listAwards.mockClear();
      give(el).click();
      await ready(fixture);

      expect(el.querySelector('[role="alert"]')?.textContent).toContain(
        'O XP desse combate já foi dado',
      );
      expect(listAwards).toHaveBeenCalled();
    });

    it('keeps the same key for a retry after a lost answer', async () => {
      award.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      answer();
      award.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      const { fixture, el } = setup();
      await ready(fixture);
      give(el).click();
      await ready(fixture);
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
      give(el).click();
      await ready(fixture);
      expect(award.mock.calls[1][4]).toBe(award.mock.calls[0][4]);
    });
  });

  describe('"Agora não"', () => {
    it('leaves the quiet line, with the focus on it, and gives nothing', async () => {
      const { fixture, el } = setup();
      await ready(fixture);
      notNow(el).click();
      await ready(fixture);

      const line = el.querySelector<HTMLButtonElement>('.later')!;
      expect(line.textContent).toContain('XP do combate ainda não dado');
      expect(document.activeElement).toBe(line);
      expect(el.querySelector('app-xp-actions')).toBeNull();
      expect(award).not.toHaveBeenCalled();
      expect(states.at(-1)).toBe('later');
    });

    it('opens "Dar XP" with the reason and the total already filled', async () => {
      const { fixture, el } = setup();
      await ready(fixture);
      notNow(el).click();
      await ready(fixture);
      el.querySelector<HTMLButtonElement>('.later')!.click();

      const data = dialogOpen.mock.calls[0][1].data;
      expect(data).toMatchObject({
        campaignId: 'camp-1',
        xpMode: XpMode.ENEMIES,
        reason: 'Combate: Emboscada na estrada',
        amount: 350,
        encounterId: 'enc-1',
      });
      expect(data.rows.map((r: { id: string }) => r.id)).toEqual(['cp', 'ct', 'cb']);
    });
  });

  it("shows the XP as given when the history already has this combat's award (a reload, another tab)", async () => {
    listAwards.mockResolvedValue(
      create(ListXPAwardsResponseSchema, {
        awards: [
          create(XPAwardSchema, {
            id: 'a1',
            mode: XPAwardMode.XP_AWARD_MODE_ENEMIES,
            encounterId: 'enc-1',
            totalXp: 350,
            shares: [
              { characterId: 'cp', characterName: 'Pensantus', xp: 175 },
              { characterId: 'ct', characterName: 'Toren', xp: 175 },
            ],
          }),
        ],
      }),
    );
    const { fixture, el } = setup();
    await ready(fixture);
    expect(text(el)).toContain(`350${nbsp}XP dados: 175 para cada.`);
    expect(text(el)).toContain(`Recebeu 175${nbsp}XP`);
    expect(el.querySelector('app-xp-actions')).toBeNull();
  });

  it('ignores an award that was undone (the XP can be given again)', async () => {
    listAwards.mockResolvedValue(
      create(ListXPAwardsResponseSchema, {
        awards: [
          create(XPAwardSchema, {
            id: 'a1',
            mode: XPAwardMode.XP_AWARD_MODE_ENEMIES,
            encounterId: 'enc-1',
            undone: true,
          }),
        ],
      }),
    );
    const { fixture, el } = setup();
    await ready(fixture);
    expect(el.querySelector('app-xp-actions')).not.toBeNull();
  });

  describe('an award given here and undone elsewhere', () => {
    const given = (undone: boolean) =>
      create(XPAwardSchema, {
        id: 'a1',
        mode: XPAwardMode.XP_AWARD_MODE_ENEMIES,
        encounterId: 'enc-1',
        totalXp: 350,
        shares: [
          { characterId: 'cp', characterName: 'Pensantus', xp: 175 },
          { characterId: 'ct', characterName: 'Toren', xp: 175 },
        ],
        undone,
      });

    async function giveThenUndo() {
      award.mockResolvedValue(create(AwardXPResponseSchema, { xpEach: 175, award: given(false) }));
      const s = setup();
      await ready(s.fixture);
      give(s.el).click();
      await ready(s.fixture);
      expect(s.el.querySelector('app-xp-actions')).toBeNull();
      listAwards.mockResolvedValue(create(ListXPAwardsResponseSchema, { awards: [given(true)] }));
      TestBed.inject(XpChanges).bump();
      s.fixture.detectChanges();
      await ready(s.fixture);
      return s;
    }

    it('returns to the give form once the award was undone', async () => {
      const { el } = await giveThenUndo();
      expect(el.querySelector('app-xp-actions')).not.toBeNull();
    });

    it('uses a fresh idempotency key to give again after the undo', async () => {
      const { fixture, el } = await giveThenUndo();
      give(el).click();
      await ready(fixture);
      expect(award).toHaveBeenCalledTimes(2);
      expect(award.mock.calls[1][4]).not.toBe(award.mock.calls[0][4]);
    });
  });

  it('draws nothing in a campaign that does not give XP for enemies, and says so', async () => {
    experience.mockResolvedValue(experienceOf({ cp: 0 }, XpMode.MILESTONES));
    const { fixture, el } = setup();
    await ready(fixture);
    expect(el.textContent?.trim()).toBe('');
    expect(states.at(-1)).toBe('none');
  });

  it('draws nothing when the XP could not be read, so the summary is still usable', async () => {
    experience.mockRejectedValue(new Error('x'));
    const { fixture, el } = setup();
    await ready(fixture);
    expect(el.textContent?.trim()).toBe('');
    expect(states.at(-1)).toBe('none');
  });

  it('reads the XP again on `xp_changed`', async () => {
    const { fixture } = setup();
    await ready(fixture);
    experience.mockClear();
    TestBed.inject(XpChanges).bump();
    fixture.detectChanges();
    await ready(fixture);
    expect(experience).toHaveBeenCalledTimes(1);
  });
});
