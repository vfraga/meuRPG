# Antes do MVP: PM-01, PM-02 e PM-03 (PM-04 fica de fora, de propósito)

Os quadros são HTML estático (estilos em linha), no formato das rodadas anteriores. Cada arquivo tem um PNG ao lado.
`project/canvas.json` põe os oito quadros numa fila, 80 px entre eles. Os `build_*.py`, `lib.py`, `mapkit.py` e
`crop.js` são só as ferramentas que desenharam os quadros (rodam com `python3 build_pm01a.py`, por exemplo).

| Quadro | Largura × altura | O que tem |
| --- | --- | --- |
| `PM-01a` sessões anteriores: o painel | 1280 × 3526 | painel da campanha com três, uma e nenhuma sessão; carregando, erro, lista longa; jogador no celular (390, 320); foco visível |
| `PM-01b` sessões anteriores: o resumo | 1280 × 5369 | a página `/campaigns/<id>/sessions/<n>`: com combates, sem combate, jogador, carregando e não encontrada |
| `PM-01c` sessões anteriores: outros estados | 1280 × 4361 | ainda aberta, erro, mestre × jogador, jogador sem personagem na sessão, mestre no celular, foco no pager |
| `PM-02a` magia de área: regras, colocar, quem está dentro | 1280 × 4434 | as 8 regras com o SRD ao lado, as 5 formas, colocar a Bola de Fogo na caverna (390, 320), quem está dentro |
| `PM-02b` magia de área: ninguém, parede, alcance, formas, teclado | 1280 × 4074 | confirmação "ninguém na área", ponto atrás da parede, fora do alcance, cone, linha e cubo, teclado e "Centrar em…" |
| `PM-02c` magia de área: escondidas e o turno parado | 1280 × 5910 | regra "Criaturas escondidas atingidas por uma área", pedido ao mestre que trava o turno, duas perguntas, depois de recarregar, resultado do jogador, o turno parado |
| `PM-02d` magia de área: seletor do mestre, teatro da mente, servidor | 1280 × 3504 | seletor do mestre com o interruptor, registros, teatro da mente, contrato do servidor |
| `PM-03a` Escudo Arcano e Ajuda | 1280 × 4594 | CA e PV do jogador, Ajuda +10, fim do Escudo, ficha, ordem do mestre, encerrar a Ajuda, Ajuda a 0 PV |
| `PM-03b` Crítico Brutal e Talento Confiável | 1280 × 4378 | dano do crítico (app e dados físicos), a conta nos níveis 9/13/17, registro, resultado do teste |
| `PM-03c` salto, ataque de oportunidade e armadilhas | 1280 × 7540 | aviso do salto (sem e com armadilha revelada), painel e registros do mestre, "Também acham com", busca do jogador, a conta do servidor |

Os números dos estados continuam entre os arquivos de PM-02: 1 a 3 em PM-02a, 4 a 7 em PM-02b, 8 a 10b (com 9b e 9c) em PM-02c, 11 a 14 em PM-02d. PM-01b e PM-01c seguem a mesma numeração (1 a 4 e 5 a 9).

## Decisões

**PM-01**

1. Painel vazio: o **mestre** vê o painel com "Nenhuma sessão encerrada ainda..." (caixa tracejada, sem botão); o **jogador não vê o painel**. É o que o painel "Sessão" já faz.
2. A linha traz também a **duração** ("3 h 42 min"), calculada dos dois horários; o servidor não manda mais nada. A data usa o formato do pedido ("qui., 8 de out., 19h05 às 22h47"), que é um formatador novo; hoje o app escreve "30/09 às 20:05". Sessão que cruza a meia-noite traz o dia da hora final; de outro ano, o ano.
3. Lista longa: as 5 mais novas e "Mostrar as outras N" (só no navegador, a lista já vem inteira).
4. O painel usa a **mesma resposta** de `ListGameSessions` que a página da campanha já lê (sem segunda chamada).
5. Cabeçalho da página: h1 "Sessão 2", embaixo "Mirathel · data e hora"; o título da aba é "Sessão 2 · Mirathel". Usei o nome da campanha do resto dos quadros (Mirathel), não "Cripta do Corvo".
6. O cartão verde "Sessão encerrada" (aviso do instante do fim) **sai** da página de arquivo; os quatro números viram o painel "Em números". O resto do resumo é o de hoje, e a página ganha o que a resposta já traz e a tela não mostrava: **"Números de cada jogador"** (só mestre). A tela do fim da sessão fica como é; as duas usam um componente.
7. Sessões vizinhas: dois links no alto ("Sessão 1", "Sessão 3"); sem vizinha, o link some. Se a seguinte está aberta, o link diz "Sessão 4 em andamento".
8. "Não houve combate nesta sessão." é um texto novo, só do navegador.
9. Não encontrada: o servidor devolve `not_found` para "número que não existe" e para "não é membro". O app separa pelo que sabe: membro (lista respondeu) vê "Não há Sessão 9" com a última sessão; quem não é membro vê uma página genérica sem o nome da campanha.

**PM-02**

10. **Quem conjura é Pensantus, Mago 5** (a Bola de Fogo pede espaço de 3º nível; nos quadros da Etapa 10 ele é Mago 4). Mapa: a caverna. Toren é o aliado dentro da área.
11. Fluxo em dois passos: 1 "Onde ela explode" (ponto ou direção; "Confirmar local") e 2 "Quem está na área" (lista com cobertura, "Mudar o local", "Conjurar"). Com o dedo, o molde segue **44 px acima** do dedo. "Ajustar o ponto" move um quadrado por toque.
12. **Cone, linha e cubo em 8 direções** (a cada 45°), não em ângulo livre; o cubo encosta por uma face (lados retos) ou uma quina (diagonais).
13. Um quadrado entra na área quando o **centro** dele está dentro (esfera: a até 4 quadrados do ponto, medida exata, não arredondada para baixo como a distância de hoje, para a bola não crescer 1,5 m).
14. **A Bola de Fogo "se espalha pelos cantos"** (texto da magia): a área é a parte da esfera ligada ao ponto, não só o que a linha reta alcança; as outras formas seguem a regra geral (só cobertura total bloqueia). Quem é atingido só pelo canto **não ganha bônus automático** no teste (o SRD não dá nenhum); o mestre marca a cobertura à mão (`SetCombatantCover`), como hoje.
15. Servidor (tudo "novo no servidor", tabela em PM-02d estado 14): `PreviewSpellArea` (só leitura), `CastSpellRequest.area` (ponto ou direção; para área em mapa em grade, `targets` é ignorado), `CastSpellRequest.reveal_hidden` (só mestre), `CastSpellResponse.area/hidden_hits/pending_reveal_id`, `ResolveHiddenReveal`, o motivo de espera `HIDDEN_REVEAL_PENDING`, `TableRules.hidden_area_hits` (`REVEAL` padrão, `KEEP_HIDDEN`, `ASK`) e o evento `hidden_hit_pending` (só mestre).
16. "Ninguém na área": a confirmação fica no lugar, com "Conjurar mesmo assim" e "Mudar o local"; o texto diz "que você vê" porque pode haver escondida lá dentro.
17. Escondidas: o efeito sempre vale. Antes do Revelar, o jogador não recebe a lista, a contagem, o resultado nem o registro (nada que a nomeie). Com "Perguntar a cada vez", o pedido ao mestre **trava o turno** (decisão do Vinicius, 08/10): o servidor recusa mover, atacar, conjurar, agir, `EndTurn` e o `NextTurn` do mestre com o motivo novo `HIDDEN_REVEAL_PENDING`, até o mestre tocar "Revelar" ou "Manter escondidas". O jogador lê só "Esperando o mestre" (a mesma frase do ataque de oportunidade), então a espera não revela nada. O mestre continua podendo agir; "Encerrar combate" descarta a pergunta. Para o mestre que conjura, o interruptor "Revelar as escondidas atingidas" só aparece se há escondida na área e começa na regra da mesa; com "Perguntar a cada vez" começa ligado.
18. A regra nova entra em `TableRules` e é gravada pelo "Salvar regras" que a página já tem; o servidor lê a regra na hora de cada magia (muda no meio da sessão).
19. Teatro da mente: a lista de hoje continua; a confirmação "Ninguém está marcado" é a mesma do passo 2; a cobertura continua sendo a marca do mestre.
20. Teclado: setas movem o ponto (Shift: 5 quadrados), Enter coloca/confirma, Escape cancela, C abre "Centrar em…", uma lista com as criaturas que quem conjura vê. Para cone, linha e cubo o botão é "Apontar para…".
21. Tokens: os do app de hoje (personagem: disco escuro com inicial clara; NPC: quadrado branco). A área da magia é um tom grená com contorno; "Fora do alcance" escurece o quadrado; na caverna os 45 m cobrem o mapa todo, então o alcance aparece num mapa de exemplo de 60 × 40.

**PM-03**

