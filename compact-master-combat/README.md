# compact-master-combat: o combate do mestre, alternativa B "Três colunas", com a opção C (C7-02, C7-03)

Ramo `feat/compact-master-combat`. **Capturas do harness, não da pilha real** (a VM não tem Docker nem CockroachDB): os componentes reais (`CombatView`, `OrderList`, `CombatBar`, `NpcCard`, `CombatMapCard`, `EffectsPanel`, `SessionRail`, `MasterComposer`, `SessionTabs`) numa página com um cliente de combate e um mapa falsos. Os painéis de névoa, armadilhas e tesouro são *stand-ins* simples (cartões com título): o chrome deles é tirado como o dos painéis reais.

Cada arquivo termina com a largura e o tema (`-claro`, `-escuro`); há os dois temas de cada tela.

| Arquivo (prefixo) | O que mostra |
| --- | --- |
| `01-computador-6-jogadores-1280-claro.png`, `01-computador-6-jogadores-1280-escuro.png` | 1280, seis combatentes: a barra, a ordem, o mapa e o trilho "Turno · Conversa" (contagem "1" no "Turno") |
| `02-computador-8-jogadores-rolado-1280-claro.png`, `02-computador-8-jogadores-rolado-1280-escuro.png` | 1280, oito jogadores, página rolada: a ordem gruda abaixo da barra, "Mais 4 abaixo" |
| `03-computador-1-jogador-1280-claro.png`, `03-computador-1-jogador-1280-escuro.png` | 1280, um jogador |
| `04-linha-aberta-1280-claro.png`, `04-linha-aberta-1280-escuro.png` | Uma linha aberta no lugar: "CA 14 · 7 de 7 PV" e "Dano/Cura · Condições · Mais" em três colunas iguais |
| `05-mais-do-combate-1280-claro.png`, `05-mais-do-combate-1280-escuro.png` | O trilho rolado até "Mais do combate", tudo fechado |
| `06-mais-efeitos-aberto-1280-claro.png`, `06-mais-efeitos-aberto-1280-escuro.png` | "Efeitos · 2" aberto no trilho (C7-02) |
| `07-mais-nevoa-e-luz-ordem-recolhida-8-1280-claro.png`, `07-mais-nevoa-e-luz-ordem-recolhida-8-1280-escuro.png` | Ordem recolhida, oito jogadores, "Névoa e luz" aberta no trilho (C7-03) |
| `08-mais-registro-aberto-1280-claro.png`, `08-mais-registro-aberto-1280-escuro.png` | "Registro" aberto no trilho |
| `09-coluna-conversa-1280-claro.png`, `09-coluna-conversa-1280-escuro.png` | A aba "Conversa" do trilho, com o compositor real: "Enviar" vazado, "Próximo turno" cheio |
| `10-um-pouco-menos-1100-1100-claro.png`, `10-um-pouco-menos-1100-1100-escuro.png` | 1100 px |
| `11-1024-1024-claro.png`, `11-1024-1024-escuro.png` | 1024 px |
| `12-preparo-da-iniciativa-1280-claro.png`, `12-preparo-da-iniciativa-1280-escuro.png` | Preparo da iniciativa, "Começar o combate" na barra |
| `13-teatro-da-mente-1280-claro.png`, `13-teatro-da-mente-1280-escuro.png` | Teatro da mente, 1280 |
| `14-tablet-turno-768-claro.png`, `14-tablet-turno-768-escuro.png` | 768 px, aba "Turno" |
| `15-tablet-ordem-8-768-claro.png`, `15-tablet-ordem-8-768-escuro.png` | 768 px, aba "Ordem", oito jogadores |
| `16-celular-turno-390-claro.png`, `16-celular-turno-390-escuro.png` | 390 px, aba "Turno": "Turno · 1" em âmbar, barra fixa com "Falta aplicar 5 de dano" e "Próximo turno", só "Combate" ativo na navegação |
| `17-celular-ordem-8-linha-aberta-390-claro.png`, `17-celular-ordem-8-linha-aberta-390-escuro.png` | 390 px, aba "Ordem" (sem título repetido), oito jogadores, uma linha aberta |
| `18-celular-ordem-1-390-claro.png`, `18-celular-ordem-1-390-escuro.png` | 390 px, aba "Ordem", um jogador |
| `19-celular-mapa-390-claro.png`, `19-celular-mapa-390-escuro.png` | 390 px, aba "Mapa" |
| `20-celular-mais-aberto-390-claro.png`, `20-celular-mais-aberto-390-escuro.png` | 390 px, "Mais" aberto com "Efeitos" aberto |
| `21-celular-preparo-390-claro.png`, `21-celular-preparo-390-escuro.png` | 390 px, preparo da iniciativa |
| `22-celular-teatro-390-claro.png`, `22-celular-teatro-390-escuro.png` | 390 px, teatro da mente |

A auditoria de layout (`auditLayout`, `e2e/tests/layout.ts`) não achou nada a 320, 390, 1024 e 1280, claro e escuro, com um, seis e oito jogadores, no preparo e no teatro, inclusive com todas as linhas de "Mais do combate" abertas.

## Medidas e provas

- **Enviar vazado** (`09`): no compositor real, `Enviar` tem fundo da superfície e borda de 1 px (claro `rgb(255,255,255)`, escuro `rgb(26,31,41)`); `Próximo turno` é o único botão cheio (claro `rgb(158,43,59)`, escuro `rgb(232,133,147)`).
- **Linha selecionada no escuro** (`04`): o texto secundário (`Inic. 14 · CA 14`) é `#a3abb9` sobre `#3a1e24`, 6,5:1 (o mínimo é 4,5:1); a linha `CA 14 · 7 de 7 PV` é `#e6e9ef`, 12,4:1.
- **Alturas fixas**: os dois cantos de baixo da barra e do trilho vêm da barra medida (`--combat-top`), não de 164 px.

## Diferenças dos quadros

- O cartão do NPC no trilho do harness não traz o resultado da rolagem como o quadro; traz a linha "Brisa: 14 de 24 PV, depois 9" e os botões "Aplicar 5 de dano" e "Não aplicar".
- "Efeitos" leva o número de efeitos; "Armadilhas" e "Tesouro", os pontos do mapa. "Quebra-cabeças" ainda não leva número.
- O cartão do mapa tem a altura natural, não a da coluna; a coluna do mapa gruda e rola sozinha se estourar.
- O relógio dos turnos fica sob a lista de "Efeitos" (o quadro mostra só a lista e "Adicionar efeito"); "Exaustão" fica ao lado de "Adicionar efeito".
- Entre 1024 e 1279 px a ordem fica no trilho, sob "Turno"; o preparo da iniciativa tem uma coluna abaixo de 1280.
- No celular, "Grupo" na navegação ainda vai à página da campanha.
- Na linha aberta, o ND de um monstro é o primeiro item do menu "Mais", como pedido; "Revelado" não é mais uma linha.
