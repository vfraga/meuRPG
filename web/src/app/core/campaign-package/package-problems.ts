import {
  type PackageProblem,
  PackageProblemKind,
  PackageProblemReason,
} from '../../../gen/meurpg/campaignpackage/v1/campaignpackage_pb';
import { formatBytes } from '../images/image-format';
import { upFirst } from '../format/text';

/** The shape of `PackageProblem` this file reads (the generated message fits it), so a spec passes a plain object. */
export type ProblemLike = Pick<PackageProblem, 'kind' | 'name' | 'reason' | 'limit'>;

/** The limits of a package, as the proto states them (campaignpackage.proto, "Importing"). */
const KIB = 1024;
const MIB = KIB * KIB;
const MAX_PACKAGE_MIB = 200;
const MAX_ENTRY_MIB = 10;
export const MAX_PACKAGE_BYTES = MAX_PACKAGE_MIB * MIB;
export const MAX_PACKAGE_ENTRIES = 2000;
export const MAX_ENTRY_BYTES = MAX_ENTRY_MIB * MIB;

/** A name of the master's package is free text: cut it so one odd name cannot swamp the list. */
const NAME_MAX_LENGTH = 60;

/** From this many bytes a `limit` is read as a size, below it as a count (the gallery holds 300 images or 500 MB). */
const LIMIT_IS_BYTES_FROM = MIB;

/** How the package's things are named in a sentence: the article, the noun and the plural for a count. */
interface Noun {
  /** "a imagem": what a sentence starts with when the thing has a name. */
  readonly definite: string;
  /** "uma imagem": when the package did not name it. */
  readonly indefinite: string;
  /** What a count of them is called: "imagens". */
  readonly plural: string;
}

const NOUNS: Readonly<Record<PackageProblemKind, Noun>> = {
  [PackageProblemKind.UNSPECIFIED]: {
    definite: 'o item',
    indefinite: 'um item',
    plural: 'itens',
  },
  [PackageProblemKind.PACKAGE]: {
    definite: 'o arquivo',
    indefinite: 'um item do pacote',
    plural: 'itens',
  },
  [PackageProblemKind.CAMPAIGN]: {
    definite: 'a campanha',
    indefinite: 'a campanha',
    plural: 'campanhas',
  },
  [PackageProblemKind.IMAGE]: {
    definite: 'a imagem',
    indefinite: 'uma imagem',
    plural: 'imagens',
  },
  [PackageProblemKind.MAP]: { definite: 'o mapa', indefinite: 'um mapa', plural: 'mapas' },
  [PackageProblemKind.POINT]: {
    definite: 'o ponto do mapa',
    indefinite: 'um ponto de um mapa',
    plural: 'pontos de mapa',
  },
  [PackageProblemKind.SCENE]: { definite: 'a cena', indefinite: 'uma cena', plural: 'cenas' },
  [PackageProblemKind.NPC]: {
    definite: 'o NPC',
    indefinite: 'um NPC ou criatura',
    plural: 'NPCs e criaturas',
  },
  [PackageProblemKind.CHARACTER]: {
    definite: 'o personagem',
    indefinite: 'um personagem',
    plural: 'personagens',
  },
  [PackageProblemKind.PUZZLE]: {
    definite: 'o quebra-cabeça',
    indefinite: 'um quebra-cabeça',
    plural: 'quebra-cabeças',
  },
  [PackageProblemKind.CONTENT]: {
    definite: 'o item do conteúdo da mesa',
    indefinite: 'um item do conteúdo da mesa',
    plural: 'itens do conteúdo da mesa',
  },
  [PackageProblemKind.ENCOUNTER]: {
    definite: 'o encontro',
    indefinite: 'um encontro',
    plural: 'encontros',
  },
  [PackageProblemKind.DOCUMENT]: {
    definite: 'o documento',
    indefinite: 'o documento',
    plural: 'documentos',
  },
};

function nounOf(kind: PackageProblemKind): Noun {
  return NOUNS[kind] ?? NOUNS[PackageProblemKind.UNSPECIFIED];
}

