import { violationText } from './content-violations';

const CTX = { aOne: 'uma classe' };
const HOST = 'table_class.levels[1].features[0]';
const OPTION = `${HOST}.options[3]`;

describe('the sentences of the infusion refusals', () => {
  const say = (field: string, reason: string) => violationText({ field, reason }, CTX);

  it('says each field in Portuguese, in the words of its own field', () => {
    expect(say(`${OPTION}.infusion`, 'bad_value')).toContain('regra de infusões');
    expect(say(`${HOST}.infusion_rule`, 'bad_value')).toContain('regra de infusões');
    expect(say(`${HOST}.infusion_rule.limit_steps`, 'bad_value')).toBe(
      'A regra precisa de pelo menos um passo.',
    );
    expect(say(`${HOST}.infusion_rule.limit_steps[1].class_level`, 'bad_value')).toBe(
      'Os níveis dos passos sobem, cada um de 1 a 20.',
    );
    expect(say(`${HOST}.infusion_rule.limit_steps[0].active`, 'bad_value')).toContain(
      'pelo menos 1 infusão viva',
    );
    expect(say(`${OPTION}.infusion.accepts`, 'bad_value')).toContain('tipo de peça');
    expect(say(`${OPTION}.infusion.effect`, 'bad_value')).toContain('o que o app calcula');
    expect(say(`${OPTION}.infusion.bonus_steps`, 'bad_value')).toContain('passos por nível');
    expect(say(`${OPTION}.infusion.bonus_steps[0].class_level`, 'bad_value')).toContain('1 a 20');
    expect(say(`${OPTION}.infusion.bonus_steps[0].bonus`, 'bad_value')).toBe(
      'O bônus vai de +1 a +3.',
    );
    expect(say(`${OPTION}.infusion.damage_types`, 'bad_value')).toContain('tipos de dano');
    expect(say(`${OPTION}.infusion.damage_types[1]`, 'bad_value')).toContain('cada um uma vez');
    expect(say(`${OPTION}.infusion.damage_types[0]`, 'dangling_reference')).toBe(
      'Este tipo de dano não existe. Escolha outro.',
    );
  });

  it('reads a table option and a subclass feature the same way', () => {
    expect(say('table_option.infusion.effect', 'bad_value')).toContain('o que o app calcula');
    expect(say('table_subclass.levels[0].features[0].infusion_rule', 'bad_value')).toContain(
      'regra de infusões',
    );
  });
});
