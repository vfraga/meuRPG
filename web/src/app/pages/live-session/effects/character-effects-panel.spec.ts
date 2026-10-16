import { TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { EffectAudience, EffectEndScope } from '../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { EffectsClient } from '../../../core/effects/effects-client';
import { catalogEffect, characterEffect } from '../../../core/effects/effects-testing';
import { pensantusVitals } from '../../../core/play/live-testing';
import { CharacterEffectsPanel } from './character-effects-panel';
import { isOff } from './effects-dialogs-testing';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

describe('CharacterEffectsPanel (outside a combat)', () => {
  const listCharacterEffects = vi.fn();
  const end = vi.fn();
  const advanceTime = vi.fn();
  const catalog = vi.fn();

  beforeEach(() => {
    listCharacterEffects.mockReset().mockResolvedValue([
      characterEffect({ id: 'fx1' }),
      characterEffect({
        id: 'fx2',
        characterId: 'ch2',
        characterName: 'Ragna',
        sourceNamePt: 'Bênção',
        durationTextPt: 'dura enquanto a conjuradora se concentrar',
        concentration: true,
        tagsPt: ['+1d4 em ataques e resistências'],
        playerVisible: false,
        audience: EffectAudience.OWNER,
      }),
    ]);
    end.mockReset().mockResolvedValue({ ended: 1, createdEffectIds: [] });
    advanceTime.mockReset().mockResolvedValue(2);
    catalog
      .mockReset()
      .mockResolvedValue([catalogEffect({ key: 'spell:bless', namePt: 'Bênção' })]);
    TestBed.configureTestingModule({
      providers: [
        { provide: EffectsClient, useValue: { listCharacterEffects, end, advanceTime, catalog } },
      ],
    });
  });

  afterEach(() => document.body.replaceChildren());

  async function render() {
    const fixture = TestBed.createComponent(CharacterEffectsPanel);
    document.body.appendChild(fixture.nativeElement);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('party', [
      pensantusVitals({ characterId: 'ch1', name: 'Toren', exhaustionLevel: 2 }),
      pensantusVitals({ characterId: 'ch2', name: 'Ragna' }),
    ]);
    fixture.detectChanges();
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
      await fixture.whenStable();
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const card = (name: string) =>
      Array.from(el.querySelectorAll('li.cx__card')).find(
        (c) => flat(c.querySelector('h3')) === name,
      ) as HTMLElement;
    return { fixture, el, settle, card };
  }

  it('lists the effects on the characters, with what the players see', async () => {
    const { el, card } = await render();
    expect(listCharacterEffects).toHaveBeenCalledWith('camp');
    expect(flat(el.querySelector('h2'))).toBe('Efeitos em jogo');
    expect(flat(el.querySelector('.cx__count'))).toBe('2 efeitos · fora do combate');
    const first = card('Passos Largos');
    expect(flat(first.querySelector('.cx__sub'))).toBe('Em Toren');
    expect(flat(first.querySelector('.cx__line'))).toContain('dura 1 hora');
    expect(flat(first)).toContain('Jogadores veem: Sim');
    const second = card('Bênção');
    expect(flat(second.querySelector('.cx__pill'))).toContain('Concentração');
    expect(flat(second)).toContain('+1d4 em ataques e resistências');
    expect(flat(second)).toContain('Jogadores veem: Não');
  });

  it('ends an effect on a character with an empty encounter id', async () => {
    const { card, settle, el } = await render();
    (card('Passos Largos').querySelector('[data-end]') as HTMLButtonElement).click();
    await settle();
    expect(end).toHaveBeenCalledWith('camp', '', 'fx1', EffectEndScope.THIS, expect.any(String));
    expect(flat(el.querySelector('[role="status"]'))).toBe(
      'Efeito encerrado: Passos Largos em Toren.',
    );
    expect(listCharacterEffects).toHaveBeenCalledTimes(2);
  });

  it('asks before ending a concentration, in danger-outline with the focus on "Cancelar"', async () => {
    const { card, settle, el } = await render();
    const endButton = card('Bênção').querySelector('[data-end]') as HTMLButtonElement;
    endButton.click();
    await settle();
    const ask = el.querySelector('[role="alertdialog"]') as HTMLElement;
    expect(flat(ask.querySelector('h4'))).toBe('Encerrar Bênção em Ragna?');
    expect(ask.querySelector('.cx__btn--danger')).toBeTruthy();
    expect(document.activeElement).toBe(ask.querySelector('[data-initial-focus]'));
    expect(end).not.toHaveBeenCalled();
    (ask.querySelector('[data-initial-focus]') as HTMLButtonElement).click();
    await settle();
    expect(document.activeElement).toBe(endButton);
    endButton.click();
    await settle();
    (el.querySelector('.cx__btn--danger') as HTMLButtonElement).click();
    await settle();
    expect(end).toHaveBeenCalledWith(
      'camp',
      '',
      'fx2',
      EffectEndScope.CONCENTRATION_GROUP,
      expect.any(String),
    );
  });

  it('reads again when the page says something changed', async () => {
    const { fixture, settle } = await render();
    fixture.componentRef.setInput('tick', 1);
    fixture.detectChanges();
    await settle();
    expect(listCharacterEffects).toHaveBeenCalledTimes(2);
  });

  it('says there is nothing, and still offers the time', async () => {
    listCharacterEffects.mockResolvedValue([]);
    const { el } = await render();
    expect(flat(el.querySelector('.cx__empty'))).toBe('Nenhum efeito nos personagens agora.');
    expect(el.querySelector('[data-testid="pass-time"]')).toBeTruthy();
  });

  it('shows the refusal of an end and keeps the effect', async () => {
    end.mockRejectedValueOnce(new ConnectError('x', Code.NotFound));
    const { card, settle, el } = await render();
    (card('Passos Largos').querySelector('[data-end]') as HTMLButtonElement).click();
    await settle();
    expect(flat(el.querySelector('[role="alert"]'))).toContain('não existe mais');
    expect(card('Passos Largos')).toBeTruthy();
  });

  describe('"Passar o tempo"', () => {
    it('offers the four presets, the seconds and the button', async () => {
      const { el } = await render();
      const time = el.querySelector('[data-testid="pass-time"]') as HTMLElement;
      expect(flat(time.querySelector('h2'))).toBe('Passar o tempo');
      expect(Array.from(time.querySelectorAll('.cx__preset')).map(flat)).toEqual([
        '1 rodada (6 s)',
        '1 minuto',
        '10 minutos',
        '1 hora',
      ]);
      expect((time.querySelector('input[data-field="seconds"]') as HTMLInputElement).value).toBe(
        '60',
      );
      const pressed = Array.from(time.querySelectorAll('.cx__preset')).map((b) =>
        b.getAttribute('aria-pressed'),
      );
      expect(pressed).toEqual(['false', 'true', 'false', 'false']);
      expect(time.querySelector('[role="group"]')?.getAttribute('aria-label')).toBe(
        'Quanto tempo passa',
      );
    });

    it('moves the time the preset says and tells how many effects ended', async () => {
      const { el, settle } = await render();
      const time = el.querySelector('[data-testid="pass-time"]') as HTMLElement;
      (time.querySelectorAll('.cx__preset')[2] as HTMLButtonElement).click();
      await settle();
      expect(flat(time.querySelector('#time-preview'))).toBe('Vai passar 10 minutos.');
      expect(advanceTime).not.toHaveBeenCalled();
      (time.querySelector('.cx__btn--go') as HTMLButtonElement).click();
      await settle();
      expect(advanceTime).toHaveBeenCalledWith('camp', 600, expect.any(String));
      expect(flat(time.querySelector('.cx__done'))).toBe('Passou 10 minutos. 2 efeitos acabaram.');
      expect(listCharacterEffects).toHaveBeenCalledTimes(2);
    });

    it('takes any number of seconds from 1 to 86,400 and refuses the rest', async () => {
      const { el, settle } = await render();
      const time = el.querySelector('[data-testid="pass-time"]') as HTMLElement;
      const input = time.querySelector('input[data-field="seconds"]') as HTMLInputElement;
      const go = time.querySelector('.cx__btn--go') as HTMLButtonElement;
      const typeSeconds = async (v: string) => {
        input.value = v;
        input.dispatchEvent(new Event('input'));
        await settle();
      };
      await typeSeconds('0');
      expect(isOff(go)).toBe(true);
      expect(flat(time.querySelector('app-field-note'))).toContain(
        'Digite de 1 a 86.400 segundos.',
      );
      await typeSeconds('86401');
      expect(isOff(go)).toBe(true);
      await typeSeconds('86400');
      expect(isOff(go)).toBe(false);
      go.click();
      await settle();
      expect(advanceTime).toHaveBeenCalledWith('camp', 86400, expect.any(String));
    });

    it('says the time did not pass when the server refuses, and retries with the same key', async () => {
      advanceTime.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      const { el, settle } = await render();
      const go = el.querySelector('.cx__btn--go') as HTMLButtonElement;
      go.click();
      await settle();
      expect(flat(el.querySelector('.cx--time [role="alert"] p'))).toBe(
        'Não deu para passar o tempo: o servidor não respondeu. Tente de novo.',
      );
      go.click();
      await settle();
      expect(advanceTime.mock.calls[1][2]).toBe(advanceTime.mock.calls[0][2]);
      expect(flat(el.querySelector('.cx__done'))).toBe('Passou 1 minuto. 2 efeitos acabaram.');
    });
  });

  it('opens the exhaustion dialog with the party and the levels it has', async () => {
    const { el, settle } = await render();
    const open = vi.spyOn(TestBed.inject(MatDialog), 'open');
    (el.querySelector('[data-testid="open-exhaustion"]') as HTMLButtonElement).click();
    await settle();
    const config = open.mock.calls[0][1] as {
      data: { options: { key: string; label: string; level: number }[]; state: unknown };
    };
    expect(config.data.options.map((o) => [o.key, o.label, o.level])).toEqual([
      ['ch1', 'Toren', 2],
      ['ch2', 'Ragna', 0],
    ]);
    expect(config.data.state).toBeNull();
    TestBed.inject(MatDialog).closeAll();
  });

  it('reads the catalog and opens "Dar efeito" with the party and the effects', async () => {
    const { el, settle } = await render();
    const open = vi.spyOn(TestBed.inject(MatDialog), 'open');
    (el.querySelector('[data-testid="open-give-effect"]') as HTMLButtonElement).click();
    await settle();
    expect(catalog).toHaveBeenCalledWith('camp');
    const config = open.mock.calls[0][1] as {
      data: {
        campaignId: string;
        characters: { id: string; name: string }[];
        catalog: { key: string }[];
      };
    };
    expect(config.data.campaignId).toBe('camp');
    expect(config.data.characters).toEqual([
      { id: 'ch1', name: 'Toren' },
      { id: 'ch2', name: 'Ragna' },
    ]);
    expect(config.data.catalog.map((c) => c.key)).toEqual(['spell:bless']);
    TestBed.inject(MatDialog).closeAll();
  });

  it('says so, in Portuguese, when the catalog could not be read, and opens nothing', async () => {
    catalog.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
    const { el, settle } = await render();
    const open = vi.spyOn(TestBed.inject(MatDialog), 'open');
    (el.querySelector('[data-testid="open-give-effect"]') as HTMLButtonElement).click();
    await settle();
    expect(flat(el.querySelector('[role="alert"] p'))).toBe(
      'Não deu para ler o catálogo de efeitos: o servidor não respondeu. Tente de novo.',
    );
    expect(open).not.toHaveBeenCalled();
  });
});
