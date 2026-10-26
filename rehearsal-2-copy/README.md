# rehearsal-2-copy — três textos da tela do mestre no combate

Ramo `fix/rehearsal-2-copy`. Cada cena tem o antes (`antes-*`, o código de `origin/main`) e o depois (`depois-*`), a 390 e 1280 px, tema claro e escuro.

| Cena | Arquivos | O que mostra |
| --- | --- | --- |
| A. `rebuke` | `*-rebuke-*` | O jogador Seraphin já respondeu à Repreensão Infernal; falta o mestre rolar o teste de resistência da Armadura animada. Antes a etiqueta do cabeçalho dizia "Falta responder por Seraphin"; depois, "Falta o teste da Armadura animada". |
| A. `due` | `*-due-*` | A resposta do jogador ainda é devida (Escudo Arcano, Pensantus): o texto continua "Falta responder por Pensantus", igual antes e depois. |
| B. `joint` | `*-joint-*` | Turno conjunto (Pensantus e Brisa). A 1280 px é a barra do combate; a 390 px é a barra fixa do celular. Antes não havia "Próximo turno" e "Encerrar combate" ocupava o lugar dele; depois "Próximo turno" fica no lugar, tracejado e desligado, com o motivo "Cada um encerra a parte no painel Turno". |
| C. `idle1`, `idle2` | `*-idle1-*`, `*-idle2-*` | A pergunta ao passar a vez de quem ainda não agiu (Seraphin, Brisa). Antes: "Passar a vez de Brisa? Ela ainda não agiu." (e "Ele" para os outros); depois: "Passar a vez de Brisa, que ainda não agiu?". |

Método: os componentes reais (`CombatView`, `CombatBar`, `NextTurn`) montados com dados inventados dos helpers de teste do código, um build de desenvolvimento do `origin/main` e outro com as mudanças do ramo, capturados com o Chromium do Playwright. A fonte de ícones não carrega offline, então os nomes dos ícones aparecem como texto (por exemplo "sk" antes de "Próximo turno").