22. Exemplos com os dados do app: Pensantus tem CA 13, então "CA 18" e "CA 13 + 5" (o pedido dizia "CA 20" e "CA 15 + 5"). A Ajuda é em Sálvia (38 + 5 = 43, como no pedido), conjurada em espaço de 2º nível por um NPC. Crítico Brutal: **Ragna, Bárbara 9** (d12, Força 16); Talento Confiável: Brisa, Ladina 11, Acrobacia +9.
23. Ajuda: `hit_points_max_bonus` novo em `CharacterVitals` e `Combatant`; `hit_points_max` passa a ser o máximo efetivo. Não conta horas: acaba quando o mestre encerra (`EndCombatEffect`, novo) ou num descanso longo. Ao acabar, os PV atuais **só perdem o que passa do novo máximo** e nunca zeram por si. A Ajuda **deixa de dar PV temporários**.
24. Crítico Brutal: `PendingDamage.extra_dice_count` e `extra_dice_name_pt`; com dado físico o jogador continua digitando a **soma** (`typed_sum`), mas a tela diz quantos dados rolar (3d12) e a faixa (3 a 36). Com a regra "máximo mais uma rolagem", o máximo entra sozinho e o Crítico Brutal continua um dado rolado.
25. Talento Confiável: `DiceRoll.treated_as` e `treated_as_source`; a tela mostra "d20: 6 → 10 (Talento Confiável) + 9 = 19" só quando a regra mudou o número; vale onde o servidor rola teste de perícia (cena, busca de armadilhas), nunca em ataque, resistência, iniciativa nem teste de habilidade sem perícia.
26. Salto: o servidor **já provoca** no salto longo (`MoveCombatant`); só faltava o aviso. Novo: `GetMoveOptionsRequest.jump` e `jump_running_start` para a prévia trazer `provokes_reactor_ids`; `OpportunityOffer.jump` só para o painel dizer "saltou". Sem aviso em salto de altura.
27. Armadilhas: `TrapSpec.also_find_skill_keys` (nenhuma por padrão; usam a CD para achar), `TrapSearchSkill.OTHER` + `SearchForTrapsRequest.other_skill_key`. O menu do jogador é igual para toda armadilha e a busca com uma perícia que nenhuma armadilha por perto aceita é **igual** a uma busca que falhou (mesma resposta, mesma ação gasta, mesmo texto; a pílula é "Achou" ou "Nada", não "Falhou"). A penalidade de luz continua só da Percepção.

## Regras verificadas (SRD 5.1, no commit `a8abc93b…` do 5e-database; capítulos de regras e magias)

| Regra | Onde | O que diz (resumo) |
| --- | --- | --- |
| Ponto de origem da esfera e do cilindro | "Areas of Effect" (Sphere, Cylinder); Bola de Fogo | esfera: o ponto é escolhido, o raio sai dele e ele entra na área; Bola de Fogo: "a point you choose within range" |
| Cone e linha saem de quem conjura | "Areas of Effect" (Cone, Line); "Spell Range"; Mãos Flamejantes, Relâmpago | alcance Pessoal para cones e linhas "que saem de você"; o ponto de origem só entra na área se o conjurador decidir |
| Cubo | "Areas of Effect" (Cube); Onda Trovejante | "originating from you"; o ponto de origem fica numa face |
| Linha de efeito | "Areas of Effect"; Bola de Fogo | só cobertura total bloqueia a linha; a Bola de Fogo "spreads around corners" |
| Cobertura contra o teste | "Cover" | meia = +2, três quartos = +5 em CA e testes de Destreza; só vale se o efeito "originates on the opposite side"; criatura (amiga ou inimiga) dá meia cobertura |
| Aliados e o conjurador entram | Bola de Fogo ("Each creature…"); "Damage Rolls" | dano rolado uma vez para todos |
| Ponto que o conjurador não vê | "Targets" | o ponto de origem "comes into being on the near side of that obstruction" |
| Escondida atingida por área | "Unseen Attackers and Targets" | só trata de ataques; o SRD não diz que um golpe de área revela: decisão do Vinicius (08/10) |
| Escudo Arcano | Magia Escudo | reação; +5 de CA até o começo do próximo turno, inclusive contra o ataque que a provocou |
| Ajuda | Magia Ajuda | até 3 criaturas; máximo e PV atuais +5 por 8 horas; +5 por nível acima do 2º |
| PV não passam do máximo | "Healing" | usado para o que acontece ao fim da Ajuda |
| Acordar a 0 PV | "Dropping to 0 Hit Points" | "This unconsciousness ends if you regain any hit points"; testes contra a morte zeram |
| PV temporários não acordam | "Temporary Hit Points" | por isso a Ajuda não pode ser PV temporários |
| Efeitos iguais não somam | "Combining Magical Effects" | vale o mais potente (Ajuda repetida) |
| Descanso longo | "Long Rest" | pelo menos 8 horas (a Ajuda dura 8 horas) |
| Crítico Brutal | Característica do Bárbaro, nível 9 | +1 dado de arma no crítico corpo a corpo; 2 no 13; 3 no 17 |
| Dados do crítico | "Damage Rolls" (Critical Hits) | rolar todos os dados de dano duas vezes |
| Talento Confiável | Característica do Ladino, nível 11 | em teste de habilidade que soma proficiência, d20 de 9 ou menos conta 10 |
| Ataque de oportunidade | "Melee Attacks" (Opportunity Attacks); "Moving Around Other Creatures"; Desengajar | provoca quem sai do alcance de um hostil que o vê; Desengajar evita; o ataque vem "right before" a saída |
| Salto longo | "Special Types of Movement" (Jumping) | cada pé saltado custa um pé de movimento; precisa de 3 m a pé antes |
| Busca de armadilhas | "Traps in Play"; "Search" | Percepção ou Investigação; **qualquer personagem pode usar Arcanismo contra uma armadilha mágica** (o app não decide isso: o mestre acrescenta) |

## Revisão (adversarial, aplicada)

- **Cobertura só vale para teste de Destreza** (SRD 5.1, "Cover"). A Onda Trovejante (Constituição) não mostra bônus ("Sem bônus de cobertura: o teste é de Constituição"); `PreviewSpellAreaResponse.targets[].cover` e a linha genérica do passo 2 passam a depender da habilidade do teste. Nenhum texto de "+5 pelo canto" sobrou: a Bola de Fogo contornando cantos não dá bônus automático; o mestre marca a cobertura à mão.
- **Escudo Arcano:** a linha com "Vez" é a do Hobgoblin (o Escudo acaba no começo do turno do Pensantus); o instante em que acaba foi desenhado (PM-03a 1c), junto com a Ajuda de 3º nível (+10).
- **"Encerrar Ajuda"** é uma confirmação contornada em `danger-ink`, com o foco em "Cancelar".
- **RN-10 no salto:** a armadilha só aparece no mapa, na legenda, no cartão e no registro dos jogadores quando já foi revelada a quem lê; o caso principal (PM-03c) é o da armadilha não achada; o teste de vazamento do repositório precisa cobrir o salto sobre armadilha escondida. `ResolveHiddenReveal`: o jogador recebe o mesmo `permission_denied` para qualquer `pending_reveal_id`, checado antes de procurar o id.
- **Layout:** colunas iguais dentro de 48–1232 (PM-01a), caixas de números do resumo com a altura de dois rótulos e valor de 28 px, `font-family` nos dois botões, chips e "Acrescentar perícia" com 44 px, lista de perícias colada ao botão, linha do SRD sem sugerir o que já é etiqueta, rótulo do escudo a 13 px, rótulo e valor alinhados no cartão do salto, 8 px entre botões, contornos do mapa em `#8a1f33` com halo maior (acima de 3:1 sobre o pergaminho).
- **Dados de exemplo:** só uma sessão aberta (a 4); as sessões 1 a 3 em quintas anteriores (17/09, 24/09, 01/10) e o aviso da aberta no formato novo; nenhuma "Sessão 6"; "Machado grande", "Meio-elfo", "Ladino 4/11", "Bárbaro 9"; monstros só do SRD (Goblin, Hobgoblin); "Zuk" é um NPC do mestre (base: Goblin); "Pensantus (você), aqui".
- **Estados novos:** foco visível (PM-01a 5, PM-01c 9), jogador sem personagem na sessão (PM-01c 7), mestre no celular (PM-01c 8), duas perguntas ao mesmo tempo (PM-02c 9b; a espera segura o andamento do turno, não as reações), depois de recarregar (PM-02c 9c), Ajuda +10 e fim do Escudo (PM-03a 1c), a conta do Crítico Brutal nos níveis 13 e 17 (PM-03b 2b; o bônus de dano da Fúria é de outro bloco e a linha ganhará "+ 3 de Fúria" depois).
- **Não aplicado:** os seletores de cone, linha e cubo do mestre no desktop (a lista de "estados que faltam" os citava, mas a decisão não os incluiu); o seletor do mestre mostra só a Bola de Fogo. O mestre usa o mesmo passo do jogador, com a lista completa.

## Perguntas para o Vinicius

