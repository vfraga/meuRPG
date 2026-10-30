# table-effects-polish: acabamento das telas de efeitos da mesa (fix/table-effects-polish)

**Capturas do harness, não da pilha real** (a VM não tem Docker nem CockroachDB): os componentes reais montados numa página descartável com dados de exemplo inventados e magias do SRD. Os arquivos são `<tela>-<before|after>-<light|dark>-<390|1280>.png`. "before" é o `main` (dc0f0249d), "after" é o ramo (b7a8a74ac, mais um commit de status vazio). Larguras 390 e 1280, tema claro e escuro.

| Tela | O que mudou (compare before e after) |
| --- | --- |
| `cast-*` | A ficha "Conjurar Mísseis Mágicos" com "Chama tingida" marcada. Depois: "Novo tipo de dano · Chama tingida" vira três cartões de opção (Frio, Fogo, Elétrico) no mesmo estilo das outras opções, em vez de botões de rádio soltos; as linhas de opção usam o cartão temado de 44 px |
| `cast-focus-*` | O mesmo, com o foco do teclado em "Surto de energia" (Tab, depois foco programático): o anel de foco do cartão |
| `die-scene-*`, `die-effect-*`, `die-concentration-*`, `die-initiative-*` | O cartão "Usar <talento>" nas jogadas (teste de cena, teste de resistência de efeito, concentração, iniciativa): depois, o mesmo cartão temado das opções (caixa e borda do tema, 44 px), em vez da caixa nativa |
| `editor-extra-damage-*` | "Dano extra": "Dados (quantos)" passa a ter uma dica só (antes, a dica da fórmula vinha repetida); o campo do alvo se chama "Contra quem" (antes "Onde o dado entra"); a descrição sob "Efeito" ganha folga antes do rótulo do campo seguinte (antes encostava) |
| `editor-spell-modifier-*` | "Opção de magia" → "Trocar o tipo de dano": a descrição sob "Efeito" com a folga; o seletor "Adicionar tipos de dano que troca" inteiro, sem o texto sob a seta, em 390 e 1280 |
| `editor-superiority-die-*` | "Dado de manobra": o campo continua "Onde o dado entra" (só o "Dano extra" mudou para "Contra quem") |

## Limitações
- Dados de exemplo inventados; os menus do editor são montados à mão espelhando o servidor, o "Tipos do menu" no alto é uma linha do harness.
- Nas telas `die-*` o cartão aparece desmarcado; nada é rolado nem conjurado. Em `cast-*` o clique em "Chama tingida" é do harness.
- O "before" das telas `cast-*`, `die-*` e `editor-*` recebe os mesmos dados do "after"; só muda o componente.
- Não aparece: o servidor, o gasto de recurso e o dado somado ao resultado.

## O que ainda não está certo
- **Anel de foco cortado dos lados** (`cast-focus-after-*`): o cartão de opção com foco tem anel de 2 px com distância de 2 px e o rótulo ocupa a largura inteira do corpo da ficha (432 px em 1280), que tem `overflow: auto`; as laterais do anel ficam cortadas (cima e baixo aparecem). Visível em `cast-focus-after-dark-1280.png` e em 390.
- O texto longo de "Onde o dado entra" ("Teste de habilidade (todos, ou os de perícias e ...") é cortado em 1280, pela largura da coluna de 352 px e não pela seta; é o mesmo antes e depois.
