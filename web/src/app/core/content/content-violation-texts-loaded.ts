/** What the master reads on an entry that loads with a part left out (`loaded_with_issues`): a warning, not a refusal. */
/** The notice of an entry that loads with a part left out (`loaded_with_issues`): its bold lead. */
export const LOADED_LEAD = 'Esta entrada tem um aviso.';
/** What the notice says when the issue is one this app does not know. */
export const LOADED_GENERIC_TEXT =
  'Uma parte desta entrada não entra nas regras. Confira os campos e salve de novo.';
/** The item of the list of a choice of tools that no choice of tools offers (`effects[n].from[i]`), whose key is `name`. */
export const LOADED_TOOL_LIST_SHAPE = /\.effects\[\d+\]\.from\[\d+\]$/;
export function loadedToolListText(name: string): string {
  return `A lista de ferramentas cita «${name}», que não é uma ferramenta, uma arma nem outro tipo aceito. Essa opção não é oferecida. Edite a escolha para corrigir.`;
}
