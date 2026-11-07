import { TestBed } from '@angular/core/testing';

import type { TvMapVm } from '../../../core/play/table-screen-view';
import { fogMapBody, plainMapBody } from '../table-screen-testing';
import { TvMap } from './tv-map';

function vm(
  body: ReturnType<typeof fogMapBody> | ReturnType<typeof plainMapBody>,
  hiddenByFog = false,
): TvMapVm {
  return {
    name: body.name,
    hiddenByFog,
    map: body.map,
    vision: 'vision' in body ? body.vision : null,
    revision: body.revision,
  };
}

function render(map: TvMapVm) {
  const fixture = TestBed.createComponent(TvMap);
  fixture.componentRef.setInput('map', map);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TvMap', () => {
  it('draws a fog map from the tiles of the group, asked with as=tv', () => {
    const el = render(vm(fogMapBody()));
    expect(el.querySelector('app-fog-map')).not.toBeNull();
    const tiles = [...el.querySelectorAll('img')].map((i) => i.getAttribute('src'));
    expect(tiles).toContain('/images/maps/m1/tiles/0/0?r=3&as=tv');
    expect(el.querySelector('section')?.getAttribute('aria-label')).toBe('Mapa Cripta do Corvo');
  });

  it('puts no zoom, fit or full-screen buttons on the TV, nor the player card', () => {
    const el = render(vm(fogMapBody()));
    expect(el.querySelector('.fm__tools')).toBeNull();
    expect(el.querySelector('button')).toBeNull();
    expect(el.querySelector('[data-testid="fog-caption"]')).toBeNull();
  });

  it('draws a map with no fog as its picture', () => {
    const el = render(vm(plainMapBody()));
    expect(el.querySelector('app-fog-map')).toBeNull();
    expect(el.querySelector('app-map-view')).not.toBeNull();
    expect(el.querySelector('img[src="/images/img-m2"]')).not.toBeNull();
  });

  it('says the map is on each one’s screen when the fog hides it, and draws nothing of it', () => {
    const el = render({ ...vm(fogMapBody(), true), map: null, vision: null, name: '' });
    expect(el.querySelector('app-tv-fog-card')).not.toBeNull();
    expect(el.querySelector('app-fog-map')).toBeNull();
    expect(el.querySelector('img')).toBeNull();
  });

  it('says the same when the server sent no map data', () => {
    const el = render({ name: '', hiddenByFog: false, map: null, vision: null, revision: 0 });
    expect(el.textContent).toContain('O mapa fica na tela de cada um.');
  });
});
