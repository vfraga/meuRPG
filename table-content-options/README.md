# Opções da mesa para os conjuntos de opções do SRD (`feat/table-content-options`)

Capturas da página de conteúdo da mesa e do editor de opção, em 390, 768 e 1280 px, tema claro (`-claro`) e escuro (`-escuro`).

| Arquivo | O que mostra |
| --- | --- |
| `antes-menu-conteudo-*` | O menu de tipos de conteúdo sem "Opções" (como estava). |
| `depois-menu-conteudo-*` | O mesmo menu com "Opções" e a contagem de entradas (em 390 px o menu é o seletor "Tipo"). |
| `depois-nova-opcao-*` | O formulário de uma opção nova: seletor de conjunto, nome, efeito, texto e "O que a opção pede". |
| `depois-olhar-do-corvo-*` | Uma opção salva ("Olhar do Corvo", uma invocação inventada que pede nível 5 de Bruxo e o Pacto do Tomo), com o conjunto fixo, o pré-requisito, a prévia "Como os jogadores veem" e a chave "Disponível para os jogadores". |

Como foram geradas: `ng build` de uma cópia de `feat/table-content-options` com uma cena de teste que monta `ContentList` e `OptionEditor` com os dados de teste do próprio código (nada de servidor); o "antes" é a mesma tela com o tipo "Opções" tirado do menu. As setas e ícones aparecem como texto (a fonte de ícones não carrega fora da rede), o que não existe no app.
