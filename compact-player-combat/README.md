# Combate do jogador, telas compactas (segunda rodada)

Todas as capturas sao da **pilha real** (backend, devidp, CockroachDB e o app Angular compilado de `feat/compact-player-combat` @ 0ca126013), semeada pelos helpers do e2e (Playwright, Chromium); **nenhuma e harness**. Claro/escuro por `prefers-color-scheme`. 320, 390 e 768 em 2x; 1280 em 1x.

Notas: as capturas "fora da vez" vem depois das "acao usada" (Pensantus e Mira ja tinham gasto a Acao/o Bonus, por isso aparece "Esquivando"). Nao ha parede no mapa semeado nem Goblin 3, entao "cobertura total" e o alvo apagado com o motivo de C7-04 nao aparecem; o alvo apagado so existe fora de alcance (Goblin 2, aria-disabled "fora de alcance").

| Arquivo | Personagem | Estado | Largura | Tema |
|---|---|---|---|---|
| `alvo-1280-claro.png` | Pensantus, alvo no mapa | C7-05: "Toque no alvo", cartao pendente no trilho, linha "Escolhendo o alvo" | 1280 px | claro |
| `alvo-1280-escuro.png` | Pensantus, alvo no mapa | C7-05: "Toque no alvo", cartao pendente no trilho, linha "Escolhendo o alvo" | 1280 px | escuro |
| `alvo-adaga-escolher-1280-claro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 1280 px | claro |
| `alvo-adaga-escolher-1280-escuro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 1280 px | escuro |
| `alvo-adaga-escolher-320-claro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 320 px | claro |
| `alvo-adaga-escolher-320-escuro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 320 px | escuro |
| `alvo-adaga-escolher-390-claro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 390 px | claro |
| `alvo-adaga-escolher-390-escuro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 390 px | escuro |
| `alvo-adaga-escolher-768-claro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 768 px | claro |
| `alvo-adaga-escolher-768-escuro.png` | Pensantus, alvo no mapa | Adaga, escolhendo o alvo (Goblin 2 fora de alcance, apagado) | 768 px | escuro |
| `alvo-adaga-longe-1280-claro.png` | Pensantus, alvo no mapa | C7-05: Adaga em Goblin 1, Goblin 2 "Longe: desvantagem" (1280) | 1280 px | claro |
| `alvo-adaga-longe-1280-escuro.png` | Pensantus, alvo no mapa | C7-05: Adaga em Goblin 1, Goblin 2 "Longe: desvantagem" (1280) | 1280 px | escuro |
| `alvo-antes-de-rolar-1280-claro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 1280 px | claro |
| `alvo-antes-de-rolar-1280-escuro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 1280 px | escuro |
| `alvo-antes-de-rolar-320-claro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 320 px | claro |
| `alvo-antes-de-rolar-320-escuro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 320 px | escuro |
| `alvo-antes-de-rolar-390-claro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 390 px | claro |
| `alvo-antes-de-rolar-390-escuro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 390 px | escuro |
| `alvo-antes-de-rolar-768-claro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 768 px | claro |
| `alvo-antes-de-rolar-768-escuro.png` | Pensantus, alvo no mapa | C7-04 3: Adaga em Goblin 1 antes de rolar, tag Alvo, Confirmar cheio | 768 px | escuro |
| `alvo-fire-bolt-1280-claro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 1280 px | claro |
| `alvo-fire-bolt-1280-escuro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 1280 px | escuro |
| `alvo-fire-bolt-320-claro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 320 px | claro |
| `alvo-fire-bolt-320-escuro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 320 px | escuro |
| `alvo-fire-bolt-390-claro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 390 px | claro |
| `alvo-fire-bolt-390-escuro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 390 px | escuro |
| `alvo-fire-bolt-768-claro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 768 px | claro |
| `alvo-fire-bolt-768-escuro.png` | Pensantus, alvo no mapa | C7-04 1: Raio de Fogo, um alvo, barra "Escolha o alvo" | 768 px | escuro |
| `alvo-misseis-1280-claro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 1280 px | claro |
| `alvo-misseis-1280-escuro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 1280 px | escuro |
| `alvo-misseis-320-claro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 320 px | claro |
| `alvo-misseis-320-escuro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 320 px | escuro |
| `alvo-misseis-390-claro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 390 px | claro |
| `alvo-misseis-390-escuro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 390 px | escuro |
| `alvo-misseis-768-claro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 768 px | claro |
| `alvo-misseis-768-escuro.png` | Pensantus, alvo no mapa | C7-04 2: Misseis Magicos "2 de 3 dardos", x2 no token, Desfazer | 768 px | escuro |
| `conversa-1280-claro.png` | Pensantus | aba Conversa aberta no trilho (1280) | 1280 px | claro |
| `conversa-1280-escuro.png` | Pensantus | aba Conversa aberta no trilho (1280) | 1280 px | escuro |
| `conversa-aviso-1280-claro.png` | Pensantus | 3 mensagens novas: aviso no topo, selo na aba/nav Conversa | 1280 px | claro |
| `conversa-aviso-1280-escuro.png` | Pensantus | 3 mensagens novas: aviso no topo, selo na aba/nav Conversa | 1280 px | escuro |
| `conversa-aviso-390-claro.png` | Pensantus | 3 mensagens novas: aviso no topo, selo na aba/nav Conversa | 390 px | claro |
| `conversa-aviso-390-escuro.png` | Pensantus | 3 mensagens novas: aviso no topo, selo na aba/nav Conversa | 390 px | escuro |
| `mira-bonus-usada-aberta-1280-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 1280 px | claro |
| `mira-bonus-usada-aberta-1280-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 1280 px | escuro |
| `mira-bonus-usada-aberta-320-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 320 px | claro |
| `mira-bonus-usada-aberta-320-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 320 px | escuro |
| `mira-bonus-usada-aberta-390-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 390 px | claro |
| `mira-bonus-usada-aberta-390-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 390 px | escuro |
| `mira-bonus-usada-aberta-768-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 768 px | claro |
| `mira-bonus-usada-aberta-768-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo aberto | 768 px | escuro |
| `mira-bonus-usada-dobrada-1280-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 1280 px | claro |
| `mira-bonus-usada-dobrada-1280-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 1280 px | escuro |
| `mira-bonus-usada-dobrada-320-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 320 px | claro |
| `mira-bonus-usada-dobrada-320-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 320 px | escuro |
| `mira-bonus-usada-dobrada-390-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 390 px | claro |
| `mira-bonus-usada-dobrada-390-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 390 px | escuro |
| `mira-bonus-usada-dobrada-768-claro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 768 px | claro |
| `mira-bonus-usada-dobrada-768-escuro.png` | Mira (monge, aba Chi) | Acao bonus gasta, grupo dobrado | 768 px | escuro |
| `mira-fora-1280-claro.png` | Mira (monge, aba Chi) | fora da vez, topo | 1280 px | claro |
| `mira-fora-1280-escuro.png` | Mira (monge, aba Chi) | fora da vez, topo | 1280 px | escuro |
| `mira-fora-320-claro.png` | Mira (monge, aba Chi) | fora da vez, topo | 320 px | claro |
| `mira-fora-320-escuro.png` | Mira (monge, aba Chi) | fora da vez, topo | 320 px | escuro |
| `mira-fora-390-claro.png` | Mira (monge, aba Chi) | fora da vez, topo | 390 px | claro |
| `mira-fora-390-escuro.png` | Mira (monge, aba Chi) | fora da vez, topo | 390 px | escuro |
| `mira-fora-768-claro.png` | Mira (monge, aba Chi) | fora da vez, topo | 768 px | claro |
| `mira-fora-768-escuro.png` | Mira (monge, aba Chi) | fora da vez, topo | 768 px | escuro |
| `mira-fora-acoes-1280-claro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 1280 px | claro |
| `mira-fora-acoes-1280-escuro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 1280 px | escuro |
| `mira-fora-acoes-320-claro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 320 px | claro |
| `mira-fora-acoes-320-escuro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 320 px | escuro |
| `mira-fora-acoes-390-claro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 390 px | claro |
| `mira-fora-acoes-390-escuro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 390 px | escuro |
| `mira-fora-acoes-768-claro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 768 px | claro |
| `mira-fora-acoes-768-escuro.png` | Mira (monge, aba Chi) | fora da vez, area de acoes | 768 px | escuro |
| `mira-turno-1280-claro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 1280 px | claro |
| `mira-turno-1280-escuro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 1280 px | escuro |
| `mira-turno-320-claro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 320 px | claro |
| `mira-turno-320-escuro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 320 px | escuro |
| `mira-turno-390-claro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 390 px | claro |
| `mira-turno-390-escuro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 390 px | escuro |
| `mira-turno-768-claro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 768 px | claro |
| `mira-turno-768-escuro.png` | Mira (monge, aba Chi) | na vez, topo da tela | 768 px | escuro |
| `mira-turno-acoes-1280-claro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 1280 px | claro |
| `mira-turno-acoes-1280-escuro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 1280 px | escuro |
| `mira-turno-acoes-320-claro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 320 px | claro |
| `mira-turno-acoes-320-escuro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 320 px | escuro |
| `mira-turno-acoes-390-claro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 390 px | claro |
| `mira-turno-acoes-390-escuro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 390 px | escuro |
| `mira-turno-acoes-768-claro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 768 px | claro |
| `mira-turno-acoes-768-escuro.png` | Mira (monge, aba Chi) | na vez, rolado ate as abas de acao | 768 px | escuro |
| `pensantus-acao-usada-aberta-1280-claro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 1280 px | claro |
| `pensantus-acao-usada-aberta-1280-escuro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 1280 px | escuro |
| `pensantus-acao-usada-aberta-320-claro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 320 px | claro |
| `pensantus-acao-usada-aberta-320-escuro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 320 px | escuro |
| `pensantus-acao-usada-aberta-390-claro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 390 px | claro |
| `pensantus-acao-usada-aberta-390-escuro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 390 px | escuro |
| `pensantus-acao-usada-aberta-768-claro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 768 px | claro |
| `pensantus-acao-usada-aberta-768-escuro.png` | Pensantus (mago) | Acao gasta, grupo aberto | 768 px | escuro |
| `pensantus-acao-usada-dobrada-1280-claro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 1280 px | claro |
| `pensantus-acao-usada-dobrada-1280-escuro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 1280 px | escuro |
| `pensantus-acao-usada-dobrada-320-claro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 320 px | claro |
| `pensantus-acao-usada-dobrada-320-escuro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 320 px | escuro |
| `pensantus-acao-usada-dobrada-390-claro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 390 px | claro |
| `pensantus-acao-usada-dobrada-390-escuro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 390 px | escuro |
| `pensantus-acao-usada-dobrada-768-claro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 768 px | claro |
| `pensantus-acao-usada-dobrada-768-escuro.png` | Pensantus (mago) | Acao gasta, grupo "Acao · Usada" dobrado | 768 px | escuro |
| `pensantus-acao-usada-topo-320-claro.png` | Pensantus (mago) | Acao gasta, topo (cartao com Esquivando) | 320 px | claro |
| `pensantus-acao-usada-topo-320-escuro.png` | Pensantus (mago) | Acao gasta, topo (cartao com Esquivando) | 320 px | escuro |
| `pensantus-acao-usada-topo-390-claro.png` | Pensantus (mago) | Acao gasta, topo (cartao com Esquivando) | 390 px | claro |
| `pensantus-acao-usada-topo-390-escuro.png` | Pensantus (mago) | Acao gasta, topo (cartao com Esquivando) | 390 px | escuro |
| `pensantus-fora-1280-claro.png` | Pensantus (mago) | fora da vez, topo | 1280 px | claro |
| `pensantus-fora-1280-escuro.png` | Pensantus (mago) | fora da vez, topo | 1280 px | escuro |
| `pensantus-fora-320-claro.png` | Pensantus (mago) | fora da vez, topo | 320 px | claro |
| `pensantus-fora-320-escuro.png` | Pensantus (mago) | fora da vez, topo | 320 px | escuro |
| `pensantus-fora-390-claro.png` | Pensantus (mago) | fora da vez, topo | 390 px | claro |
| `pensantus-fora-390-escuro.png` | Pensantus (mago) | fora da vez, topo | 390 px | escuro |
| `pensantus-fora-768-claro.png` | Pensantus (mago) | fora da vez, topo | 768 px | claro |
| `pensantus-fora-768-escuro.png` | Pensantus (mago) | fora da vez, topo | 768 px | escuro |
| `pensantus-fora-acoes-1280-claro.png` | Pensantus (mago) | fora da vez, area de acoes | 1280 px | claro |
| `pensantus-fora-acoes-1280-escuro.png` | Pensantus (mago) | fora da vez, area de acoes | 1280 px | escuro |
| `pensantus-fora-acoes-320-claro.png` | Pensantus (mago) | fora da vez, area de acoes | 320 px | claro |
| `pensantus-fora-acoes-320-escuro.png` | Pensantus (mago) | fora da vez, area de acoes | 320 px | escuro |
| `pensantus-fora-acoes-390-claro.png` | Pensantus (mago) | fora da vez, area de acoes | 390 px | claro |
| `pensantus-fora-acoes-390-escuro.png` | Pensantus (mago) | fora da vez, area de acoes | 390 px | escuro |
| `pensantus-fora-acoes-768-claro.png` | Pensantus (mago) | fora da vez, area de acoes | 768 px | claro |
| `pensantus-fora-acoes-768-escuro.png` | Pensantus (mago) | fora da vez, area de acoes | 768 px | escuro |
| `pensantus-turno-1280-claro.png` | Pensantus (mago) | na vez, topo da tela | 1280 px | claro |
| `pensantus-turno-1280-escuro.png` | Pensantus (mago) | na vez, topo da tela | 1280 px | escuro |
| `pensantus-turno-320-claro.png` | Pensantus (mago) | na vez, topo da tela | 320 px | claro |
| `pensantus-turno-320-escuro.png` | Pensantus (mago) | na vez, topo da tela | 320 px | escuro |
| `pensantus-turno-390-claro.png` | Pensantus (mago) | na vez, topo da tela | 390 px | claro |
| `pensantus-turno-390-escuro.png` | Pensantus (mago) | na vez, topo da tela | 390 px | escuro |
| `pensantus-turno-768-claro.png` | Pensantus (mago) | na vez, topo da tela | 768 px | claro |
| `pensantus-turno-768-escuro.png` | Pensantus (mago) | na vez, topo da tela | 768 px | escuro |
| `pensantus-turno-acoes-1280-claro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 1280 px | claro |
| `pensantus-turno-acoes-1280-escuro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 1280 px | escuro |
| `pensantus-turno-acoes-320-claro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 320 px | claro |
| `pensantus-turno-acoes-320-escuro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 320 px | escuro |
| `pensantus-turno-acoes-390-claro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 390 px | claro |
| `pensantus-turno-acoes-390-escuro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 390 px | escuro |
| `pensantus-turno-acoes-768-claro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 768 px | claro |
| `pensantus-turno-acoes-768-escuro.png` | Pensantus (mago) | na vez, rolado ate as abas de acao | 768 px | escuro |
| `teatro-1280-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 1280 px | claro |
| `teatro-1280-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 1280 px | escuro |
| `teatro-320-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 320 px | claro |
| `teatro-320-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 320 px | escuro |
| `teatro-390-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 390 px | claro |
| `teatro-390-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 390 px | escuro |
| `teatro-768-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 768 px | claro |
| `teatro-768-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, topo | 768 px | escuro |
| `teatro-acoes-1280-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 1280 px | claro |
| `teatro-acoes-1280-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 1280 px | escuro |
| `teatro-acoes-320-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 320 px | claro |
| `teatro-acoes-320-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 320 px | escuro |
| `teatro-acoes-390-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 390 px | claro |
| `teatro-acoes-390-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 390 px | escuro |
| `teatro-acoes-768-claro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 768 px | claro |
| `teatro-acoes-768-escuro.png` | Pensantus, combate sem mapa (teatro da mente) | teatro da mente, rolado ate as abas | 768 px | escuro |
| `toren-fora-1280-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 1280 px | claro |
| `toren-fora-1280-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 1280 px | escuro |
| `toren-fora-320-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 320 px | claro |
| `toren-fora-320-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 320 px | escuro |
| `toren-fora-390-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 390 px | claro |
| `toren-fora-390-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 390 px | escuro |
| `toren-fora-768-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 768 px | claro |
| `toren-fora-768-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, topo | 768 px | escuro |
| `toren-fora-acoes-1280-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 1280 px | claro |
| `toren-fora-acoes-1280-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 1280 px | escuro |
| `toren-fora-acoes-320-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 320 px | claro |
| `toren-fora-acoes-320-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 320 px | escuro |
| `toren-fora-acoes-390-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 390 px | claro |
| `toren-fora-acoes-390-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 390 px | escuro |
| `toren-fora-acoes-768-claro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 768 px | claro |
| `toren-fora-acoes-768-escuro.png` | Toren (guerreiro, aba Guerreiro) | fora da vez, area de acoes | 768 px | escuro |
| `toren-turno-1280-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 1280 px | claro |
| `toren-turno-1280-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 1280 px | escuro |
| `toren-turno-320-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 320 px | claro |
| `toren-turno-320-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 320 px | escuro |
| `toren-turno-390-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 390 px | claro |
| `toren-turno-390-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 390 px | escuro |
| `toren-turno-768-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 768 px | claro |
| `toren-turno-768-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, topo da tela | 768 px | escuro |
| `toren-turno-acoes-1280-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 1280 px | claro |
| `toren-turno-acoes-1280-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 1280 px | escuro |
| `toren-turno-acoes-320-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 320 px | claro |
| `toren-turno-acoes-320-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 320 px | escuro |
| `toren-turno-acoes-390-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 390 px | claro |
| `toren-turno-acoes-390-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 390 px | escuro |
| `toren-turno-acoes-768-claro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 768 px | claro |
| `toren-turno-acoes-768-escuro.png` | Toren (guerreiro, aba Guerreiro) | na vez, rolado ate as abas de acao | 768 px | escuro |
