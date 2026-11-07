import { TestBed } from '@angular/core/testing';

import { type EffectDraft, choiceUses, emptyEffect } from '../../../../core/content/effect-draft';
import { FeatChoices } from './feat-choices';

const pick = (choice: string, name: string): EffectDraft => ({
  ...emptyEffect('choice'),
  choice,
  name,
  count: 1,
});
const mod = (patch: Partial<EffectDraft>): EffectDraft => ({
  ...emptyEffect('modifier'),
  ...patch,
});

describe('FeatChoices', () => {
  function draw(effects: EffectDraft[]): string {
    TestBed.resetTestingModule();
    const fixture = TestBed.createComponent(FeatChoices);
    fixture.componentRef.setInput('uses', choiceUses(effects));
    fixture.detectChanges();
    return ((fixture.nativeElement as HTMLElement).textContent ?? '').replace(/\s+/g, ' ').trim();
  }

  it('lists each named choice with its effect and the effects that use it', () => {
    const text = draw([
      pick('ability', 'habilidade'),
      mod({ target: 'score.@habilidade' }),
      mod({ proficiency: 'save.@habilidade' }),
    ]);
    expect(text).toContain('Escolhas deste talento');
    expect(text).toContain('Habilidade · efeito 1 usada em 2 efeitos');
    expect(text).toContain(
      'Você não digita nada: "Habilidade" vem antes dos valores fixos nos campos que aceitam uma habilidade.',
    );
  });

  it('says "1 efeito" for one and "ainda não" for none, with the gender of the kind', () => {
    const text = draw([
      pick('ability', 'a'),
      pick('damage_type', 't'),
      mod({ proficiency: 'save.@a' }),
    ]);
    expect(text).toContain('Habilidade · efeito 1 usada em 1 efeito');
    expect(text).toContain('Tipo de dano · efeito 2 ainda não usado');
    expect(text).toContain(
      '"Habilidade" vem antes dos valores fixos nos campos que aceitam uma habilidade; "Tipo de dano" vem antes dos valores fixos nos campos que aceitam um tipo de dano.',
    );
  });

  it('numbers two choices of a kind as the player will see them', () => {
    const text = draw([pick('ability', 'a'), pick('ability', 'b')]);
    expect(text).toContain('Habilidade (1ª) · efeito 1');
    expect(text).toContain('Habilidade (2ª) · efeito 2');
  });

  it('draws nothing without a named choice', () => {
    expect(draw([mod({})])).toBe('');
  });
});
