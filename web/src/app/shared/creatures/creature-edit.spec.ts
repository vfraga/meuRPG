import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { CreaturesClient } from '../../core/creatures/creatures-client';
import { FakeCreaturesClient, creature, flat, isOff } from '../../core/creatures/creatures-testing';
import { CreatureEdit, type EditMode } from './creature-edit';
import { nextTurn } from '../../../test-settle';

describe('CreatureEdit: the questions a card asks in place', () => {
  let api: FakeCreaturesClient;

  async function setup(mode: EditMode, over: { ownerView?: boolean } = {}) {
    api = new FakeCreaturesClient();
    TestBed.configureTestingModule({ providers: [{ provide: CreaturesClient, useValue: api }] });
    Element.prototype.scrollIntoView = vi.fn();
    const fixture = TestBed.createComponent(CreatureEdit);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput(
      'creature',
      creature('cr-1', 'Nanquim', { hitPointsCurrent: 1, hitPointsMax: 1 }),
    );
    fixture.componentRef.setInput('mode', mode);
    fixture.componentRef.setInput('ownerView', over.ownerView ?? true);
    fixture.componentRef.setInput('ownerName', 'Pensantus');
    const closed = vi.fn();
    fixture.componentInstance.closed.subscribe(closed);
    const settle = async () => {
      for (let i = 0; i < 5; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await nextTurn();
      }
      fixture.detectChanges();
    };
    // Attached, so the focus the component moves is the document's.
    document.body.appendChild(fixture.nativeElement);
    fixture.detectChanges();
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        flat(b)?.includes(name),
      )!;
    return { fixture, el, flat, button, closed, settle };
  }

  it('dismiss: an alertdialog that names the consequence, with the focus on "Voltar"', async () => {
    const { el, flat, button } = await setup('dismiss');
    const ask = el.querySelector('[role=alertdialog]')!;
    expect(flat(ask.querySelector('.ask__t'))).toBe('Dispensar Nanquim?');
    expect(flat(ask.querySelector('.ask__d'))).toContain(
      'Para ter um familiar de novo, é preciso conjurar Convocar Familiar outra vez (ritual de 1 hora).',
    );
    expect(document.activeElement).toBe(button('Voltar'));
    expect(api.dismiss).not.toHaveBeenCalled();
  });

  it('dismiss: the second button sends the dismissal and closes with "changed"; "Voltar" closes with nothing changed', async () => {
    const a = await setup('dismiss');
    a.button('Dispensar Nanquim').click();
    await a.settle();
    expect(api.dismiss).toHaveBeenCalledWith('camp-1', 'cr-1');
    expect(a.closed).toHaveBeenCalledWith(true);

    TestBed.resetTestingModule();
    const b = await setup('dismiss');
    b.button('Voltar').click();
    expect(b.closed).toHaveBeenCalledWith(false);
    expect(api.dismiss).not.toHaveBeenCalled();
  });

  it("the master's dismissal says whose sheet it leaves", async () => {
    const { flat, el } = await setup('dismiss', { ownerView: false });
    expect(flat(el.querySelector('.ask__d'))).toContain(
      'Nanquim some da ficha de Pensantus e do mapa.',
    );
  });

  it('rename: the field starts with the name, counts "7 de 40" and saves only a changed, non-blank name', async () => {
    const { el, button, fixture, settle, closed } = await setup('rename');
    const input = el.querySelector<HTMLInputElement>('input')!;
    expect(input.value).toBe('Nanquim');
    expect(input.maxLength).toBe(40);
    expect(el.querySelector('mat-hint')?.textContent).toContain('7 de 40');
    expect(isOff(button('Salvar o nome'))).toBe(true);
    expect(flat(el.querySelector('.why'))).toBe('Escreva um nome diferente do atual.');
    input.value = 'Tinta';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await settle();
    expect(isOff(button('Salvar o nome'))).toBe(false);
    button('Salvar o nome').click();
    await settle();
    expect(api.rename).toHaveBeenCalledWith('camp-1', 'cr-1', 'Tinta');
    expect(closed).toHaveBeenCalledWith(true);
  });

  it('hp (the master): sets the hit points from 0 to the maximum, and a refusal stays in place in words', async () => {
    const { el, button, fixture, settle, closed, flat } = await setup('hp');
    const input = el.querySelector<HTMLInputElement>('input')!;
    expect(flat(el.querySelector('mat-hint'))).toBe('De 0 a 1. Com 0, a criatura vai embora.');
    input.value = '5';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await settle();
    expect(isOff(button('Corrigir os PV'))).toBe(true);
    expect(flat(el.querySelector('.why'))).toBe('Escreva um número inteiro de 0 a 1.');
    input.value = '0';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await settle();
    api.setHitPoints.mockRejectedValueOnce(new ConnectError('x', Code.FailedPrecondition));
    button('Corrigir os PV').click();
    await settle();
    expect(api.setHitPoints).toHaveBeenCalledWith('camp-1', 'cr-1', 0);
    expect(closed).not.toHaveBeenCalled();
    expect(el.querySelector('[role=alert]')).not.toBeNull();
  });
});
