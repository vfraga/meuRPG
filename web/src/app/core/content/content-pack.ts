import { fromJson, toJson, type JsonValue } from '@bufbuild/protobuf';

import {
  type TableContentPack,
  TableContentPackSchema,
  type TableContentViolation,
  TableImportStatus,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { type ContentNavKind, CONTENT_NAV, navOfKind } from './content-kinds';
import { violationText } from './content-violations';

/**
 * The content pack as a file (MR-025): the campaign's own entries in proto JSON (the proto's field names), saved as
 * "<campanha>-conteudo.json" and loaded again by another campaign, or this one. A file the master chooses is untrusted:
 * it is checked here for the three things the server would only say after the upload (not JSON, too big, not a pack),
 * with a message in Portuguese, and then the server judges every entry. Nothing here knows a rule.
 */

export const PACK_FORMAT = 'meurpg.table-content';
/** The most a pack file may weigh (the server refuses more). */
const KIB = 1024;
const PACK_MAX_MIB = 2;
export const PACK_MAX_BYTES = PACK_MAX_MIB * KIB * KIB;

/** The file's text: proto JSON with the proto's own field names, two spaces of indent. */
export function packToText(pack: TableContentPack): string {
  return JSON.stringify(toJson(TableContentPackSchema, pack, { useProtoFieldName: true }), null, 2);
}

/** "Mesa de Mirathel" becomes "mesa-de-mirathel-conteudo.json". */
export function packFileName(campaignName: string): string {
  const slug = campaignName
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return `${slug === '' ? 'campanha' : slug}-conteudo.json`;
}

export type PackParse =
  | { readonly ok: true; readonly pack: TableContentPack; readonly text: string }
  | { readonly ok: false; readonly message: string };

const NOT_A_PACK =
  'Este arquivo não parece um pacote de conteúdo do MeuRPG. Escolha um arquivo exportado pelo app.';

/** Reads the file's text as a pack, or says in Portuguese why it is not one. `size` is the file's size in bytes. */
export function parsePack(text: string, size: number): PackParse {
  if (size > PACK_MAX_BYTES) {
    return {
      ok: false,
      message: 'O arquivo passou de 2 MiB. Divida o conteúdo em pacotes menores e tente de novo.',
    };
  }
  let json: JsonValue;
  try {
    json = JSON.parse(text) as JsonValue;
  } catch {
    return {
      ok: false,
      message: 'Este arquivo não é um JSON válido. Escolha um arquivo exportado pelo app.',
    };
  }
  if (typeof json !== 'object' || json === null || Array.isArray(json)) {
    return { ok: false, message: NOT_A_PACK };
  }
  if (json['format'] !== PACK_FORMAT) {
    return {
      ok: false,
      message:
        'Este arquivo não é um pacote de conteúdo do MeuRPG (o formato não é "meurpg.table-content"). Escolha um arquivo exportado pelo app.',
    };
  }
  try {
    const pack = fromJson(TableContentPackSchema, json);
    return { ok: true, pack, text };
  } catch {
    return {
      ok: false,
      message:
        'Não reconheci algum campo deste arquivo. Ele pode ser de outra versão do app, ou ter sido editado à mão. Exporte o pacote de novo e tente outra vez.',
    };
  }
}

/** "Texto", "Tabela dos níveis": what the field of `changed_fields` is, in our words (the field's own name when unknown). */
const FIELD_WORDS: Readonly<Record<string, string>> = {
  name_pt: 'o nome',
  desc_pt: 'o texto',
  higher_level_pt: 'o texto em níveis altos',
  levels: 'a tabela dos níveis',
  features: 'as características',
  traits: 'os traços',
  feature: 'a característica',
  effects: 'os efeitos',
  prerequisite: 'o pré-requisito',
  swap_levels: 'os níveis de troca de opção',
  hit_die: 'o dado de vida',
  saving_throws: 'os testes de resistência',
  skill_choose: 'quantas perícias escolhe',
  skill_from: 'a lista de perícias',
  skills: 'as perícias',
  tools: 'as ferramentas',
  proficiencies: 'as proficiências',
  casting: 'a conjuração',
  subclass_level: 'o nível da subclasse',
  asi_levels: 'os níveis de incremento de habilidade',
  class_key: 'a classe',
  race_key: 'a raça',
  races: 'as raças',
  lacks_feature_key: 'o que o personagem não pode ter',
  level: 'o nível',
  school_key: 'a escola',
  range: 'o alcance',
  target: 'o alvo',
  duration: 'a duração',
  casting_time: 'o tempo de conjuração',
  components: 'os componentes',
  damage: 'o dano',
  heal: 'a cura',
  attack: 'o ataque',
  save: 'o teste de resistência',
  concentration: 'a concentração',
  ritual: 'o ritual',
  class_keys: 'as classes',
  size: 'o tamanho',
  speed_ft: 'o deslocamento',
  darkvision_ft: 'a visão no escuro',
  ability_bonuses: 'os bônus de habilidade',
  choice_bonuses: 'os bônus à escolha',
  choice_bonus_sets: 'os grupos de bônus à escolha',
  sizes: 'os tamanhos à escolha',
  languages: 'os idiomas',
  language_choices: 'os idiomas à escolha',
  equipment_pt: 'o equipamento',
  always_prepared: 'as magias sempre preparadas',
  minimums: 'os mínimos de multiclasse',
  any_of: 'o mínimo de uma das habilidades',
};

/** "o nome, o texto e os efeitos": what an UPDATED entry changes, from the paths the server lists. */
export function changedText(fields: readonly string[]): string {
  const words: string[] = [];
  for (const path of fields) {
    // "table_class.levels" → "levels"; a path with no known word is "outros dados".
    const leaf = path.slice(path.lastIndexOf('.') + 1);
    const word = FIELD_WORDS[leaf] ?? 'outros dados';
    if (!words.includes(word)) {
      words.push(word);
    }
  }
  if (words.length === 0) {
    return 'outros dados';
  }
  return words.length === 1
    ? words[0]
    : `${words.slice(0, -1).join(', ')} e ${words[words.length - 1]}`;
}

const STATUS_WORDS: Readonly<Record<number, string>> = {
  [TableImportStatus.NEW]: 'Nova',
  [TableImportStatus.UPDATED]: 'Atualizada',
  [TableImportStatus.UNCHANGED]: 'Igual',
  [TableImportStatus.REFUSED]: 'Recusada',
};

export function statusWord(status: TableImportStatus): string {
  return STATUS_WORDS[status] ?? '';
}

/** What a refused entry or a refused pack says for one violation, and how to fix it, in Portuguese. */
export function packViolationText(
  v: Pick<TableContentViolation, 'field' | 'reason'>,
  aOne = 'uma entrada',
): string {
  const field = v.field;
  if (field === 'pack') {
    return 'O arquivo passou de 2 MiB. Divida o conteúdo em pacotes menores.';
  }
  if (field === 'pack.format') {
    return 'O formato do arquivo não é "meurpg.table-content". Use um arquivo exportado pelo app.';
  }
  if (field === 'pack.version') {
    return 'A versão do pacote não é a 1. Exporte o pacote de novo com esta versão do app.';
  }
  if (/^pack\.entries\[\d+\]\.key$/.test(field)) {
    return v.reason === 'duplicate_key'
      ? 'Duas entradas do arquivo têm a mesma chave. Tire uma delas.'
      : 'A chave desta entrada não combina com o tipo dela (tipo:nome@mesa). Corrija o campo "key" no arquivo.';
  }
  if (/^pack\.entries\[\d+\]\.body$/.test(field)) {
    return 'Esta entrada está sem conteúdo, ou o conteúdo é de outro tipo que o da chave. Corrija o arquivo.';
  }
  if (v.reason === 'unknown_field') {
    const name = field.slice(field.lastIndexOf('.') + 1).replace(/\[\d+\]$/, '');
    return `Campo desconhecido: ${name}. Confira a grafia no arquivo.`;
  }
  if (field.startsWith('pack')) {
    return 'O arquivo tem um problema que não dá para importar. Exporte o pacote de novo.';
  }
  return violationText(v, { aOne });
}

/** The entries of a preview counted by the left menu's kinds: "Classes 2 · Magias 5" (sub-races count with the races). */
export function countsByKind(
  entries: readonly { readonly kind: number }[],
): { nav: ContentNavKind; count: number }[] {
  return CONTENT_NAV.map((nav) => ({
    nav,
    count: entries.filter((e) => navOfKind(e.kind) === nav).length,
  })).filter((c) => c.count > 0);
}