1. **Bola de Fogo contorna cantos** (texto da magia): a área é a parte da esfera ligada ao ponto, e quem é atingido só pelo canto não ganha bônus automático (o SRD não dá nenhum); **o mestre marca a cobertura à mão**, como hoje. Serve?
2. **Cone/linha em 8 direções.** Prefere ângulo livre? Fica mais fiel, mas o servidor teria que receber um ângulo e as regras de borda ficam menos previsíveis.
3. **O que é "dentro" do círculo:** centro do quadrado a até 4 quadrados do ponto (exato). A distância de hoje (RN-21) arredonda para baixo; usá-la aqui faria o raio de 6 m virar 7,5 m. Manter o exato?
4. **O turno parado pela pergunta** não tem tempo limite: espera o mestre ou o fim do combate. Quer um "Pular" depois de um tempo (por exemplo, se o mestre saiu)?
5. **Interruptor do mestre com "Perguntar a cada vez":** começa ligado ("Revelar"). Ou melhor sem valor inicial e obrigar a escolha?
6. **Ajuda:** o app não conta as 8 horas (acaba por descanso longo ou por "Encerrar Ajuda"). E o fim da Ajuda só corta o que passa do novo máximo (o SRD não diz). Confirma?
7. **Ajuda em quem está a 0 PV acorda** (o pedido já dizia; o SRD não diz se o aumento conta como "recuperar"). Ao zerar, os testes contra a morte também zeram?
8. **Segunda Ajuda sobre a primeira:** vale a mais forte, não soma (SRD, "Combining Magical Effects"). Aceita?
9. **Armadilha mágica:** o SRD diz que qualquer personagem pode usar Arcanismo contra armadilha mágica; pelo seu critério o app não presume isso. Quer que as predefinições mágicas (Estátua que cospe fogo) já venham com Arcanismo marcado, ou nenhuma?
10. **Busca com "Outra perícia…":** a penalidade de luz continua só da Percepção. Certo?
11. **Salto fora do alcance:** nenhum campo novo para o ataque de oportunidade (o servidor já resolve); só o aviso e a palavra "saltou". Serve para esta etapa?
12. **Data das sessões** ("qui., 1 de out., 19h05 às 22h47"): num formatador novo. Desenhei o aviso da sessão aberta também no formato novo ("desde qui., 8 de out., 19h05"). Trocar o "30/09 às 20:05" que existe hoje por ele em todo o app?
13. **PM-04** (reações) não foi desenhado, como pedido.

## Lote 2: Designer A (PM-04, PM-06, PM-07)

Quadros: `PM-04a` 4019 px, `PM-04b` 4068, `PM-04c` 5532 (reações e teste de concentração); `PM-06a` 4855, `PM-06b` 6332
(vantagem e desvantagem; o que se soma ao dano); `PM-07a` 4059, `PM-07b` 4246, `PM-07c` 5366 (estados, resistências,
contadores e diálogos de recursos). Ferramentas: `build_pm04a.py` a `build_pm07c.py`, `pm4lib.py`.

### Decisões

**PM-04**
1. **Uma só janela de reação** para as sete reações, o ataque de oportunidade, a pergunta de revelar de PM-02 e o teste de concentração: `Encounter.reaction_windows[]`, `AnswerReaction`, motivo de espera `REACTION_PENDING` (e `CONCENTRATION_SAVE_PENDING`). A ação que a provocou fica `AWAITING_REACTION`.
2. **O que o jogador lê:** "Esperando o mestre" para qualquer reator NPC ou não visto; "Esperando a reação de Sálvia" só para um reator **jogador que ele vê**. Nunca qual NPC nem por quê.
3. **Uma reação por rodada** (SRD, "Reactions"); incapacitado não reage (SRD, Incapacitado): janelas que deixam de valer **fecham sozinhas**, com a razão só do próprio reator (PM-04c 11).
4. **O prompt chega depois do acerto e antes do dano** (Esquiva Sobrenatural, Defletir Projéteis, Escudo Arcano) e **sem o total** (RN-20). Contramágica chega **sem o nome nem o nível da magia**.
5. **Palavras de Interrupção** (o nome em `names_pt.json`) gasta **1 uso de Inspiração de Bardo** (hoje não gasta); o prompt vem antes de o mestre dizer o resultado.
6. **Repreensão Infernal** oferece o Legado Infernal do tiefling (1 uso, 2º nível) e o espaço de pacto; a CD é a de quem conjura; o teste do agressor NPC é do mestre.
7. **Defletir Projéteis:** o chi só se gasta na devolução; a devolução faz parte da mesma reação.
8. **A espera segura o andamento do turno, não as reações**: uma reação de outro jogador pode abrir uma segunda janela durante a espera; o mestre responde na ordem da iniciativa.
9. **Concentração:** prompt ao dono com a CD (maior entre 10 e metade do dano), dado do app, digitado ou "Deixar o mestre rolar por mim"; a ação que deu o dano espera; a 0 PV a concentração acaba sem teste; vários danos do mesmo ataque são uma fonte, fontes diferentes são janelas separadas.
10. O registro ganha uma linha por reação; o dos jogadores sem números de NPC e sem nomear um reator que eles não veem (`ReactionWindow.log_text_pt` com `for_master` / `for_players`).

**PM-06**
11. As fontes de vantagem e desvantagem vêm do servidor (`AdvantageSource`) e a tela as lista; o resultado é o do SRD (qualquer número de fontes de um lado + uma do outro = Normal). Entram também as desvantagens de alcance (SRD, "Ranged Attacks") que o servidor já conhece.
12. Mudar a sugestão exige **motivo** (1 a 120 caracteres) e vai ao registro; vale para jogador e mestre.
13. Com vantagem/desvantagem, **os dois d20** aparecem, o que vale com borda e "vale", o outro riscado e "descartado"; dados físicos: dois campos, `d20_faces[]`.
14. O Sentido de Perigo **não aparece** como fonte contra uma armadilha que o personagem não vê (RN-10).
15. Dano: `PendingDamage.parts[]`; extras oferecidos (Ataque Furtivo, Destruição Divina, Marca do Caçador, Matador de Colossos) com a condição escrita; **condição que falha = linha desativada com o motivo**, nunca some; automáticas (Fúria, Duelismo, Armas Grandes, Crítico Brutal) como linhas escritas. O crítico dobra os **dados** dos extras, não os valores fixos (SRD, "Damage Rolls").
16. Dados físicos: **um campo por grupo de dados** (`typed_parts[]`), com a contagem e a faixa. O rerrolar do Combate com Armas Grandes é à mão.
17. O mestre vê o dano separado por parte e pode tirar um extra com motivo.

**PM-07**
18. Estados: `Combatant.states[]` (Em fúria, Marcado, Esquivando, Ataque descuidado); Atordoado usa a condição existente com origem e fim.
19. **O fim da fúria é uma pergunta ao jogador** ("Voltar e atacar" ou "Deixar a fúria acabar") quando o turno termina sem ataque a hostil nem dano; o servidor decide o fato. Em fúria o app recusa conjurar.
20. Resistências na prévia do dano: cada passo com a origem (`DamageStep`); arredonda para baixo (PHB); resistências repetidas ao mesmo tipo contam uma só vez (SRD). A de um **NPC** é só do mestre (RN-20).
21. Contadores com a recarga escrita e **reposição no descanso** (`recharge`, `TakeRest`), conforme o SRD recurso a recurso (PM-07b 8).
22. Cura pelas Mãos: alvo, quantia (1 até a reserva) ou curar doença/veneno (5 cada); recusa morto-vivo e constructo; nenhum tipo de NPC é dito ao jogador.
23. Conjuração Flexível: custos 2/3/5/6/7 (SRD), espaço criado some no descanso longo; converter dá o nível em pontos, até o máximo.
24. Metamagia na folha de conjurar: só as opções conhecidas, com custo; a que a magia não aceita fica desativada com o motivo; a Duplicada custa o nível da magia (1 para um truque).
25. Inspiração de Bardo: alvo que não é o bardo, a 18 m, que o ouve; um dado por vez; o jogador usa o dado **depois de rolar e antes do resultado**; cartão de dado na ficha.

### Regras verificadas (SRD 5.1 no commit fixado; PHB brasileiro só para nomes, gatilhos e arredondamento)

| Regra | Onde |
| --- | --- |
| Uma reação por rodada; reação como resposta a um gatilho | "Reactions" |
| Incapacitado não age nem reage; Atordoado, Paralisado, Inconsciente | Condições |
| Escudo Arcano, Repreensão Infernal, Contramágica, Queda Suave (efeito) | Magias (gatilho: PHB, "Tempo de Conjuração") |
| Esquiva Sobrenatural | Ladino 5 |
| Palavras de Interrupção, Inspiração de Bardo | Bardo 1 e Colégio do Conhecimento 3 |
| Defletir Projéteis, Ataque Atordoante, Defesa Paciente, Passo do Vento (para a ação bônus) | Monge 2, 3 e 5 |
| Concentração e o teste de Constituição | "Duration" |
| Vantagem e desvantagem, fontes que não se somam, uma de cada se anula | "Advantage and Disadvantage" |
| Derrubado, Impedido, Cego, Envenenado, Amedrontado, Invisível | Condições |
| Esquivar; Ataque Descuidado; Sentido de Perigo; Fúria | "Dodge"; Bárbaro 1 e 2 |
| Táticas de Matilha | Lobo, Lobo atroz, Chacal |
| Atacante não visto | "Unseen Attackers and Targets" |
| Alcance normal e longo; ataque à distância em combate corpo a corpo | "Ranged Attacks" |
| Ataque Furtivo; Destruição Divina; Marca do Caçador; Matador de Colossos; Duelismo; Armas Grandes | Ladino 1; Paladino 2; magia; Caçador 3; Estilos de Luta |
| O crítico dobra todos os dados de dano | "Damage Rolls" |
| Resistência, vulnerabilidade, ordem e "contam como uma vez" | "Damage Resistance and Vulnerability" |
| Resistências raciais | tiefling (Hellish Resistance), anão (Dwarven Resilience), draconato (Damage Resistance) |
| Descansos de cada recurso | Bárbaro 1, Monge 2, Feiticeiro 2, Clérigo 2, Paladino 1, Bardo 1 e 5, Druida 2, Guerreiro 1 e 2; "Spell Slots" |
| Cura pelas Mãos; Conjuração Flexível (tabela de custos); Metamagia (custos) | Paladino 1; Feiticeiro 2 e 3 |
| Arredondar para baixo ao dividir | PHB ("Arredonde para baixo"); o SRD só o diz do modificador |