/** The package's own name for a thing, made safe to print: no control characters, one space, cut when long. */
export function cleanName(name: string): string {
  const flat = name
    // eslint-disable-next-line no-control-regex
    .replace(/[\u0000-\u001f\u007f]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  return flat.length > NAME_MAX_LENGTH ? `${flat.slice(0, NAME_MAX_LENGTH - 1).trimEnd()}…` : flat;
}

/** "a imagem “Mapa antigo.png”", or "uma imagem" when the package named nothing. Lowercase start. */
function thing(problem: ProblemLike): string {
  const noun = nounOf(problem.kind);
  const name = cleanName(problem.name);
  return name === '' ? noun.indefinite : `${noun.definite} “${name}”`;
}

/** The count a LIMIT problem passed, in words: "300 imagens", or "500 MB" for a size. */
function limitWords(problem: ProblemLike): string {
  const limit = Number(problem.limit);
  if (limit <= 0) {
    return '';
  }
  return limit >= LIMIT_IS_BYTES_FROM
    ? formatBytes(limit)
    : `${limit.toLocaleString('pt-BR')} ${nounOf(problem.kind).plural}`;
}

const TEST_ACTION_SCENE = 'usa uma ação de teste que este servidor ainda não conhece.';
const UNKNOWN_CREATURE = 'usa uma criatura que este servidor ainda não conhece.';
const UNKNOWN_RULE = 'usa uma regra que este servidor ainda não conhece.';

/** UNKNOWN_CONTENT: what the thing uses that this server has never heard of, named by what the thing is. */
function unknownContent(kind: PackageProblemKind): string {
  switch (kind) {
    case PackageProblemKind.SCENE:
      return TEST_ACTION_SCENE;
    case PackageProblemKind.NPC:
    case PackageProblemKind.ENCOUNTER:
      return UNKNOWN_CREATURE;
    case PackageProblemKind.MAP:
    case PackageProblemKind.POINT:
      return 'usa uma armadilha pronta ou uma criatura que este servidor ainda não conhece.';
    case PackageProblemKind.CHARACTER:
      return 'usa uma classe, raça ou regra que este servidor ainda não conhece.';
    case PackageProblemKind.IMAGE:
    case PackageProblemKind.PACKAGE:
    case PackageProblemKind.UNSPECIFIED:
      return 'tem algo que este servidor ainda não conhece.';
    default:
      return UNKNOWN_RULE;
  }
}

/** NOT_IN_PACKAGE: what the thing uses that the package does not carry. */
function notInPackage(kind: PackageProblemKind): string {
  switch (kind) {
    case PackageProblemKind.CHARACTER:
      return 'usa uma classe da mesa que não está no conteúdo do pacote.';
    case PackageProblemKind.NPC:
    case PackageProblemKind.CONTENT:
      return 'usa algo da mesa (classe, raça, antecedente ou magia) que não está no conteúdo do pacote.';
    case PackageProblemKind.MAP:
    case PackageProblemKind.POINT:
    case PackageProblemKind.SCENE:
    case PackageProblemKind.PUZZLE:
    case PackageProblemKind.ENCOUNTER:
    case PackageProblemKind.DOCUMENT:
    case PackageProblemKind.CAMPAIGN:
      return 'usa um mapa ou uma imagem que não está no pacote.';
    default:
      return 'usa algo que não está no pacote.';
  }
}

/** LIMIT: "A campanha passa do limite de 300 imagens." (the number comes from the problem). */
function overLimit(problem: ProblemLike): string {
  const words = limitWords(problem);
  return words === ''
    ? 'A campanha passa de um dos limites que cada campanha tem.'
    : `A campanha passa do limite de ${words}.`;
}

/**
 * One problem of a package in a plain sentence for the master, by what it is (`kind`), its name and why
 * (`reason`): "A imagem “Mapa antigo.png” tem mais de 10 MB."
 *
 * It is total: every kind and every reason has words, and a value this app does not know (a newer server)
 * falls back to "Não dá para criar a cena “X”." It never prints an enum name, an entry path or a server
 * message, and the package's own name for the thing is cleaned (`cleanName`) because it is the master's free
 * text. Predicates avoid words that change with the noun's gender, so one sentence fits every kind.
 */
export function problemSentence(problem: ProblemLike): string {
  const words = SENTENCES[problem.reason];
  return words ? words(problem, upFirst(thing(problem))) : `Não dá para criar ${thing(problem)}.`;
}

type Sentence = (problem: ProblemLike, subject: string) => string;

/** One sentence per reason; `subject` is the thing with a capital ("A imagem “Mapa antigo.png”"). */
const SENTENCES: Readonly<Partial<Record<PackageProblemReason, Sentence>>> = {
  [PackageProblemReason.NOT_A_PACKAGE]: () =>
    'Este arquivo não é um pacote de campanha do MeuRPG, ou está sem a lista do que ele guarda.',
  [PackageProblemReason.NEWER_VERSION]: () =>
    'Este pacote é de uma versão mais nova do MeuRPG, e este servidor ainda não sabe abri-lo.',
  [PackageProblemReason.PACKAGE_TOO_BIG]: () =>
    `O pacote passa do limite de ${formatBytes(MAX_PACKAGE_BYTES)}.`,
  [PackageProblemReason.TOO_MANY_ENTRIES]: () =>
    `O pacote tem itens demais: o limite é ${MAX_PACKAGE_ENTRIES.toLocaleString('pt-BR')}.`,
  [PackageProblemReason.ENTRY_TOO_BIG]: (_p, subject) =>
    `${subject} tem mais de ${formatBytes(MAX_ENTRY_BYTES)}.`,
  [PackageProblemReason.COMPRESSION_RATIO]: (_p, subject) =>
    `${subject} se expande para muito mais do que ocupa no arquivo, e por segurança não é aceito.`,
  [PackageProblemReason.BAD_ENTRY_NAME]: (_p, subject) =>
    `${subject} tem um nome de arquivo que o MeuRPG não aceita.`,
  [PackageProblemReason.UNKNOWN_FIELD]: (_p, subject) =>
    `${subject} tem um dado que este servidor ainda não conhece.`,
  [PackageProblemReason.HASH_MISMATCH]: (_p, subject) =>
    `${subject} não confere com o que foi exportado: o arquivo foi alterado ou chegou com defeito.`,
  [PackageProblemReason.MISSING_ENTRY]: (_p, subject) =>
    `${subject} depende de algo que não está no pacote.`,
  [PackageProblemReason.UNEXPECTED_ENTRY]: (_p, subject) =>
    `${subject} não consta na lista do pacote, ou aparece duas vezes nela.`,
  [PackageProblemReason.INVALID]: (_p, subject) =>
    `${subject} tem dados fora do que o app aceita (um tamanho, um valor ou um nome).`,
  [PackageProblemReason.UNKNOWN_CONTENT]: (p, subject) => `${subject} ${unknownContent(p.kind)}`,
  [PackageProblemReason.NOT_IN_PACKAGE]: (p, subject) => `${subject} ${notInPackage(p.kind)}`,
  [PackageProblemReason.LIMIT]: (p) => overLimit(p),
  [PackageProblemReason.IMAGE_UNREADABLE]: (_p, subject) =>
    `${subject} não é um JPEG, PNG ou WebP que dê para ler.`,
  [PackageProblemReason.IMAGE_DIMENSIONS]: (_p, subject) =>
    `${subject} é grande demais: mais de 8192 pixels de lado ou 40 megapixels.`,
  [PackageProblemReason.DUPLICATE]: (_p, subject) =>
    `${subject} tem o mesmo nome de outro item do pacote.`,
};

/** Whether the package is from a newer MeuRPG: the preview then says so alone, in its own card. */
export function hasNewerVersion(problems: readonly Pick<PackageProblem, 'reason'>[]): boolean {
  return problems.some((p) => p.reason === PackageProblemReason.NEWER_VERSION);
}
