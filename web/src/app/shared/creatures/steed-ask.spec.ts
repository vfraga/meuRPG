import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { FakeCreaturesClient, creature, flat } from '../../core/creatures/creatures-testing';
import { nextTurn } from '../../../test-settle';
import { SteedAsk } from './steed-ask';

describe('SteedAsk: "Dispensar Estrela?" and "Libertar Estrela?", asked in place', () => {
  let api: FakeCreaturesClient;

  async function setup(mode: 'dismiss' | 'release', inputs: Record<string, unknown> = {}) {
    api = new FakeCreaturesClient();
    const estrela = creature('steed-1', 'Estrela', { bonded: true, creatureType: 'celestial' });
    api.creatures = [estrela];
    TestBed.configureTestingModule({ providers: [{ provide: CreaturesClient, useValue: api }] });
    const fixture = TestBed.createComponent(SteedAsk);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('creature', estrela);
    fixture.componentRef.setInput('mode', mode);
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    const closed: boolean[] = [];
    const refused: string[] = [];
    fixture.componentInstance.closed.subscribe((c) => closed.push(c));
    fixture.componentInstance.refused.subscribe((r) => refused.push(r));
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await nextTurn();
      }
      fixture.detectChanges();
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const buttons = () => Array.from(el.querySelectorAll<HTMLButtonElement>('button'));
    return { fixture, el, buttons, closed, refused, settle };
  }

  const blocked = (reason: EncounterBlockedReason) =>
    new ConnectError('no', Code.FailedPrecondition, undefined, [
      { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
    ]);

  it('is a surface with a danger border, a 16 px bold sans title and "Voltar | action" as an equal pair', async () => {
    const { el, buttons } = await setup('dismiss');
    const ask = getComputedStyle(el.querySelector('.ask')!);
    // The colours are tokens the test page does not define; the e2e shots judge them.
    expect(ask.borderTopStyle).toBe('solid');
    expect(ask.borderTopWidth).toBe('1px');
    const title = getComputedStyle(el.querySelector('.ask__t')!);
    expect(title.fontFamily).toBe('var(--mr-font-sans)');
    expect(title.fontSize).toBe('var(--mr-font-16)');
    expect(title.fontWeight).toBe('700');
    const pair = el.querySelector('.pair')!;
    expect(Array.from(pair.children)).toEqual(buttons());
    expect(getComputedStyle(pair).gridTemplateColumns).toBe('minmax(0, 1fr) minmax(0, 1fr)');
    // A long name wraps inside its button instead of leaving it.
    expect(getComputedStyle(el.querySelector('.big--danger')!).whiteSpace).toBe('normal');
  });

  it('asks the dismissal as an alertdialog: the title, what it does, "Voltar" first with the focus, the confirm marked', async () => {
    const { el, buttons } = await setup('dismiss');
    const ask = el.querySelector('[role=alertdialog]')!;
    expect(flat(el.querySelector(`#${ask.getAttribute('aria-labelledby')}`))).toBe(
      'Dispensar Estrela?',
    );
    expect(flat(el.querySelector(`#${ask.getAttribute('aria-describedby')}`))).toContain(
      'O vínculo fica',
    );
    expect(buttons().map((b) => flat(b))).toEqual(['Voltar', 'Dispensar']);
    // The title names the steed; the button's name keeps it for a screen reader.
    expect(buttons()[1].getAttribute('aria-label')).toBe('Dispensar Estrela');
    // the confirm carries the "✕" of the card's action.
    expect(buttons()[1].querySelector('mat-icon')?.textContent?.trim()).toBe('close');
    expect(flat(el.querySelector('.ask__d'))).toBe(
      'Estrela some do mapa e da sua ficha. O vínculo fica: Convocar Montaria traz Estrela de volta. Fora de combate, não gasta nada.',
    );
    expect(document.activeElement).toBe(buttons()[0]);
    expect(buttons()[0].hasAttribute('data-confirm')).toBe(false);
    expect(buttons()[1].hasAttribute('data-confirm')).toBe(true);
    expect(el.querySelector('.ask__chip')).toBeNull();
  });

  it('in a combat that holds the steed, adds the chip "Gasta a sua ação", with its dot', async () => {
    const { el } = await setup('dismiss', { where: { combat: true, spends: true, mounted: true } });
    expect(flat(el.querySelector('.ask__d'))).toBe(
      'Estrela some do mapa e da sua ficha, e você desce de pé no quadrado onde Estrela estava. O vínculo fica: Convocar Montaria traz Estrela de volta.',
    );
    const chip = el.querySelector('.ask__chip');
    expect(flat(chip)).toBe('● Gasta a sua ação');
    expect(chip?.querySelector('[aria-hidden=true]')?.textContent).toBe('●');
  });

  it('"Voltar" closes with nothing sent', async () => {
    const { buttons, closed } = await setup('dismiss');
    buttons()[0].click();
    expect(closed).toEqual([false]);
    expect(api.dismiss).not.toHaveBeenCalled();
  });

  it('the dismissal keeps the bond (DISMISS) and the release ends it (RELEASE)', async () => {
    const d = await setup('dismiss');
    d.buttons()[1].click();
    await d.settle();
    expect(api.dismiss).toHaveBeenCalledWith('camp-1', 'steed-1', 'dismiss');
    expect(d.closed).toEqual([true]);

    TestBed.resetTestingModule();
    const r = await setup('release');
    expect(flat(r.el.querySelector('.ask__t'))).toBe('Libertar Estrela?');
    expect(flat(r.el.querySelector('.ask__d'))).toContain('O vínculo acaba');
    expect(r.buttons().map((b) => flat(b))).toEqual(['Voltar', 'Libertar']);
    r.buttons()[1].click();
    await r.settle();
    expect(api.dismiss).toHaveBeenCalledWith('camp-1', 'steed-1', 'release');
    expect(r.closed).toEqual([true]);
  });

  it("a refusal for the turn or the action goes to the card, in the owner's words, and nothing closes as changed", async () => {
    const { buttons, closed, refused, settle } = await setup('dismiss', { ownerName: 'Lia' });
    api.failWith = blocked(EncounterBlockedReason.NOT_YOUR_TURN);
    buttons()[1].click();
    await settle();
    expect(refused).toEqual(['Só na vez de Lia: dispensar gasta a ação.']);
    expect(closed).toEqual([]);
  });

  it('any other failure stays in the question, in words, with both buttons back', async () => {
    const { el, buttons, closed, refused, settle } = await setup('dismiss');
    // The live region is on the page before the error: it is what a screen reader announces.
    const live = el.querySelector('.mr-live[role=alert]')!;
    expect(live.textContent?.trim()).toBe('');
    api.failWith = new ConnectError('down', Code.Unavailable);
    buttons()[1].click();
    await settle();
    expect(el.querySelector('.mr-live[role=alert]')).toBe(live);
    expect(live.textContent).toContain('o servidor não respondeu');
    expect(live.querySelector('.err')?.hasAttribute('role')).toBe(false);
    expect(refused).toEqual([]);
    expect(closed).toEqual([]);
    expect(buttons().every((b) => !b.disabled)).toBe(true);
  });

  it('the release, out of a combat, says it spends nothing, and its confirm carries the broken link', async () => {
    const { el, buttons } = await setup('release');
    expect(flat(el.querySelector('.ask__t'))).toBe('Libertar Estrela?');
    expect(flat(el.querySelector('.ask__d'))).toBe(
      'O vínculo acaba: Estrela some da ficha e do mapa, e a próxima Convocar Montaria escolhe de novo a forma e o tipo. Fora de combate, não gasta nada.',
    );
    expect(buttons()[1].querySelector('mat-icon')?.textContent?.trim()).toBe('link_off');
    expect(flat(buttons()[1])).toBe('Libertar');
  });

  it('in a combat still being set up there is no chip: nothing is charged', async () => {
    const { el } = await setup('dismiss', {
      where: { combat: true, spends: false, mounted: false },
    });
    expect(el.querySelector('.ask__chip')).toBeNull();
    expect(flat(el.querySelector('.ask__d'))).not.toContain('não gasta nada');
  });

  it("to the master the sentence names the owner's sheet, and no chip even in a combat", async () => {
    const { el } = await setup('dismiss', {
      ownerView: false,
      ownerName: 'Lia',
      where: { combat: true, spends: true, mounted: true },
    });
    expect(flat(el.querySelector('.ask__d'))).toContain(
      'da ficha de Lia, e Lia desce de pé no quadrado onde Estrela estava',
    );
    expect(el.querySelector('.ask__chip')).toBeNull();
  });
});