### Perguntas para o Vinicius (Lote 2, Designer A)

1. **Prazo para a janela de reação:** nenhum (espera o mestre ou o fim do combate), como a pergunta de revelar. Quer um "Pular" depois de algum tempo?
2. **Palavras de Interrupção** pode abrir uma pergunta a cada rolagem inimiga (ataque, teste, dano). Proponho uma escolha na ficha: "perguntar em todas / só nos ataques / nunca". Serve?
3. **Contramágica:** o espaço do conjurador cuja magia foi anulada **se gasta** (a magia foi conjurada; o SRD não diz o contrário). Confere?
4. **Imunidade de Palavras de Interrupção** (a criatura não ouve ou é imune a enfeitiçar): o servidor não abre a janela e o jogador não vê nada. Aceita que um jogador não saiba por que não houve prompt?
5. **Destruição Divina e o tipo do alvo:** a linha escreve "+1d8: o alvo é morto-vivo" depois do acerto; isso revela o tipo de um NPC. Prefere só "+1d8" sem a razão?
6. **Desvantagens de alcance** (além do alcance normal, hostil a 1,5 m) entram como fontes porque o servidor as conhece; o pedido listava só condições. Mantém?
7. **Nomes que faltam em `names_pt.json`:** "Táticas de Matilha", "Em fúria", "Esquivando", "Marcado". Posso pedir as entradas?
8. **Mudar a rolagem sugerida** (PM-06 4): um jogador pode mudar para um modo melhor para ele? Hoje o motivo é só escrito; talvez deva exigir o mestre.
9. **Recarga automática no descanso** (PM-07b 8): hoje os descansos são à mão. Concorda em o servidor repor os contadores?
10. **Fúria:** perguntar ao fim do turno só quando não houve ataque nem dano, ou nunca perguntar (acabar sozinha)? O pedido dizia "o app pergunta ou acaba".
11. **Conjuração Flexível acima do máximo de pontos:** o excesso se perde com aviso; ou recusar a conversão?


## Lote 2: Designer B (PM-05 e PM-08)

| Quadro | Largura × altura | O que tem |
| --- | --- | --- |
| `PM-05a` as escolhas da classe e da raça: o passo | 1280 × 7451 | a tabela de tudo o que a etapa pergunta, com o SRD ao lado; o passo no nível 1 (Tharn, Guerreiro 1, Draconato) em desktop e celular; escolha pendente que trava "Criar personagem"; o nível 5 (Kaelith, Patrulheiro 5, Meio-elfo) |
| `PM-05b` as escolhas do Bruxo e do Feiticeiro | 1280 × 5746 | Dádiva do Pacto e Invocações Místicas com o pré-requisito à vista, a troca de Dádiva, o botão "Escolher Rajada Mística agora", o Pacto do Tomo, o Ancestral Dracônico e a Metamagia |
| `PM-05c` Patrulheiro, Druida, elfos e seletores | 1280 × 4909 | Inimigo Favorito e idioma, o terreno do Círculo da Terra com as magias dele, o truque do Alto Elfo, as magias do patrono (Corruptor) e os Segredos Mágicos nos seletores |
| `PM-05d` a ficha, a ficha travada, o subir de nível e o servidor | 1280 × 4921 | o que a ficha mostra (Arma de Sopro, resistência, invocações), "Completar escolhas pendentes", o subir de nível com escolhas atrasadas, o aviso ao iniciar a sessão e o contrato do servidor |
| `PM-08a` pedir ajustes | 1280 × 5596 | os três botões do mestre, o motivo obrigatório, o que o mestre e o jogador veem, "Enviar de novo", a recusa que continua apagando, o servidor |
| `PM-08b` multiclasse: regras e o passo da classe | 1280 × 4312 | as regras do SRD uma a uma, "Subir em qual classe?" em desktop e celular, o pré-requisito recusado |
| `PM-08c` multiclasse: passos, resumo e servidor | 1280 × 5822 | Vida e Magias da classe nova, o resumo de Guerreiro 5 e Mago 1, o segundo subir de nível e a tabela de espaços, as proficiências do Ladino e as exceções, as recusas e os testes |
| `PM-08d` Reviver, Revivificar e o jogador do morto | 1280 × 5656 | "Reviver" do mestre (com o bloqueio de um personagem vivo), Revivificar (alvo, material, resultado), a lista de mortos na ordem do mestre, a página do jogador do personagem morto, o servidor |

Os estados de PM-05 vão de 1 a 15 (a: 1 a 5; b: 6 a 7; c: 8 a 10b; d: 11 a 15). PM-08a tem 1 a 6; PM-08b, 1 a 3; PM-08c, 4 a 8; PM-08d, 1 a 6. Os `build_pm05*.py`, `build_pm08*.py` e `pm05lib.py` são só as ferramentas que desenharam os quadros.

### Decisões

**PM-05**

1. **Onde fica o passo.** Passo 3 de 6, entre "Habilidades" e "Perícias" (o pedido dizia "entre as habilidades e as magias": Perícias vem depois porque as escolhas de raça e classe mexem nelas). Só existe quando a ficha tem alguma escolha; sem escolha, ficam os 5 passos de hoje.
2. **Uma seção por origem** (Raça, Classe nível N, Subclasse), com a etiqueta "Feita" ou "Pendente" e um contador "Escolhas feitas: 4 de 5". Cada opção é um cartão de 56 px com o nome e a regra em uma linha. Escolher mostra o que a escolha dá (a Arma de Sopro com CD, dano, forma e resistência).
3. **"Criar personagem" não desabilita mudo.** Fica pontilhado e focável, com a razão em texto ("Falta uma escolha: Estilo de Luta (Patrulheiro, nível 2)"), e o passo ganha "!". O servidor também recusa (`CHOICES_MISSING`), só para personagem de jogador; o NPC curto e a ficha travada editada pelo mestre não bloqueiam.
4. **"+1 em duas habilidades" do Meio-elfo sai do campo "Bônus manuais"** e vira uma escolha deste passo.
5. **Opção que não vale fica na lista, pontilhada, com o motivo** (cadeado e frase: "Exige o nível 7 de Bruxo. Você está no 5."). Nunca some. Pré-requisito de magia (Rajada Mística) tem o botão "Escolher Rajada Mística agora", que acrescenta o truque, porque os truques vêm no passo seguinte.
6. **Trocar a Dádiva do Pacto** pergunta antes e diz quais invocações saem; o foco abre na escolha segura.
7. **Inimigo Favorito:** 13 tipos de criatura ou duas raças de humanoides (texto livre), e um idioma ou "Nenhum". Os nomes dos tipos não estão em `names_pt.json`: entram como `creature-type:*` (pergunta 3).
8. **Ficha travada: "Completar escolhas pendentes"** numa página só, sem stepper, só os grupos abertos; as já feitas ficam numa linha tracejada e nunca viram campo. O mestre recebe um registro. "+1 em duas habilidades" conta como escolha completável (a ficha muda: aviso na tela).
9. **Subir de nível:** o passo "Escolhas" ganha, no alto, "Escolhas que ficaram para trás"; "Próximo" fica bloqueado até todas estarem feitas. Nenhum passo novo.
10. **"Iniciar sessão" avisa** quando há escolha em aberto (lista de personagens e o que falta), com "Esperar os jogadores" (foco) e "Iniciar mesmo assim". O servidor não recusa; o aviso é do navegador. O contador "N escolhas em aberto" aparece só para o mestre e o dono (RN-10).
11. **O Círculo da Terra** precisa de uma correção nos dados do servidor (a característica não está em nenhuma linha de nível), não só da tela.
12. **O SRD acrescentou ao pedido:** truque extra do Círculo da Terra (druida 2), as escolhas do Caçador nos níveis 7, 11 e 15, Arcana Mística, Dominar Magia e Assinatura Mágica. O Estilo de Luta do Paladino e o do Patrulheiro têm 4 opções, mas não as mesmas.

**PM-08**

13. **Pedir ajustes:** o personagem continua "Pendente" (o jogador já edita um personagem pendente); ganha uma revisão `REVIEW_CHANGES_REQUESTED` com o motivo (1 a 500 caracteres, obrigatório), e o jogador tem "Enviar de novo". O mestre pode aprovar ou recusar a qualquer momento. O motivo é dado pessoal do mestre sobre o jogador: só mestre e dono o leem, e o servidor o apaga ao aprovar, ao recusar ou ao substituir.
14. **Recusar continua apagando**, com a confirmação desenhada como ação sem volta (botão contornado em danger-ink, foco em "Cancelar"). A tela de hoje usa o botão vermelho cheio (pergunta 9).
15. **Multiclasse:** o passo de classe abre sempre ("Subir em qual classe?") e ganha "Uma classe nova", com as 11 outras classes, o pré-requisito e o que o personagem tem em cada uma. O pré-requisito da classe atual conta (e das classes que o personagem já tem). No subir de nível o servidor recusa; na criação continua só avisando.
16. **Resumo da multiclasse** com "Antes → Depois" e o porquê de cada número; sem diálogo extra de confirmação (como o subir de nível de hoje).
17. **Dados de vida por tipo** (5d10 e 1d6): hoje o gasto é um número só; passa a ser por tipo.
18. **Reviver** (mestre): 1 PV, sem Inconsciente, testes contra a morte zerados; com combate aberto volta à ordem **sem turno na rodada atual**. Bloqueado enquanto o jogador tiver outro personagem vivo (RN-03).
19. **Revivificar** (SRD: "Revivify"; no app, **Revivificar**): só criaturas **mortas confirmadas pelo mestre**, que o conjurador vê, ao toque, e **até 10 rodadas** depois da morte (1 rodada = 6 s). Só em combate (é 1 ação). O material é um lembrete que se marca (o app não tem inventário de pedras); o registro diz que foi gasto.
20. **O jogador do personagem morto** continua na sessão, com a visão do grupo (a dos personagens vivos somada), sem agir, e vê "Criar um novo personagem", que passa pela aprovação do mestre como qualquer ficha nova.

