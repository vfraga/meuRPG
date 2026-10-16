import { create } from '@bufbuild/protobuf';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import { MapPointKind, MapPointSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { type LightOption, LightPresets } from '../../../core/maps/light-presets';
import type { LightReach } from '../../../shared/editor-overlay/editor-overlay';
import { LightPointPanel } from './light-point-panel';
import { nextTurn } from '../../../../test-settle';

const options: LightOption[] = [
  {
    key: 'light:torch',
    name: 'Tocha',
    radii: '6 m claro + 6 m de penumbra',
    brightFt: 20,
    dimFt: 20,
  },
  {
    key: 'light:hooded-lantern',
    name: 'Lanterna coberta',
    radii: '9 m claro + 9 m de penumbra',
    brightFt: 30,
    dimFt: 30,
  },
  {
    key: 'light:daylight',
    name: 'Luz do Dia',
    radii: '18 m claro + 18 m de penumbra',
    brightFt: 60,
    dimFt: 60,
  },
];

const torch = create(MapPointSchema, {
  id: 'l1',
  mapId: 'map-1',
  kind: MapPointKind.LIGHT,
  name: 'Tocha da guarita',
  xBp: 8000,
  yBp: 3000,
  light: { presetKey: 'light:torch', brightFt: 20, dimFt: 20 },
});
const custom = create(MapPointSchema, {
  id: 'l2',
  mapId: 'map-1',
  kind: MapPointKind.LIGHT,
  name: 'Brasa do altar',
  xBp: 5000,
  yBp: 7000,
  light: { presetKey: '', brightFt: 15, dimFt: 15 },
});

describe('LightPointPanel', () => {
  let fixture: ComponentFixture<LightPointPanel>;
  let el: HTMLElement;
  let reach: (LightReach | null)[];

  async function setup(point = torch) {
    reach = [];
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: LightPresets, useValue: { list: () => Promise.resolve(options) } }],
    });
    fixture = TestBed.createComponent(LightPointPanel);
    fixture.componentRef.setInput('point', point);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentInstance.reachChange.subscribe((r) => reach.push(r));
    fixture.detectChanges();
    el = fixture.nativeElement;
    await settle();
  }
  const settle = async () => {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const text = () => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const radio = (t: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]')).find((b) =>
      b.textContent?.includes(t),
    )!;
  const panel = () => fixture.componentInstance;

  it('shows the sources of the SRD as radios, then "Personalizada", with the radii in metres', async () => {
    await setup();
    const names = Array.from(el.querySelectorAll('[role="radio"] b')).map((b) => b.textContent);
    expect(names).toEqual(['Tocha', 'Lanterna coberta', 'Luz do Dia', 'Personalizada']);
    expect(text()).toContain('6 m claro + 6 m de penumbra');
    expect(radio('Tocha').getAttribute('aria-checked')).toBe('true');
  });

  it('says the reach in squares for a preset', async () => {
    await setup();
    expect(text()).toContain(
      'Raios: 4 quadrados de luz clara e mais 4 de penumbra, até 8 quadrados no total.',
    );
  });

  it('tells the map the radii, in feet, at the point (never lit squares: no read gives the master those)', async () => {
    await setup();
    expect(reach.at(-1)).toEqual({ xBp: 8000, yBp: 3000, brightFt: 20, dimFt: 20 });
    radio('Luz do Dia').click();
    await settle();
    expect(reach.at(-1)).toEqual({ xBp: 8000, yBp: 3000, brightFt: 60, dimFt: 60 });
    expect(text()).toContain('As paredes cortam a luz de verdade só com a névoa ligada');
  });

  it('a custom light has the two radii in metres, with the squares counted under them', async () => {
    await setup(custom);
    expect(radio('Personalizada').getAttribute('aria-checked')).toBe('true');
    const fields = Array.from(el.querySelectorAll('mat-form-field')).map((f) =>
      f.querySelector('mat-label')?.textContent?.trim(),
    );
    expect(fields).toContain('Luz clara até');
    expect(fields).toContain('Mais penumbra até');
    expect(text()).toContain('múltiplos de 1,5 m (um quadrado)');
    expect(text()).toContain(
      'Raios: 3 quadrados de luz clara e mais 3 de penumbra, até 6 quadrados no total.',
    );
  });

  it('refuses a radius that is not a whole square, and saves the preset with its radii', async () => {
    await setup(custom);
    const bright = Array.from(el.querySelectorAll('input')).find((i) =>
      i.closest('mat-form-field')?.textContent?.includes('Luz clara até'),
    )!;
    bright.value = '4';
    bright.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(panel().changes()).toBeNull();
    await settle();
    expect(text()).toContain('Use múltiplos de 1,5 m (um quadrado), de 0 a 36 m.');
    expect(reach.at(-1)).toBeNull();
    bright.value = '6';
    bright.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(panel().changes()).toEqual({ light: { presetKey: '', brightFt: 20, dimFt: 15 } });
  });

  it("says it is the master's alone: players see the light, never the point", async () => {
    await setup();
    expect(text()).toContain('Este ponto é só do mestre. Os jogadores veem a luz, nunca o ponto.');
  });
});
