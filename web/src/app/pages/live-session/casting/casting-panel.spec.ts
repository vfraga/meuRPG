import { TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';

import { DiceMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { OutsideCastEnd, OutsideCastStatus } from '../../../../gen/meurpg/play/v1/casting_pb';
import { CastingClient } from '../../../core/casting/casting-client';
import { FakeCastingClient, outsideCast } from '../../../core/casting/casting-testing';
import { RosterClient } from '../../../core/maps/roster-client';
import { CastingPanel } from './casting-panel';
import { nextTurn } from '../../../../test-settle';

const plain = (s: string | null | undefined) => (s ?? '').replace(/\s+/g, ' ').trim();

describe('CastingPanel (MR-048)', () => {
  async function setup(
    master: boolean,
    build: (api: FakeCastingClient) => void,
    diceMode = DiceMode.APP,
  ) {
    const api = new FakeCastingClient();
    build(api);
    TestBed.configureTestingModule({
      providers: [
        { provide: CastingClient, useValue: api },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
      ],
    });
    const fixture = TestBed.createComponent(CastingPanel);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('master', master);
    fixture.componentRef.setInput('characterId', master ? '' : 'ilaria');
    fixture.componentRef.setInput('characterName', 'Ilaria');
    fixture.componentRef.setInput('diceMode', diceMode);
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        await nextTurn();
        fixture.detectChanges();
      }
    };
    fixture.detectChanges();
    await settle();
    const press = async (text: string) => {
      const b = Array.from(el.querySelectorAll('button')).find((x) =>
        plain(x.textContent).includes(text),
      );
      expect(b, text).toBeTruthy();
      b!.click();
      await settle();
    };
    return { el, api, fixture, settle, press };
  }

  const casting = outsideCast({
    id: 'long',
    spellNamePt: 'Oração de Cura',
    status: OutsideCastStatus.CASTING,
    castingMinutes: 10,
    concentrating: true,
  });

  it('a player waits for the master and may stop the casting', async () => {
    const { el, api, press } = await setup(false, (a) => (a.active = [casting]));
    expect(plain(el.textContent)).toContain('Conjurando');
    expect(plain(el.textContent)).toContain('Esperando o mestre concluir');
    expect(plain(el.textContent)).not.toContain('Concluir conjuração');
    await press('Parar a conjuração');
    expect(api.stopped).toEqual(['long']);
  });

  it('the master completes the cast, with the dice in the app', async () => {
    const { el, api, press } = await setup(true, (a) => (a.active = [casting]));
    expect(plain(el.textContent)).toContain('Conjurações em andamento');
    expect(plain(el.textContent)).toContain('Conjurar como NPC');
    await press('Concluir conjuração');
    expect(api.finished).toHaveLength(1);
    expect(api.finished[0]).toMatchObject({ castId: 'long', dice: { inApp: true } });
  });

  it("the master's queue card reads as the board draws it: the title, the line, and the note that there is no clock", async () => {
    const ritual = outsideCast({
      id: 'r',
      spellNamePt: 'Alarme',
      casterName: 'Pensantus',
      status: OutsideCastStatus.CASTING,
      ritual: true,
      castingMinutes: 11,
    });
    const { el } = await setup(true, (a) => (a.active = [ritual]));
    expect(plain(el.querySelector('.cast__name')?.textContent)).toBe('Alarme (ritual)');
    expect(plain(el.querySelector('.cast__who')?.textContent)).toBe('Pensantus · 11 minutos');
    expect(plain(el.querySelector('[role="note"] p')?.textContent)).toBe(
      'Fora do combate não há relógio: a conjuração termina quando você confirma que o tempo passou.',
    );
  });

  it('the player has no queue note: they only wait for the master', async () => {
    const { el } = await setup(false, (a) => (a.active = [casting]));
    expect(el.querySelector('[role="note"]')).toBeNull();
  });

  it('with physical dice the master types the sum, and it goes along', async () => {
    const { el, api, press, settle } = await setup(
      true,
      (a) => (a.active = [casting]),
      DiceMode.PHYSICAL,
    );
    const field = el.querySelector<HTMLInputElement>('.cast__sum input')!;
    field.value = '9';
    field.dispatchEvent(new Event('input'));
    await settle();
    await press('Concluir conjuração');
    expect(api.finished[0].dice).toEqual({ typedSum: 9 });
  });

  it('lists the spells that last with their duration in game time and the note, never a clock', async () => {
    const bless = outsideCast({ lasts: true, durationSeconds: 60, concentrating: true });
    const mage = outsideCast({
      id: 'armor',
      spellNamePt: 'Armadura Arcana',
      casterName: 'Pensantus',
      casterId: 'pensantus',
      lasts: true,
      durationSeconds: 8 * 3600,
    });
    const { el } = await setup(true, (a) => (a.active = [bless, mage]));
    const text = plain(el.textContent);
    expect(text).toContain('Magias ativas');
    expect(text).toContain('concentração · até 1 minuto');
    expect(text).toContain('dura 8 horas');
    expect(text).toContain('descanso longo do mestre');
    expect(text).not.toMatch(/\d{1,2}:\d{2}/);
  });

  it('asks before ending a spell and ends it only when confirmed', async () => {
    const bless = outsideCast({ lasts: true, durationSeconds: 60, concentrating: true });
    const { api, press, settle } = await setup(false, (a) => (a.active = [bless]));
    await press('Encerrar');
    const dialog = document.querySelector('app-cast-confirm-sheet');
    expect(plain(dialog?.textContent)).toContain('Encerrar Bênção de Ilaria?');
    expect(api.ended).toEqual([]);
    (
      Array.from(dialog!.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Cancelar'),
      ) as HTMLElement
    ).click();
    await settle();
    expect(api.ended).toEqual([]);
    await press('Encerrar');
    const again = document.querySelector('app-cast-confirm-sheet')!;
    (
      Array.from(again.querySelectorAll('button')).find(
        (b) => plain(b.textContent) === 'Encerrar',
      ) as HTMLElement
    ).click();
    await settle();
    expect(api.ended).toEqual(['cast-1']);
    TestBed.inject(MatDialog).closeAll();
  });

  it('reads the log, and a failed cast says it spent nothing', async () => {
    const failed = outsideCast({
      id: 'f',
      spellNamePt: 'Oração de Cura',
      status: OutsideCastStatus.FAILED,
      endReason: OutsideCastEnd.INTERRUPTED,
    });
    const { el } = await setup(false, (a) => (a.log = [failed]));
    expect(plain(el.querySelector('.casting__log')?.textContent)).toContain('espaço não foi gasto');
  });

  it('says nothing was cast when the log is empty', async () => {
    const { el } = await setup(false, () => undefined);
    expect(plain(el.textContent)).toContain('Nada foi conjurado nesta sessão ainda.');
  });
});