### Regras verificadas

| Regra | Onde | O que diz (resumo) |
| --- | --- | --- |
| Estilo de Luta: 6 do Guerreiro, 4 do Paladino (Defesa, Duelismo, Armas Grandes, Proteção), 4 do Patrulheiro (Arquearia, Defesa, Duelismo, Duas Armas) | SRD 5.1, Fighter, Paladin, Ranger: "Fighting Style" | cada estilo com o seu texto |
| Ancestral Dracônico: dano, forma e teste de cada dragão | SRD 5.1, Dragonborn: "Draconic Ancestry" e "Breath Weapon" (5e-database, traços `draconic-ancestry-*`) | Negro, Cobre: ácido; Azul, Bronze: elétrico; Latão: fogo (linha de 9 m por 1,5 m, Destreza); Ouro, Vermelho: fogo; Prata, Branco: frio (cone de 4,5 m); Verde: veneno, Constituição |
| Dragon Ancestor do Feiticeiro | SRD 5.1, Sorcerer: "Dragon Ancestor", "Elemental Affinity" | a mesma tabela de tipos de dano |
| Invocações Místicas por nível; pré-requisitos | SRD 5.1, Warlock: tabela e "Eldritch Invocations" (5e-database `features`, campo `prerequisites`) | 2, 3, 4, 5, 6, 7 e 8 nos níveis 2, 5, 7, 9, 12, 15 e 18; 32 opções, cada uma com nível, magia ou Dádiva |
| Dádiva do Pacto | SRD 5.1, Warlock: "Pact Boon", "Pact of the Chain/Blade/Tome" | Corrente, Lâmina, Tomo (3 truques de qualquer classe) |
| Metamagia | SRD 5.1, Sorcerer: "Metamagic" | 2, 3 e 4 nos níveis 3, 10 e 17; 8 opções com o custo |
| Inimigo Favorito, Explorador Natural, Presa do Caçador | SRD 5.1, Ranger e Hunter | 13 tipos ou 2 raças; 7 terrenos; 3 presas |
| Círculo da Terra: terreno, magias do círculo, truque extra | SRD 5.1, Druid, "Circle of the Land", "Circle Spells", "Bonus Cantrip" | 7 terrenos; magias nos níveis 3, 5, 7 e 9 |
| Meio-elfo; Alto Elfo | SRD 5.1, Half-Elf "Ability Score Increase"; High Elf "Cantrip" | +1 em duas habilidades que não Carisma; um truque de Mago |
| Magias do patrono; Segredos Mágicos | SRD 5.1, Warlock, The Fiend, "Expanded Spell List"; Bard, "Magical Secrets"; Lore, "Additional Magical Secrets" | dez magias a mais na lista; 2 magias de qualquer classe |
| Multiclasse: pré-requisitos e proficiências | 5e-database `multi_classing` (a tabela do SRD); Livro do Jogador, cap. 6 | 13 na habilidade; tabela reduzida de proficiências |
| Multiclasse: XP, PV, dados de vida, bônus de proficiência, características, conjuração, espaços | Livro do Jogador, cap. 6 (o capítulo não está no 5e-database) | pelo nível total; dados por tipo; quatro exceções; nível de conjurador com metade e um terço; tabela de espaços |
| Revivify | SRD 5.1, spell Revivify | 3º nível, 1 ação, toque, diamantes de 300 PO consumidos, morto há até 1 minuto, volta com 1 PV |
| Um minuto são 10 rodadas | SRD 5.1, "Combat", "The Order of Combat" | uma rodada é cerca de 6 segundos |
| Voltar a 0 PV, e acordar | SRD 5.1, "Dropping to 0 Hit Points" | a inconsciência termina com qualquer PV; os testes contra a morte zeram |
| Um personagem vivo por jogador por campanha | RN-03 (regras do produto) | o índice único do servidor |

### Perguntas para o Vinicius (Lote 2, Designer B)

1. **Livro do Jogador x SRD 5.1 no Inimigo Favorito.** O livro impresso que o Vinicius tem traz outro Inimigo Favorito (5 tipos, +2 de dano, "Inimigo Favorito Maior", Conclaves). O app segue o SRD 5.1 (13 tipos, vantagem em rastrear, idioma). Serve?
2. **Passo novo "entre as habilidades e as perícias"** em vez de "entre as habilidades e as magias". Concorda com a ordem?
3. **Nomes dos 13 tipos de criatura** (Inimigo Favorito) não estão em `names_pt.json`; usei os do livro ("Corruptores", "Fadas", "Limos"). Posso pedir as entradas, com esses nomes?
4. **"+1 em duas habilidades" completável numa ficha travada.** Muda números (Destreza 15 → 16 sobe o modificador, e uma Constituição maior sobe os PV). Deixo completável, com aviso, ou fica só com o mestre?
5. **Rajada Mística pelo botão no passo das invocações:** acrescenta o truque aos truques da ficha. Se os truques do Bruxo já estão cheios, o botão vira "Trocar um truque por Rajada Mística…". Serve?
6. **Pedir ajustes mantém o personagem "Pendente"** (o jogador já edita um pendente). O pedido falava em "volta a rascunho editável"; para o jogador dá no mesmo. O mestre ainda pode aprovar durante o pedido. Confere?
7. **O motivo do pedido** entra no inventário de dados pessoais (`docs/privacy.md`) e é apagado ao aprovar, recusar ou substituir. Aceita?
8. **Pré-requisito da multiclasse com duas classes:** a regra diz "classe atual e a nova"; exijo o de todas as classes que o personagem tem. Prefere só a classe em que ele sobe? E no subir de nível o servidor recusa, na criação só avisa: manter?
9. **A confirmação "Recusar personagem" de hoje** é um botão vermelho cheio; o desenho segue o design.md (contornado em danger-ink, foco em "Cancelar"). Troco a tela de hoje também?
10. **Multiclasse sem diálogo de confirmação**, como o subir de nível de hoje. Quer um diálogo "Isto não se desfaz"?
11. **Dados de vida por tipo** pedem um campo novo e migração (hoje é um número só). Faz parte desta etapa?
12. **Reviver não tem prazo e não gasta nada** e é bloqueado quando o jogador já tem outro personagem vivo (RN-03). Em vez de bloquear, quer que o app peça ao mestre para marcar o novo como morto ali mesmo?
13. **Revivificar só em combate** (1 ação; a janela de 10 rodadas). Fora de combate, o tempo não é contado: só o "Reviver" do mestre. Serve?
14. **Quem volta com Revivificar ou Reviver fica sem turno na rodada atual.** O SRD não diz. Aceita?
15. **Material (diamantes de 300 PO):** um lembrete que se marca, sem descontar da bolsa. Quer que desconte 300 PO da bolsa ao conjurar?
16. **Uma morte que Revivificar não desfaz** (velhice, sem a cabeça): o app não sabe. Quer um interruptor do mestre na confirmação da morte ("Revivificar não funciona")?
17. **O jogador do personagem morto vê a visão do grupo** (a dos personagens vivos somada). Serve, ou prefere só a ordem e o registro, sem o mapa?

### Lote 2: revisão aplicada (Designer A)

Alturas novas: `PM-04a` 4099, `PM-04b` 4364, `PM-04c` 5532, **`PM-04d` 1856 (novo)**, `PM-06a` 5562, `PM-06b` 6572,
`PM-07a` 4059, `PM-07b` 4952, `PM-07c` 5912.

