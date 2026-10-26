# table-effects-damage: dano extra com dados, opções de magia e dados de talento nas jogadas (feat/table-effects-damage)

**Capturas do harness, não da pilha real** (a VM não tem Docker nem CockroachDB): os componentes reais montados numa página descartável com dados de exemplo inventados e magias do SRD. Os arquivos são `<tela>-<before|after>-<light|dark>-<390|1280>.png`. "before" é o `main` atual (origin/main), "after" é o ramo (689eb202a). Larguras 390 e 1280, tema claro e escuro.

| Tela | O que mudou (compare before e after) |
| --- | --- |
| `editor-extra-damage-*` | O editor de efeito "Dano extra". Antes: "Onde o dado entra", "Valor" (fórmula, obrigatório) e o recurso. Depois: "Dano fixo", "Dados (quantos)", "Lados do dado" (4 a 12), "Tipo de dano", "Arma", "Quantas vezes" e a chave "Não dobra no acerto crítico", todos à mostra |
| `editor-spell-modifier-*` | O novo tipo "Opção de magia" (a linha "Tipos do menu" no alto mostra que ele entra na lista). Depois: três cartões, um por mudança: "Trocar o tipo de dano" (tipos que troca), "Dano extra" (dano fixo somado e tipo) e "Dados a mais por nível" (dados a mais por nível acima), cada um com "Vale para" e só os campos da sua mudança. Antes: o tipo não existe; o seletor fica no padrão e não há campos |
| `editor-superiority-die-*` | "Dado de manobra". Depois: "Onde o dado entra" ganha Teste de habilidade, Teste de resistência e Iniciativa; no teste de habilidade surge a lista de perícias/habilidades ("Investigação", "Testes de Destreza"), no teste de resistência a habilidade, e na iniciativa nenhum dos dois; nenhum custa uma ação. Antes: só as cinco opções de dano e ataque, um cartão |
| `cast-*` | A ficha "Conjurar Mísseis Mágicos" (SRD) ao lado da Metamagia. Depois: o painel "Opções das suas características" com "Mãos mornas" (sempre ativa, sem caixa), "Chama tingida" marcada com "Gasta 1 uso · restam 3" e a escolha do novo tipo de dano (Frio, Fogo, Elétrico), "Surto de energia" e "Reserva" recusada com o motivo ("Sem usos: volta num descanso longo."). Antes: só a Metamagia |
| `die-scene-*` | A ficha de rolagem do teste de cena (Investigação): depois, "Usar Faro fino" com o custo ("d8 · gasta 1 uso de dados de vigor") acima de "Rolar no app" |
| `die-effect-*` | O teste de resistência de efeito ("Fim do seu turno"): depois, "Usar Mente firme" com o custo |
| `die-concentration-*` | O teste de concentração ("Concentração em risco"): depois, "Usar Mente firme" com o custo |
| `die-initiative-*` | A tela de iniciativa do jogador ("Role a iniciativa"): depois, "Usar Passo vivo" com o custo |

## Limitações
- Dados de exemplo inventados (talentos "Mãos mornas", "Chama tingida", "Faro fino", ...); os menus do editor são montados à mão espelhando `tablemenu.go` de cada lado, não vêm do servidor.
- Antes, as telas `cast-*`, `die-*` recebem os mesmos dados que o depois: o que muda é só o que o componente do `main` ignora (sem `tableOptions`, sem `rollDice`).
- A captura de `cast-*` marca "Chama tingida" por um clique do harness para mostrar a escolha do tipo; nada é conjurado nem rolado, e "Usar ..." nas telas `die-*` aparece desmarcado.
- `editor-spell-modifier-before-*` mostra o editor vazio porque o tipo não existe no `main`.
- Não aparece: o servidor aplicando o dano extra e a troca de tipo, o gasto do recurso, o dado somado ao resultado, e a rolagem física do dado ("Resultado do d8 ..."), que os testes do ramo cobrem.
- O cabeçalho "Servidor indisponível" e o rodapé são o app sem servidor.
