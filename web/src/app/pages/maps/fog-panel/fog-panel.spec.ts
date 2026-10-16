import { ComponentFixture, TestBed } from '@angular/core/testing';

import { LightLevel, type Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient, mapMessage } from '../../../core/maps/maps-testing';
import { FogPanel } from './fog-panel';
import { nextTurn } from '../../../../test-settle';

describe('FogPanel', () => {
  let fixture: ComponentFixture<FogPanel>;
  let el: HTMLElement;
  let api: FakeMapsClient;
  let changed: MapMessage[];
  let forgotten: number;

  async function setup(map: MapMessage) {
    api = new FakeMapsClient();
    changed = [];
    forgotten = 0;
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [{ provide: MapsClient, useValue: api }] });
    fixture = TestBed.createComponent(FogPanel);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('map', map);
    fixture.componentInstance.changed.subscribe((m) => changed.push(m));
    fixture.componentInstance.forgotten.subscribe(() => forgotten++);
    fixture.detectChanges();
    el = fixture.nativeElement;
    await fixture.whenStable();
  }

  const text = () => (el.textContent ?? '').replace(/\s+/g, ' ');
  const settle = async () => {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const switchOf = (label: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('[role="switch"]')).find(
      (s) =>
        document.getElementById(s.getAttribute('aria-labelledby')!)?.textContent?.trim() === label,
    )!;
  const radio = (text: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]')).find((b) =>
      b.textContent?.trim().endsWith(text),
    )!;
  const button = (t: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim().endsWith(t))!;

  const on = mapMessage('map-1', 'A caverna', {
    gridColumns: 24,
    gridRows: 16,
    fogEnabled: true,
    baseLight: LightLevel.DARK,
    groupVision: false,
  });

  it("turns the fog on, and the light of base and the group's vision, each in its own call", async () => {
    await setup(
      mapMessage('map-1', 'A caverna', {
        gridColumns: 24,
        gridRows: 16,
        fogEnabled: false,
        baseLight: LightLevel.DARK,
      }),
    );
    switchOf('Ligar a névoa').click();
    await settle();
    expect(api.calls).toEqual(['setFog map-1 {"fogEnabled":true}']);
    expect(changed).toHaveLength(1);
    await setup(on);
    radio('Penumbra').click();
    await settle();
    switchOf('Visão do grupo').click();
    await settle();
    expect(api.calls).toEqual([
      'setFog map-1 {"baseLight":2}',
      'setFog map-1 {"groupVision":true}',
    ]);
  });

  it('shows the light of base as Claro, Penumbra or Escuro with the chosen one checked', async () => {
    await setup(on);
    expect(radio('Escuro').getAttribute('aria-checked')).toBe('true');
    expect(radio('Escuro').textContent).toContain('check');
    expect(radio('Claro').getAttribute('aria-checked')).toBe('false');
    expect(text()).toContain('Desligada: cada um vê o que o próprio personagem vê.');
  });

  it('tells the master what Claro does only while Claro is the light of base', async () => {
    await setup(on);
    expect(text()).not.toContain('os jogadores veem o mapa todo');
    await setup(
      mapMessage('map-1', 'A caverna', {
        gridColumns: 24,
        gridRows: 16,
        fogEnabled: true,
        baseLight: LightLevel.BRIGHT,
      }),
    );
    expect(text()).toContain(
      'Claro: os jogadores veem o mapa todo; criaturas só onde os personagens enxergam. Para uma sala secreta, use Penumbra.',
    );
  });

  it('without a grid the fog cannot be turned on and says why', async () => {
    await setup(mapMessage('map-1', 'Sem grade', { gridColumns: 0, gridRows: 0 }));
    const fog = switchOf('Ligar a névoa');
    expect(fog.getAttribute('aria-disabled')).toBe('true');
    expect(text()).toContain('Precisa da grade definida.');
    fog.click();
    radio('Penumbra').click();
    await settle();
    expect(api.calls).toEqual([]);
    expect(text()).not.toContain('Esquecer o que foi visto');
    expect(text()).toContain('Sem névoa, não há o que esquecer.');
  });

  it('"Esquecer o que foi visto" asks in place: nothing is sent before the second click, then the focus returns', async () => {
    await setup(on);
    button('Esquecer o que foi visto').click();
    await settle();
    expect(el.querySelector('h3')?.textContent).toContain('Esquecer o que foi visto?');
    expect(document.activeElement).toBe(el.querySelector('h3'));
    expect(text()).toContain('Isso não dá para desfazer.');
    expect(api.calls).toEqual([]);
    button('Voltar').click();
    await settle();
    expect(api.calls).toEqual([]);
    expect(document.activeElement).toBe(button('Esquecer o que foi visto'));
    button('Esquecer o que foi visto').click();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .filter((b) => b.textContent?.trim() === 'Esquecer o que foi visto')
      .at(-1)!
      .click();
    await settle();
    expect(api.calls).toEqual(['forgetVision map-1']);
    expect(forgotten).toBe(1);
    expect(text()).toContain('os jogadores voltaram a ver só o que o personagem deles vê agora');
  });
});