- **As respostas do Vinicius** (`decisions-batch2-A.md`) estão aplicadas: sem "Pular" na janela; Palavras de Interrupção com a escolha "em todos / só em ataques / nunca" (padrão: só em ataques); o espaço de uma magia anulada se gasta; "+1d8" sem razão; descansos que repõem recursos (PM-07b 9, com a confirmação do que volta); Conjuração Flexível **recusa** acima do máximo (PM-07c 10, terceiro quadro); as desvantagens de alcance ficam; "Em fúria", "Esquivando" e "Marcado" são rótulos da tela; "Táticas de Matilha" fica marcado para conferir no Livro dos Monstros em português.
- **Vantagem escolhida por um jogador** (PM-06a 4): aceitar a sugestão ou escolher Desvantagem é livre; **Vantagem que o servidor não sugeriu** vira um pedido na fila do mestre, com o motivo ("Aprovar Vantagem", "Recusar: rola Normal", "Desvantagem"); o mestre define qualquer modo. Substitui a decisão 6 do arquivo dele.
- **Nova regra da mesa "Reações dos inimigos"** (PM-04d): "Só quando um inimigo pode reagir" (padrão; a ajuda diz que uma pausa pode sugerir que alguém reage) ou "Sempre" (toda ação contra um inimigo espera o toque "Sem reação" do mestre; não vaza nada). O quadro desenha a regra em "Regras da mesa" e o toque do mestre. `TableRules.enemy_reactions`, `ReactionKind.MASTER_CHECK`.
- **RN-10:** a ausência de um prompt não vaza: Palavras de Interrupção abre para qualquer NPC hostil visível (reação e uso se gastam; "sem efeito" nas mesmas palavras); Cura pelas Mãos aceita qualquer alvo e, se a regra nega, "Nada acontece", sem nomear o tipo; "+1d8" aparece em toda linha de Destruição Divina e o d8 a mais está sempre em "Você vai rolar" (o servidor só o conta quando vale); o registro dos jogadores nunca imprime números ou resistências de NPC ("caiu pela metade", dano final apenas); o prompt da Esquiva mostra o dano do **próprio personagem**.
- **Regras corrigidas:** Tavo é Paladino 5 com 4 espaços de 1º e 2 de 2º (sem 3º); o crítico da Destruição Divina é 6d8 contra o Esqueleto (8 dados, 6 a 48); o segundo ataque de Brisa é a mão secundária com duas armas leves (Adaga 1d4, sem modificador); a Esquiva Sobrenatural saiu da fila da Bola de Fogo (só responde a um ataque que acerta) e as filas são duas; Ragna só tem Fúria; Nael conhece exatamente 2 opções de Metamagia (Duplicada e Cuidadosa); o Hobgoblin causa 1d8 + 1; Táticas de Matilha vem do bloco do monstro (17 criaturas no SRD, sem lista no app); o Petrificado entra nas fontes.
- **Elenco único:** Kai Monge 5 (CA 16, Chi 5, +3 Des, CD 14), Ragna Bárbaro 3 (CA 14, 35 PV, +5 com o machado grande), Orla Bardo 5 (d8, 3 usos), Toren Guerreiro 4 (+5 com a espada longa e com o machado grande), Tavo Paladino 5 (reserva de 25), Nael Feiticeiro 5 (5 pontos), Brisa Ladino 5 (+7, Espada curta 1d6 + 4). **O app imprime uma forma por classe** (Ladino, Bárbaro, Patrulheiro, Feiticeiro): as telas usam a forma do arquivo, não o feminino.
- **Acessibilidade:** anel de foco desenhado em cada família (linhas de rádio, caixas de marcar, campos de dados físicos, passo de quantidade, linha da ordem, "Usar por ele", "Cancelar"); `font-family` em todos os "Fechar"; as seis combinações de contraste corrigidas (texto em `ink-muted`; nada de opacidade em texto de informação); quadros a 320 px para as três folhas mais densas (PM-04b 5, PM-06b 10, PM-07c 11); **as etiquetas não são interativas** (a linha toda é um botão de 44 px, decisão única); confirmações em contorno `danger-ink` para "Converter o espaço", "Deixar a fúria acabar" e "Tirar o Ataque Furtivo" (com o campo de motivo e o foco em "Cancelar" quando a confirmação é no lugar).
- **Cópia:** "teste de resistência" em lugar de "salvaguarda"; a espera da concentração diz "Esperando o teste de Constituição de Sálvia"; PM-04c 11 corrigido ("Vez do Toren"; "Você está inconsciente…"); "Metamagia" para a característica.
- **Não aplicado:** os estados "que faltam" da seção E da revisão que **não** entraram na lista de decisões do Vinicius: a desvantagem de alcance e do hostil a 1,5 m desenhadas, a folha de dados físicos do Matador de Colossos, a Marca do Caçador com o alvo morto no meio do turno, a contagem do Atordoado, o fim da Fúria por "inconsciente", a Inspiração de Bardo em teste e a sua expiração, a concentração de um NPC levada a 0 PV (todos descritos nas notas, nenhum com quadro próprio); o conteúdo permanece como está.

**Perguntas do Lote 2 já respondidas:** 1 a 11 em `decisions-batch2-A.md`; as respostas 4 e 6 foram revistas pela revisão (acima).

## Lote 3: PM-09, o pacote da campanha e os links de personagem

Quadro: `PM-09-pacote-da-campanha-e-links-de-personagem-…` (5915 px; terceira fileira, y = 18000).

### Decisões
1. **Exportar** (configurações da campanha, só o mestre): lista do que vai (documento, galeria, mapas com grade/camadas/portas/luzes/armadilhas/pontos, NPCs, cenas com ações, pistas e ganchos, quebra-cabeças, pontos de batalha e encontros, pontos de tesouro, regras e conteúdo da mesa, personagens **reservados**) e do que nunca vai (anotações privadas, histórico e registros, contas). `<campanha>.meurpg.zip` com o tamanho estimado e o exato; a exportação roda no servidor com progresso e o arquivo fica 24 h.
2. **Importar** (lista de campanhas): escolher → envio **em partes e retomável** (limite de 32 MiB por pedido) → prévia (contagens, imagens, o que foi **recusado com a razão**) → "Criar campanha". Tudo ou nada; falha no meio não cria nada; pacote de versão mais nova é recusado com a razão.
3. **Reservados à parte** na lista de personagens, com o estado do link (sem link, enviado até…, assumido por…, revogado); "Criar personagem para um jogador" abre o editor de sempre em modo mestre e salva como reservado, sem aprovação.
4. **Gerar link** (7 dias por padrão; 1, 7 ou 30), uso único, mostrado **uma vez**, "Copiar link"; "Revogar" é ação irreversível (contorno danger, confirmação no lugar, foco em "Cancelar").
5. **/claim/<token>:** saído (o que é + "Entrar com Google", nada do personagem), entrado (cartão público + "Assumir este personagem"), resultado (membro e dono, link da ficha). **Uma página de erro só** para inválido/expirado/usado/revogado (mesma frase, mesmo código, mesmo tempo); a **única recusa específica** é RN-03 (já tem um personagem vivo), só depois do login, dizendo o que fazer.
6. **Segurança e RN-10:** token aleatório de 256 bits, uso único, guardado só como hash, nunca em registro; assumir entra na campanha **sem a aprovação**; um reservado é invisível aos jogadores até ser assumido e o link mostra só o cartão público; limite de taxa por IP.
7. **Novo no servidor:** `StartCampaignExport`/`GetCampaignExport`/download; `BeginCampaignImport`, `UploadImportPart`, `PreviewCampaignImport`, `CreateCampaignFromImport`; `Character.reserved`, `CreateCharacter.for_player`; `CreateClaimLink`, `RevokeClaimLink`, `claim_state`; `PreviewClaim`, `ClaimCharacter`.

### Perguntas
1. A validade do link: 7 dias padrão com 1, 7 ou 30 dias à escolha. Serve?
2. O pacote leva os retratos dos personagens? Assumi que sim (galeria).
3. RN-03 no claim: assumi que o personagem vivo existente fica com o jogador e o mestre resolve; não há troca automática.

## Lote 3 — correções (PM-09, decisões em `decisions-pm09.md`)

`PM-09` agora tem 7700 px. O que mudou:
- **O link é `/claim#t=<código>`** (fragmento, nunca caminho nem consulta); a página lê, faz `replaceState` e manda o código só em corpo de POST. Sai "a rota é mascarada".
- **Saído, a página é idêntica para todo link** (o que é + "Entrar com Google"), sem campanha nem personagem. **Entrar não assume** (`intent=character_claim` só volta ao cartão); só "Assumir este personagem" assume.
- Cartão com **"Enviado por Vinicius"** e **"Não é você? Entrar com outra conta"**; "Voltar para minhas campanhas" no lugar de "Agora não"; "Sair" no cabeçalho. Sem amarra de e-mail.
- **"Devolver à reserva"** na linha de um personagem assumido (danger com confirmação no lugar; o jogador continua membro); **Editar** e **Excluir** nos reservados (excluir revoga o link vivo); "Revogar o link" sai do diálogo e confirma na linha, sobre o mesmo personagem; quadro da revogação perdida ("Este link já foi usado por Lia").
- Casos de borda anotados e desenhados: o mestre abre o próprio link, pedido de entrada pendente, personagem morto, segundo uso, corrida com a revogação, RN-03 (única recusa específica).
- **Importação de verdade tudo ou nada:** qualquer item recusado bloqueia "Criar campanha" (prévia só oferece "Escolher outro arquivo"); limites do pacote (200 MiB, 2.000 entradas, 10 MiB por entrada, sem extrair para caminho, manifesto estrito com SHA-256), envio amarrado ao mestre com partes por 1 hora, "Tentar de novo" reaproveita as partes, "Você já tem 10 campanhas", cópia sem jargão.
- **Exportação:** lista "Nunca vai" completa (contas, donos e membros, convites e links, memória de névoa), imagens reencodadas, foto em uma transação, arquivo 24 h com "Baixar de novo", download que reconfere o mestre; `docs/privacy.md` (e a versão em português) ganham o pacote (trabalho do G1); lista de RN-10 dos reservados (lista, grupo, combate, mapas, resumo, retrato) para o teste de vazamento do G2.
- **Diálogo do link:** "Válido por 1 / 7 / 30 dias" (padrão 7), link inteiro em campo só de leitura que quebra a linha, "Copiar link" com ícone de copiar e "Link copiado." em `aria-live`, "Pronto" virou "Fechar".
- **Quadros a 320 px:** a linha de reservados, o diálogo e o cartão do cartão de assumir (e a página de entrada). Nenhum texto abaixo de 14 px; "NPCs e criaturas" em todo lugar.
- Não feito: os links do cabeçalho do app seguem como no app real (36 a 40 px de altura, decisão do Vinicius); o editor do modo do mestre não foi redesenhado (só identificado).


