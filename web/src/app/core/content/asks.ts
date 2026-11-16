/** What a feat or an option asks of the one who takes it, besides the older fields (#580): the same five values in every draft. */
export interface Asks {
  classKey: string;
  classLevel: string;
  requiresPact: string;
  requiresSpell: string;
  requiresOptions: string[];
}
