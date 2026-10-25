# Características de classe da mesa e troca de escolha (`feat/table-class-features`)

Capturas em 390, 768 e 1280 px, tema claro (`-claro`) e escuro (`-escuro`).

| Arquivo | O que mostra |
| --- | --- |
| `antes-menu-conteudo-*` | O menu de tipos de conteúdo sem "Características de classe" (como estava). |
| `depois-menu-conteudo-*` | O mesmo menu com o tipo "Características de classe" e a contagem de entradas (em 390 px o menu é o seletor "Tipo"). |
| `depois-nova-caracteristica-*` | O formulário de uma característica nova: classe, nível na classe, "Substitui", nome, efeito e texto. |
| `depois-rastro-do-corvo-*` | Uma característica salva ("Rastro do Corvo", inventada: Patrulheiro, nível 1, substitui Inimigo Favorito), com a prévia "Como os jogadores veem" e a chave "Disponível para os jogadores". |
| `antes-regras-da-mesa-*` | "Regras da mesa" no ponto onde fica o painel novo, sem ele. |
| `depois-regras-da-mesa-*` | O painel "Subida de nível" com a chave "Trocar uma escolha no aumento de atributo" (ligada, com o texto de ajuda). |
| `antes-subir-de-nivel-*` | A etapa de aumento de habilidade da subida de nível (Guerreiro 3 → 4) sem grupos de troca: só o incremento. |
| `depois-subir-de-nivel-*` | A mesma etapa com "Trocar uma escolha": grupo "Estilo de luta", abrir mão de Defesa, pegar Duelismo, e a linha "Trocar Estilo de Luta: Defesa por Estilo de Luta: Duelismo". |

Como foram geradas: `ng build` de uma cópia de `feat/table-class-features` com uma cena de teste que monta `ClassFeatureEditor`, `ContentList`, a página de regras da mesa e a etapa de habilidades com os dados de teste do próprio código (nada de servidor). Os "antes" são as mesmas telas sem o tipo no menu, com o painel escondido e sem `swapGroups`, não uma compilação do `main`. As setas e ícones aparecem como texto (a fonte de ícones não carrega fora da rede), o que não existe no app. Os nomes são do SRD 5.1 ou inventados.