### Lote 2: Designer B, revisão aplicada

Aplicada a revisão adversarial (`/tmp/pm-review3/review.md`) e as respostas de `decisions-batch2-B.md` a PM-05a a d e PM-08a a d. Alturas finais: PM-05a 7487, PM-05b 7091, PM-05c 6024, PM-05d 5018, PM-08a 6584, PM-08b 5256, PM-08c 6868, PM-08d 6714. `canvas.json` atualizado.

**Decisões do Vinicius aplicadas (as perguntas 1 a 17 acima estão respondidas):**
- Inimigo Favorito segue o SRD 5.1 (a versão de 5 tipos é o Patrulheiro Revisado de 2016: conteúdo da mesa, não a classe embutida). Os tipos usam os nomes do bestiário (`creature-types.ts`: Aberração, Fera, Ínfero, Limo, Fada...), nunca "Corruptores"; as entradas `creature-type:*` têm de ter exatamente esses nomes.
- A ficha travada completa o "+1 em duas habilidades" do Meio-elfo (recalcula, e o mestre recebe o registro).
- Multiclasse: o pré-requisito é a habilidade principal de todas as classes atuais e da nova; o jogador é recusado no subir de nível e na criação; o editor do mestre pode passar por cima, com aviso. Fonte citada: SRD 5.1 "Multiclassing" (não o Livro do Jogador).
- "Recusar" segue o design.md (danger-outline, foco em "Cancelar"). Dados de vida por tipo, nesta etapa: descanso curto gasta por tipo; descanso longo devolve até metade do total (no mínimo 1), e o jogador escolhe os tipos.
- Reviver: sem regra "sem turno nesta rodada"; o combatente fica na posição e age no próximo turno. Bloqueado enquanto há outro personagem vivo; o mestre arquiva ou marca o outro antes. O diamante é um lembrete que se marca. O jogador do morto mantém a visão do grupo, mapa incluído.

**Regras e dados:**
- Pré-requisitos das invocações (Dádiva, magia): não estão nos dados do servidor (só o nível); entram num arquivo de efeitos escrito à mão, com a fonte do SRD 5.1 em cada linha, revisão nova e um teste por invocação (PM-05d, linha "Dados").
- Amostra de antecedente: Acólito (o Eremita só existe no pacote privado da mesa).
- Revivificar só com regras do SRD: toca uma criatura morta no último minuto (o toque é o alcance); a frase "que o conjurador vê" saiu. Em combate o app conta as 10 rodadas; fora de combate o alvo aparece com "o mestre confirma que faz menos de 1 minuto" e a magia pergunta ao mestre (estado 2b). A linha de Fenwick saiu. A leitura "a janela vai até a iniciativa da morte" está marcada como leitura do app.
- Lâmina Sedenta que não soma: marcada como interpretação fora do SRD. O instrumento do Bardo: nota, fora dos dados. Espaços: "132 pares ordenados" (os que não passam têm a recusa afirmada). Dominar Magia: uma magia de 1º e uma de 2º nível. O que pode mudar depois: a Dádiva não se troca (`CHOICE_ALREADY_MADE`); uma invocação se troca a cada subir de nível de Bruxo (regra do SRD, não desenhada). "Escolhas feitas" conta cada seleção em todas as telas (Kaelith: 6 de 7).

**RN-10 e privacidade:**
- A lista de alvos de Revivificar do jogador traz só quem pode ser revivido: criatura longe, vencida, escondida ou marcada não aparece e nenhum motivo (`NOT_SEEN`, `MASTER_BLOCKED`) chega ao jogador; os motivos são só do mestre. O interruptor "Revivificar não funciona nesta morte" está desenhado (PM-08d, estado 4).
- A visão do jogador do morto é a união da visão dos vivos, dita como decisão deliberada; a frase falsa saiu. A página de um morto: dono e mestre a veem inteira; os outros, só o cartão público.
- O motivo de "Pedir ajustes" é apagado ao aprovar, recusar, apagar o personagem, quando o jogador sai da campanha ou apaga a conta, quando a campanha é apagada e quando um novo pedido o substitui; depois do reenvio fica guardado para o histórico do mestre. `not_found` igual para o dono e o não membro (ReviveCharacter, ResubmitCharacter, CompleteCharacterChoices). `pending_choice_count` fica não definido (nunca 0) para quem não é dono nem mestre.

**Layout e acessibilidade:** números inativos do stepper com `ink-muted` (4,5:1 ou mais nos dois temas); alvos de 44 px ("Reviver" e o link do alerta); `font-family` no "Menu" de todos os quadros de celular; cartões da direita até 1232; ficha de fundo desenhada em PM-08a; um só botão preenchido por tela (PM-05d estado 14, PM-08c estado 5); quadros de 320 px novos (PM-05b 6d, PM-05c 9b, PM-08b 3b, PM-08c 5b); fileira de classes completa no seletor de Segredos Mágicos; filtro e bloco "Ainda não disponíveis" na lista de invocações; sem anel de foco na barra de progresso; confirmação no lugar para uma classe nova no subir de nível (danger-outline, foco em "Voltar").

**Cópia e amostras:** Goblin 1 em 2 de 7 e Brisa em 20 de 27 (coerentes com o registro e com "Ferido"/"Ferida"); o contador "125 de 500"; "Pode ser revivido" e "Quem morreu por perto"; sem texto em primeira pessoa nas notas (as decisões ficam aqui); Marlo (Força 12, Destreza 12) no lugar de Doran para o pré-requisito não cumprido.

**Não aplicado, com o motivo:**
- Foco desenhado nas linhas do painel "Personagens" (PM-05d, estado 14): com o aviso aberto o foco está no botão "Esperar os jogadores", e só há um foco por vez; as linhas ficam descritas na nota. O foco de "Reviver" e da linha de Toren está desenhado em PM-08d.
- A troca de uma invocação a cada subir de nível de Bruxo (SRD) está só citada, não desenhada: o fluxo pertence ao subir de nível e foge do escopo destes quadros.
- 320 px de PM-08b e PM-08c cobrem os cartões de classe e o resumo; a densidade do Bruxo a 320 está na lista de invocações (6d).

## Lote 4 — Designer A

### W7-M (o monstro com a ficha inteira): `W7-Ma` (ficha, ações, dano) e `W7-Mb` (estados, lendárias, testes, magias, o jogador)
- A ficha e as ações do monstro são **só do mestre**; "Ataque múltiplo" é uma sequência guiada; o ataque mostra **cada parte de dano** com a resistência de cada tipo separada (SRD, "Damage Resistance and Vulnerability").
- Contagem das 884 ações das 334 criaturas: 527 ataque com dano estruturado, 58 teste com dano, 148 ataque múltiplo, 77 em parte, 74 só texto (lembrete); 130 com recarga ou "x/dia"; 32 criaturas com ações lendárias (99 ações); 36 conjuram.
- Recarga: o **servidor rola** no começo da vez e mostra o d6; ação cinza com o motivo. "x/dia" é contador do encontro.
- Lendárias: oferta no fim do turno de outro (só mestre), custo, voltam no começo da vez dele; Resistência Lendária é um prompt quando falha, com o foco em "Deixar falhar".
- Condição de que a criatura é imune é recusada com o motivo **só ao mestre**; o jogador vê só o que acontece (RN-10, RN-20).

### W7-C (conjurar fora do combate): `W7-Ca` (lista, alvo, resultado) e `W7-Cb` (rituais, longas, ativas, mestre)
- Alvos são só quem o personagem vê; um NPC escondido nunca aparece nem tem PV mostrado. O que o motor aplica (cura rolada, PV temporários, Ajuda, Armadura Arcana) tem resultado; o resto é uma conjuração **registrada**, dita assim na tela.
- **Rituais:** seletor só para magia com a etiqueta e classe que pode (Mago do grimório, Clérigo e Druida preparadas, Bardo conhecidas); +10 minutos, sem espaço.
- **Longa:** concentração desde o início; se o combate começa antes, a conjuração se perde e o espaço não se gasta (SRD, "Longer Casting Times"). Segunda concentração pergunta antes ("Isso encerra Bênção"), danger-outline, foco em "Cancelar".
- Magias ativas com o fim e o que o descanso longo termina; o registro dos jogadores nunca mostra a conjuração de um NPC escondido.

### W7-I (o inventário na ficha): `W7-Ia` (equipamento, sintonia, números, cargas) e `W7-Ib` (dar, não identificado, dar a outro)
- **Sintonia: o jogador pede, o mestre confirma** (o descanso curto é decisão do mestre na mesa e não acontece em combate); limite de 3 e restrição de classe checados, com o motivo escrito.
- Cada número que um item muda diz "por causa de"; só com o item equipado (e sintonizado onde requer); o resto é lembrete. Os números de personagens existentes não mudam (teste dourado).
- Texto livre de hoje vira um item de texto sem perder nada, com "Transformar em itens".
- Item não identificado: o jogador lê só o aspecto ("Uma espada com runas"); o servidor não manda nome nem efeitos.
- Cargas (varinha, volta ao amanhecer), poção (ação em combate), munição (recuperar metade depois do combate, SRD).
- Perguntas: (1) "Flechas" e "Arma, +1" não têm entrada em `names_pt.json`; (2) o mestre prefere sintonizar direto em vez de aprovar?


