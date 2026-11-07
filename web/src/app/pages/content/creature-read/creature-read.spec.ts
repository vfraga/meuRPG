import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { ContentSchema, CreatureSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableCreatureSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { catalogVm } from '../../../core/content/catalog';
import { entry } from '../../../core/content/content-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { settle } from '../../../../test-settle';
import { CreatureRead } from './creature-read';

describe('CreatureRead', () => {
  const statBlock = vi.fn();
  const text = (el: Element) => (el.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const wolf = entry(TableContentKind.CREATURE, 'Lobo do Vale', {
    body: {
      case: 'tableCreature',
      value: create(TableCreatureSchema, {
        baseKey: 'monster:wolf',
        damageMods: [{ kind: 'immune', damageTypes: ['damage-type:fire'] }],
      }),
    },
  });

  async function open() {
    TestBed.configureTestingModule({
      providers: [{ provide: CreaturesClient, useValue: { statBlock } }],
    });
    const fixture = TestBed.createComponent(CreatureRead);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('entry', wolf);
    fixture.componentRef.setInput('catalog', catalogVm(create(ContentSchema, {}), []));
    await settle(fixture, 4);
    return fixture.nativeElement as HTMLElement;
  }

  it("writes the table's lines at once, then the base's when its stat block comes", async () => {
    statBlock
      .mockReset()
      .mockResolvedValue(
        create(CreatureSchema, { summary: { namePt: 'Lobo', typePt: 'fera', sizePt: 'Médio' } }),
      );
    const el = await open();
    expect(statBlock).toHaveBeenCalledWith('camp-1', 'monster:wolf');
    expect(text(el)).toContain('Lobo (SRD), fera média');
    expect(text(el)).toContain('(da mesa)');
    expect(text(el)).toContain('Só você muda estas linhas.');
  });

  it("keeps the table's lines when the stat block does not come", async () => {
    statBlock.mockReset().mockRejectedValue(new Error('offline'));
    const el = await open();
    expect(text(el)).toContain('wolf (SRD)');
    expect(text(el)).not.toContain('fera');
    expect(text(el)).toContain('Imunidade');
  });
});
