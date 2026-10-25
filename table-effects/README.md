# table-effects-speeds-spells: outros deslocamentos, respirar na água e magia concedida (feat/table-effects-speeds-spells)

**Capturas do harness, não da pilha real** (a VM não tem Docker nem CockroachDB): os componentes reais montados com dados de exemplo inventados e magias do SRD. Nome: `<tela>-<antes|depois>-<largura>-<claro|escuro>.png`; "antes" é o `main` (f28b9c12), "depois" é o ramo. Larguras: 390, 768 e 1280.

| Tela | O que mudou (compare antes e depois) |
| --- | --- |
| `editor-de-efeito-*` | O editor de um talento da mesa: o menu de efeitos ganha "Magia concedida" (magia, habilidade de conjuração, "a partir do nível", "cada conjuração gasta um uso de", "também com espaço de magia", "só como ritual", "conta como magia conhecida") e "Respira na água"; antes a única forma era uma nota com magias |
| `ficha-*` | A caixa "Deslocamento" ganha uma linha por deslocamento além do de caminhada ("Natação 9 m (30 pés)", "Voo 3 m (10 pés)") e "Respira na água"; surge o painel "Magias concedidas" (uma linha por magia com a origem, como se conjura e a CD e o ataque da concessão, ou o pedido da habilidade de conjuração) |
| `conjurar-fora-do-combate-depois-*` | A ficha "Conjurar fora do combate" mostra, numa magia concedida, "Com espaço de magia" e um cartão por concessão ("Graça de teste", "Dom do vale · À vontade"), sem espaço de magia para a concessão |
| `conjurar-no-combate-depois-*` | A ficha de conjuração do combate diz a origem e os usos ("Graça de teste · 1 uso") e, numa concessão que também aceita espaço, "Sem espaço (usa a concessão)" marcado |

## O que não tem captura de antes

As fichas de conjuração (fora e no combate) não mudam para uma magia da classe: a captura "depois" mostra o estado novo (uma magia concedida), que não existia.

## O que não tem captura, e por quê

- **A página do editor inteira:** o editor de efeitos é o mesmo componente em todas as páginas (talento, raça, antecedente, classe, subclasse); a captura é o cartão da característica do editor de talento, com o texto do cartão.
- **A escolha da habilidade de conjuração:** é uma escolha do motor de escolhas (o mesmo componente da ficha e do nível), com as opções Inteligência, Sabedoria e Carisma; coberta por `grantedspells_test.go`.
