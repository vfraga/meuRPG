// Read by the rule as the campaign names the specs create (a template literal that ends in the clock).
export const names = [`Nomes e legenda ${Date.now()}`, `Mesa de teste ${Date.now()}`];

async function atTable(name: string): Promise<string> {
  return `${name} ${Date.now()}`;
}
export const made = atTable('Pontes e fossos');
