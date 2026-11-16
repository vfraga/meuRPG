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

describe('SteedAsk: "Dispensar Estrela?" and "Libertar Estrela?", asked in place (#588, boards 588-24 and 588-25)', () => {
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

  it('asks the dismissal as an alertdialog: the title, what it does, "Voltar" first with the focus, the confirm marked', async () => {
    const { el, buttons } = await setup('dismiss');
    const ask = el.querySelector('[role=alertdialog]')!;
    expect(flat(el.querySelector(`#${ask.getAttribute('aria-labelledby')}`))).toBe(
      'Dispensar Estrela?',
    );
    expect(flat(el.querySelector(`#${ask.getAttribute('aria-describedby')}`))).toContain(
      'O vínculo fica',
    );
    expect(buttons().map((b) => flat(b))).toEqual(['Voltar', 'Dispensar Estrela']);
    expect(document.activeElement).toBe(buttons()[0]);
    expect(buttons()[0].hasAttribute('data-confirm')).toBe(false);
    expect(buttons()[1].hasAttribute('data-confirm')).toBe(true);
    expect(el.querySelector('.ask__chip')).toBeNull();
  });

  it('in a combat that holds the steed, adds the chip "Gasta a sua ação", with its dot', async () => {
    const { el } = await setup('dismiss', { spendsAction: true });
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
    expect(r.buttons().map((b) => flat(b))).toEqual(['Voltar', 'Libertar Estrela']);
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
    api.failWith = new ConnectError('down', Code.Unavailable);
    buttons()[1].click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('o servidor não respondeu');
    expect(refused).toEqual([]);
    expect(closed).toEqual([]);
    expect(buttons().every((b) => !b.disabled)).toBe(true);
  });

  it("to the master the sentence names the owner's sheet", async () => {
    const { el } = await setup('dismiss', { ownerView: false, ownerName: 'Lia' });
    expect(flat(el.querySelector('.ask__d'))).toContain('da ficha de Lia, e Lia desce de pé');
  });
});