## Lote 4 — Designer A, correções

Aplicadas as revisões de `/tmp/pm-review5` como decididas em `decisions-batch4-A.md` (a decisão vence a revisão). Scripts: `build_w7m.py`, `build_w7c.py`, `build_w7i.py` (importam `w7alib`, não `w7lib`, que é do Designer B). Todos os quadros novos têm a versão de 320 px desenhada, texto de 14 px ou mais, nota "Servidor" e os estados vazio/carregando/erro onde há lista.

**Quadros (linha y=27000, x de 0; B começa em 13600):** W7-Ma 3987, W7-Mb 4147, W7-Mc 4207, W7-Ca 2388, W7-Cb 4560, W7-Cc 4304, W7-Ia 5451, W7-Ib 4115, W7-Ic 2587, W7-Id 7563 (todos abaixo de 8000 px). Os antigos W7-Ca/Cb/Ia/Ib foram substituídos.

### W7-M
- Resistências seguem **quem é dono** da criatura (passos do jogador x do mestre); dano de Ragna 19 → 9 + fogo 7 = 16; esqueleto com veneno imune; lendárias 3/3, 1 restante, 0 de 3 gastos; Resistência Lendária com foco em "Deixar falhar"; o jogador vê só nome (ou "Criatura desconhecida") e a faixa de estado.

### W7-C
- Elenco: Ilaria (Clérigo 5, Domínio da Vida) e Pensantus (Mago 5); Curar Ferimentos com a linha "1d8 (6) + 3 + 3 (Discípulo da Vida) = 12"; Ajuda (+5, até 3 alvos, 9 m); Armadura Arcana recusa quem está de armadura.
- **Começar o combate não cancela a conjuração longa**; ela usa a ação do conjurador a cada turno; a concentração quebrada a faz falhar sem gastar o espaço (exemplo com Glifo de Vigilância, não ritual). Fora do combate o mestre toca "Concluir conjuração".
- **Durações são tempo de jogo**, nunca hora do relógio ("dura 8 horas"). Uma concentração só, com confirmação danger ("Isso encerra Bênção"); o mestre "Encerra" com confirmação; o mestre conjura por um NPC e a conjuração de NPC escondido nunca chega aos jogadores (RN-10). Sem "Ação usada" fora do combate.
- Rituais: tempo da magia + 10 minutos (Alarme 11 minutos); tabela de quem conjura rituais.

### W7-I
- Números: Toren Guerreiro 4, Força 19, Espada longa +1 = +7 e 1d8 + 5; Pedra iônica (proteção) só +1 de CA; "Cajado do arcano" com o rótulo `attunement:*`.
- **Sintonia e identificação entram no descanso curto do mestre** (mesmo fluxo e mesma transação do PM-07b); o jogador marca "Sintonizar no próximo descanso curto" ou "Encerrar a sintonia"; máximo 3; item amaldiçoado não se solta.
- **Não identificado:** o jogador recebe só o aspecto, a quantidade e se está equipado; os efeitos só valem após identificar (o mestre vê "efeitos esperando"); itens iguais não identificados nunca se juntam; "Identificar" gera linha no registro, que nomeia o item como o ator o vê.
- Pergaminho de magia (lista da classe, tempo normal, teste CD 10 + nível da magia, mantém-se se interrompido), quatro poções de cura, "Dar a alguém para beber" (ação) x "Dar", varinha com d20 na última carga, armadura recusada em combate e escudo = ação, munição recupera metade (arredondar para baixo é regra do app), editar moedas, "Transformar em itens" com prévia e confirmação.

### Perguntas abertas
- Nomes que faltam em `names_pt.json`: Flechas, Item maravilhoso, Pedra iônica (proteção), Presença Aterradora, Sopro de Fogo e o Book of Ancient Secrets (sem nome em português).
- Arredondamento da munição recuperada: regra do app, a confirmar com Samuel.

## Lote 4 — Designer B

Linha y = 27000, x a partir de 13600. Nove placas de efeitos, disputas e zonas, mais as correções das revisões.

| Placa | x | Altura | Conteúdo |
| --- | --- | --- | --- |
| W7-Ea, Eb, Ec (efeitos que duram) | 13600, 14960, 16320 | 6462, 7215, 6272 | Regras e as 15 condições, lista e turno, painel do mestre, visibilidade, dados de Bênção e Perdição, Velocidade, Teia, exaustão e o contrato. |
| W7-Xa, Xb, Xc (disputas e ações especiais) | 17680, 19040, 20400 | 4675, 7808, 7824 | Regras e disputa de um jogador contra um NPC; o jogador como alvo, escapar, arrastar e empurrar; esconder, ajudar, teste em grupo, surpresa e o contrato (estado 12). |
| W7-Za, Zb, Zc, Zd (zonas no mapa) | 21760, 23120, 24480, 25840 | 6258, 5578, 7051, 7263 | Regras das 12 magias; gatilhos e zonas que se movem; Muralha de Fogo, Crescer Espinhos, Silêncio, Escorregadia, Constrição e Névoa Mortal; lista do mestre, “Pôr uma zona”, teatro da mente e o contrato. |

### Decisões (todas as que mudam o jogo estão nas placas, estas são as de desenho)
- **Elenco único.** Brisa (Ladino 5, Furtividade +7, espada curta), Toren (Guerreiro 4, espada longa +5), Tavo, Nael, Orla, Ragna; nas zonas, Pensantus, Sálvia e Ilaria (PM-02). Inimigos só do SRD: Goblin, Hobgoblin, Capitão bandido, Cobra constritora gigante, Fanático do culto; “Zuk” é um goblin do mestre.
- **RN-10 e RN-20 em tudo.** O jogador lê a habilidade de um teste de um NPC, nunca a CD; esperas dizem “Esperando o mestre”; um efeito ou zona com a visibilidade desligada não aparece em nenhum canal (espera, fonte de vantagem, crítico, registro, contagem).
- **Um mecanismo.** Todo teste que espera resposta (fim de turno, disputa, zona) é uma janela de reação do PM-04 (`EFFECT_SAVE`, `CONTEST`, `ZONE_SAVE`), respondida com `AnswerReaction`, sem esperas novas.
- **Esconder:** antes do ataque o jogador lê só “Você está escondida.”; atacar encerra o esconderijo para todos; empate mantém a criatura notada (leitura do app).
- **Zonas:** simétrico para o muito obscurecido (ninguém vê dentro, fora ou através, nem quem está dentro); uma zona que se move sobre uma criatura conta como entrada (leitura do app); o raio de 1,5 m do Raio Lunar são 5 quadrados (regra do centro de PM-02, de propósito); um dano por criatura, zona e turno; Crescer Espinhos fica desconhecido até ser reconhecido; “Os jogadores veem esta zona” é uma chave por zona (desligada para o Silêncio).
- **Metros e 14 px.** Distâncias em metros (5 pés = 1,5 m), nenhum texto de produto abaixo de 14 px, alvos de 44 px (48 px no botão cheio do celular), um quadro de 320 px para cada cartão novo.

### Regras verificadas no SRD 5.1
Conditions (as 15, mais exaustão), Duration e Concentration (o que for maior), Dodge, Help, Hide e Unseen Attackers and Targets, Contests, Grappling, Escaping a Grapple, Moving a Grappled Creature, Shoving a Creature, Group Checks, Surprise, Vision and Light, Areas of Effect, e o texto das magias Bênção, Perdição, Imobilizar Pessoa, Velocidade, Teia, Névoa Obscurecente, Muralha de Fogo, Crescer Espinhos, Escuridão, Névoa Fétida, Névoa Mortal, Silêncio, Espíritos Guardiões, Raio Lunar, Área Escorregadia e Constrição. Habilidades de criaturas conferidas na base 5e-database (Hobgoblin: Força 13, sem Atletismo).

### Perguntas abertas
- Leituras do app que o SRD não decide, a confirmar com Samuel: o empate no esconderijo mantém a criatura notada; a ação bônus de quem está surpreso; o empurrão contra parede vira “não sai do lugar”; o simétrico do muito obscurecido; a zona que se move conta como entrada; o custo de terreno difícil dentro de Espíritos Guardiões (quádruplo).
- Crescer Espinhos junta os 2d4 de todo o movimento em uma rolagem (rapidez); uma rolagem por quadrado é a alternativa.
- Conjuradores das zonas: Muralha de Fogo (4º nível) é de Maleck, mago do mestre (Mago 7); Névoa Mortal (5º nível) é de Vorn, feiticeiro do mestre; Área Escorregadia é de Pensantus; Constrição, de Sálvia; Zuk mantém a Névoa Fétida. A Teia presa ou solta é pergunta do mestre.
- Placas de zonas sem terceira rodada de revisão depois desta.

### PM-02, decisão de 09/10 (depois da verificação)

- **"Perguntar a cada vez" segura o turno em TODA magia de área que um jogador conjura em combate**, e não só quando há uma criatura escondida na área: o mestre responde com um toque ("Sem escondidas" quando não há nenhuma). Se a espera só aparecesse com uma escondida atingida, o "Esperando o mestre" contaria ao jogador que havia algo ali (RN-10), o mesmo problema que a regra "Reações dos inimigos: Sempre" resolve. "Revelar" (o padrão) e "Manter escondidas" não seguram nada.
