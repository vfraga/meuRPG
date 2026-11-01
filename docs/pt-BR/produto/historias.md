[English](../../product/stories.md)

# Histórias e critérios de aceite

O MVP tem 45 histórias: 43 histórias mais 2 pré-requisitos (o convite, MR-002, que leva à MR-003, e os NPCs, MR-005, que são os inimigos do combate).
Outras dez histórias ficam para depois do MVP.

Uma história está pronta quando todos os critérios dela passam.
Cada critério vira um teste automático: Playwright para o que aparece na tela, teste em Go para a regra no servidor (ver [Testes](../../testing.md)).
O sistema novo só precisa provar os próprios critérios de aceite.

Como ler uma história:

- a frase da história;
- a prioridade (MVP, MVP pré-requisito ou Depois);
- as regras que ela segue (RN-xx, em [Regras de negócio](regras.md)) e os módulos que ela toca;
- os critérios de aceite (Dado / quando / então);
- "No app" e "Relacionadas", notas curtas sobre onde ler mais.

As histórias marcadas como Depois são propostas.
O modelo de dados já nasce preparado para a MR-021 e a MR-022 (ver [Modelo de dados](../../data/README.md)).
Quando um comportamento veio de uma resposta do produto, o [arquivo](../../archive/decisions.md) guarda isso.

## Índice

| ID | Área | Prioridade |
| --- | --- | --- |
| [MR-001](#mr-001-criar-campanha) | Campanha | MVP |
| [MR-003](#mr-003-entrar-pelo-convite) | Personagem | MVP |
| [MR-004](#mr-004-ficha-no-formato-do-pdf) | Personagem | MVP |
| [MR-006](#mr-006-ficha-travada) | Personagem | MVP |
| [MR-008](#mr-008-pontos-de-interesse) | Mapa | MVP |
| [MR-009](#mr-009-mapa-sem-spoiler) | Mapa | MVP |
| [MR-011](#mr-011-iniciar-a-sessão) | Sessão | MVP |
| [MR-012](#mr-012-acompanhar-a-sessão) | Sessão | MVP |
| [MR-013](#mr-013-ordem-dos-turnos) | Combate | MVP |
| [MR-014](#mr-014-sua-vez) | Combate | MVP |
| [MR-015](#mr-015-ações-da-cena-de-rp) | RP | MVP |
| [MR-016](#mr-016-dar-xp) | Progressão | MVP |
| [MR-018](#mr-018-documento-de-campanha) | Apoio | MVP |
| [MR-019](#mr-019-galeria-de-imagens) | Apoio | MVP |
| [MR-024](#mr-024-aprovar-o-personagem-do-convite) | Personagem | MVP |
| [MR-028](#mr-028-mostrar-uma-imagem-aos-jogadores) | Sessão | MVP |
| [MR-025](#mr-025-cadastrar-conteúdo-da-mesa) | Regras | MVP |
| [MR-010](#mr-010-gerar-masmorras) | Masmorra | MVP |
| [MR-029](#mr-029-ganchos-e-pistas-da-cena) | RP | MVP |
| [MR-030](#mr-030-anotações-do-jogador) | RP | MVP |
| [MR-031](#mr-031-npcs-na-cena) | RP | MVP |
| [MR-032](#mr-032-destaques-do-combate) | Combate | MVP |
| [MR-033](#mr-033-imprimir-o-mapa-com-a-grade) | Mapa | MVP |
| [MR-034](#mr-034-movimentos-especiais) | Combate | MVP |
| [MR-035](#mr-035-armadilhas) | Mapa | MVP |
| [MR-036](#mr-036-névoa-de-guerra-pela-visão) | Mapa | MVP |
| [MR-037](#mr-037-criaturas-do-personagem) | Combate | MVP |
| [MR-038](#mr-038-quebra-cabeças) | Sessão | MVP |
| [MR-039](#mr-039-imagens-geradas-para-masmorras-e-cenas) | Apoio | MVP |
| [MR-040](#mr-040-subir-de-nível-pela-ficha) | Progressão | MVP |
| [MR-041](#mr-041-tesouros-e-xp-por-ouro) | Mapa | MVP |
| [MR-042](#mr-042-bestiário) | Combate | MVP |
| [MR-043](#mr-043-gerar-encontros) | Combate | MVP |
| [MR-044](#mr-044-gerar-tesouro) | Mapa | MVP |
| [MR-045](#mr-045-consultar-as-magias) | Regras | MVP |
| [MR-048](#mr-048-conjurar-magias-fora-do-combate) | Combate | MVP |
| [MR-050](#mr-050-pacote-da-campanha-exportar-e-importar) | Campanha | MVP |
| [MR-051](#mr-051-conversa-da-cena) | Sessão | MVP |
| [MR-052](#mr-052-o-rolador-de-dados-livre) | Sessão | MVP |
| [MR-049](#mr-049-personagens-reservados-e-links-para-assumir) | Personagem | MVP |
| [MR-053](#mr-053-o-mestre-age-pelo-personagem-de-um-jogador) | Combate | MVP |
| [MR-054](#mr-054-usar-a-ação-de-uma-criatura-no-turno-dela) | Combate | MVP |
| [MR-055](#mr-055-responder-ao-efeito-extra-de-uma-criatura) | Combate | MVP |
| [MR-002](#mr-002-gerar-convite) | Campanha | MVP (pré-requisito) |
| [MR-005](#mr-005-criar-npcs) | Personagem | MVP (pré-requisito) |
| [MR-007](#mr-007-importar-ficha-em-pdf) | Personagem | Depois |
| [MR-017](#mr-017-subir-de-nível) | Progressão | Depois |
| [MR-020](#mr-020-consultar-o-livro-de-regras) | Apoio | Depois |
| [MR-021](#mr-021-copiar-personagem) | Personagem | Depois |
| [MR-022](#mr-022-reutilizar-npcs) | Personagem | Depois |
| [MR-023](#mr-023-passar-ou-dividir-a-campanha) | Campanha | Depois |
| [MR-026](#mr-026-propor-uma-raça-ou-classe-nova) | Regras | Depois |
| [MR-027](#mr-027-ler-as-regras-de-um-pdf) | Regras | Depois |
| [MR-046](#mr-046-o-estilo-da-mesa-recurso-por-recurso) | Regras | Depois |
| [MR-047](#mr-047-mais-quebra-cabeças) | Sessão | Depois |

## Prioridade: MVP

### MR-001: Criar campanha

**Como** mestre, **quero** criar uma campanha que agrupe as sessões, os personagens e os mapas, **para que** cada mesa fique organizada por conta própria.

- Prioridade: MVP
- Regras: RN-05, RN-30
- Módulos: campaigns

#### Critérios de aceite
- **Dado** que estou conectado, **quando** crio a campanha "Mirathel", **então** viro o mestre dela **e** só os membros dela a veem na lista.
- **Dado** que já sou mestre do número máximo de campanhas de uma conta (10 por padrão), **quando** crio outra, **então** nada é criado **e** o formulário mostra uma frase com o máximo (RN-30).
- **Dado** que o servidor deixa só alguns e-mails criarem campanhas, **quando** uma conta com outro e-mail tenta criar uma, **então** ela é recusada com o motivo e a tela explica; quem entra por convite continua jogando (RN-30).

#### No app
- A lista em `/campaigns` mostra o papel em cada campanha ("Mestre" ou "Jogador") e o formulário "Criar campanha". Uma lista vazia explica como criar uma campanha ou entrar por convite.
- A tela não sabe o máximo antes de o servidor recusar.
- Ver [Arquitetura](../../architecture/campaigns.md#campaigns-and-authorization).

### MR-003: Entrar pelo convite

**Como** jogador, **quero** criar meu personagem pelo link do convite, **para que** ele fique ligado à campanha desde o início.

- Prioridade: MVP
- Regras: RN-03
- Módulos: characters, campaigns

#### Critérios de aceite
- **Dado** um convite válido para "Mirathel", **quando** o jogador abre o link e entra com o Google, **então** ele vira jogador da campanha e cria um personagem do tipo jogador, que o mestre já vê na campanha.
- **Dado** um convite expirado, **quando** alguém abre o link, **então** vê uma mensagem clara **e** nada é criado.
- **Dado** um visitante que não está conectado, **quando** ele abre o link do convite e entra, **então** ele ingressa nesse mesmo passo, e o token do convite não aparece em nenhum endereço que o navegador requisita.
- **Dado** uma pessoa que já é membro, **quando** ela aceita o convite de novo, **então** nada muda.
- **Dado** um convite com um uso restante, **quando** duas pessoas o aceitam ao mesmo tempo, **então** só uma entra.
- **Dado** um jogador na campanha, **quando** ele cria o personagem, **então** o personagem começa como rascunho, e o mestre o vê na lista da campanha com o nome do jogador, a classe e a raça. O editor é o editor passo a passo da [MR-004](#mr-004-ficha-no-formato-do-pdf).

#### No app
- `/invite` lê o token do endereço, apaga-o na hora e oferece "Entrar para aceitar o convite" a um visitante desconectado.

#### Relacionadas
- RN-03: um jogador só cria um personagem novo nesta campanha quando o atual morre; o personagem morto continua no sistema (ver [Regras de negócio](regras.md)).
- A RN-17 descreve a entrada de um jogador sem Google (ainda não construída; ver [ADR-0009](../../adr/0009-table-accounts-for-players-without-google.md)).
- Quando o convite exige aprovação (RN-15), o jogador entra como membro pendente, cria o personagem, e ele fica pendente até o mestre aprovar ou recusar. Ver [MR-024](#mr-024-aprovar-o-personagem-do-convite).

### MR-004: Ficha no formato do PDF

**Como** jogador, **quero** ver minha ficha num formato próximo ao do PDF oficial, **para que** eu encontre tudo onde estou acostumado.

- Prioridade: MVP
- Regras: —
- Módulos: characters, rules

#### Critérios de aceite
- **Dado** um personagem completo, **quando** o jogador abre a ficha no celular, **então** vê as seções da ficha oficial (habilidades, perícias, combate, magias, equipamento) **e** os valores calculados, como modificadores, classe de armadura, pontos de vida, CD de resistência das magias e ataque mágico, vêm prontos do servidor.
- **Dado** o "Equipamento" da ficha, **quando** o jogador o lê, **então** ele lista a armadura, o escudo, as armas e os itens, depois o equipamento do antecedente como texto ("Do antecedente: …", o de um antecedente da mesa ou o que o jogador escreveu para um antecedente "Outro" personalizado), depois as moedas. Uma arma versátil mostra o dano com duas mãos abaixo do dano normal ("Com duas mãos: 1d8+1").
- **Dado** o passo "Perícias", **quando** a raça, a sub-raça ou o antecedente já dão uma perícia, **então** ela aparece marcada e travada com a origem ("da raça", "do antecedente") e não conta como escolha. O jogador escolhe outra perícia em vez de repetir uma proficiência, como o SRD permite.
- **Dado** um personagem novo feito acima do nível 1 numa campanha que sobe de nível por XP de inimigos derrotados, **quando** o editor abre, **então** o campo de experiência começa no XP do nível dele (6.500 no nível 5) e acompanha o nível até o jogador digitar nele. Uma campanha por marcos mantém 0.
- **Dado** o campo de subclasse, **quando** o jogador edita a classe, **então** ele oferece "Nenhuma", diz em que nível a classe escolhe a subclasse ("O Bárbaro escolhe a subclasse no nível 3.") e troca a subclasse quando a classe muda.
- **Dado** o passo "Magias", **quando** o jogador o abre, **então** a lista mostra só magias até o maior nível da magia que o nível atual permite, ordenadas por nível da magia e depois por nome. Uma magia já escolhida acima do limite fica, marcada "acima do nível", para poder ser desmarcada.
  - Uma classe que começa a conjurar mais tarde diz isso ("O Paladino conjura magias a partir do nível 2." no nível 1).
  - "Truques" aparece só para uma classe com truques, ou quando a ficha já tem um.
  - Cada lista conta contra os números da classe ("3 de 4 truques escolhidos"). Uma classe que prepara magias (clérigo, druida, paladino, mago) com menos magias preparadas do que pode ver "Prepare até N", e o "Prepare mais N" da subida de nível diminui com as escolhas.
- **Dado** o passo "Habilidades" de um jogador criando uma ficha, **quando** a mesa permite vários métodos (RN-24, [MR-025](#mr-025-cadastrar-conteúdo-da-mesa)), **então** o jogador escolhe entre eles e o servidor rola e guarda os 4d6.
- **Dado** o editor livre dos valores de habilidade (o que o mestre vê para NPCs e o que a edição de uma ficha sem método registrado mantém), **quando** o mestre abre "Como definir os valores", **então** há três cartões: "Digitar", "Rolar 4d6" e "Conjunto padrão".
  - "Rolar 4d6" rola 4d6 seis vezes no navegador, risca o dado mais baixo de cada um, lista os resultados do maior para o menor e tem "Rolar de novo", que substitui os seis e desfaz a distribuição.
  - O jogador põe cada resultado numa habilidade: um seletor em tablet ou desktop (escolher um resultado já usado troca os dois), toque no resultado e depois na habilidade no celular.
  - "Conjunto padrão" funciona do mesmo jeito com 15, 14, 13, 12, 10 e 8.
  - Enquanto um resultado não tem habilidade, o editor não salva e diz "coloque cada resultado numa habilidade".
  - Essas rolagens não são registradas e não fazem parte da RN-18 (os 4d6 do jogador da RN-24 fazem). A ficha continua editável até a primeira sessão e o mestre a revisa.
- **Dado** o passo "Pontos de vida" com "Rolado", **quando** o jogador o abre, **então** há uma linha por nível a partir do 2º ("Nível 2 (1d12)"), com "Rolar" e "Rolar os níveis que faltam" (só os vazios; o jogador pode digitar o que saiu num dado de mesa).
  - A fórmula aparece como "1d12 (8) + 3 (Constituição) = 11 PV", com a Constituição final.
  - Um multiclasse rola cada nível com o dado da classe que o dá.
  - O editor não salva enquanto um nível não tem rolagem ou tem uma rolagem que não cabe no dado, e nomeia o nível.
  - O que raça, classe e características somam (por exemplo um ponto de vida por nível para um Anão da Colina) vem da prévia do rascunho feita pelo servidor e é nomeado numa linha curta ("+1 PV de raça, classe ou característica"). Se a prévia falha, a caixa mostra só os dados e diz que a prévia deixa esses pontos de vida de fora.
- **Dado** uma magia no editor, **quando** o jogador toca no "?" ao lado dela, **então** a descrição abre (diálogo no desktop, painel inferior no celular): nome, nível e escola da magia, tempo de conjuração, alcance (em metros: 5 ft = 1,5 m), componentes, duração, as etiquetas Ritual e Concentração e o texto do SRD em português, mais "Em níveis superiores" quando houver.
  - "Ver em inglês" / "Ver em português" troca o texto para o inglês do SRD no app inteiro até a página recarregar.
  - Uma magia sem tradução mostra o inglês, marcado "(em inglês)".

#### Relacionadas
- O motor de regras calcula a ficha a partir de regras como dados. Ver [ADR-0008](../../adr/0008-rules-content-as-data-pure-engine.md) e [Arquitetura](../../architecture/characters.md).

### MR-006: Ficha travada

**Como** mestre, **quero** que a ficha do jogador seja só de leitura a partir da primeira sessão, **para que** só eu e o sistema a alteremos.

- Prioridade: MVP
- Regras: RN-01
- Módulos: characters

#### Critérios de aceite
- **Dado** que a primeira sessão da campanha começou, **quando** o jogador tenta editar as habilidades da própria ficha, **então** o servidor recusa **e** o mestre pode editar a mesma ficha.
- **Dado** que nenhuma sessão começou, **quando** o jogador edita a ficha, **então** a alteração é salva.
- **Dado** um personagem criado depois da primeira sessão, **quando** o jogador edita a ficha antes da próxima sessão, **então** a alteração é salva **e**, quando a próxima sessão começa, a ficha trava.
- **Dado** que a ficha está travada, **quando** o jogador tenta editar a história do personagem, **então** o servidor recusa; **depois** que o mestre destrava a história desse personagem, o jogador edita e salva, **e** a permissão termina quando a próxima sessão começa.
- **Dado** uma ficha travada, **quando** o jogador abre o endereço de edição dela (digitado ou salvo nos favoritos), **então** a tela mostra "Ficha travada" (ou "Personagem morto") com o motivo e "Voltar para a ficha", antes de qualquer formulário.
- **Dado** que a ficha do jogador está aberta quando o mestre inicia uma sessão, **quando** a sessão começa, **então** a ficha mostra "Travada" sem recarregar ("Editar ficha" dá lugar a ela).

#### No app
- Iniciar uma sessão trava as fichas na mesma transação. O jogador recebe uma recusa com o motivo `SHEET_LOCKED` ou `STORY_LOCKED`.

### MR-008: Pontos de interesse

**Como** mestre, **quero** criar pontos de interesse que abram uma batalha, um submapa ou uma cena de RP.

- Prioridade: MVP
- Regras: —
- Módulos: maps

#### Critérios de aceite
- **Dado** um mapa da campanha, **quando** o mestre cria um ponto do tipo batalha, submapa ou cena de RP, **então** o ponto aparece no mapa, escondido no início, **e** abrir o ponto leva ao encontro, ao submapa ou à cena.
- **Dado** um mapa criado a partir de uma imagem da galeria, **quando** o mestre o cria, **então** ele começa escondido.
- **Dado** um ponto de submapa, **quando** o mestre escolhe o destino, **então** ele pode levar a outro mapa da mesma campanha, nunca ao próprio mapa.
- **Dado** um ponto de submapa que leva a um mapa, **quando** o mestre olha o painel do ponto no editor, **então** "Abrir <mapa>" leva o mestre ao editor desse mapa; **e** na sessão ao vivo a linha do ponto tem "Ir para <mapa>", que faz desse mapa o mapa atual da sessão (um destino escondido é revelado, como no seletor "Mapa atual").
- **Dado** um ponto de submapa ainda sem mapa, **quando** o mestre escolhe "Criar mapa novo…" em "Leva para", dá nome ao mapa (o nome do ponto por padrão) e escolhe uma imagem da galeria, **então** o mapa é criado escondido, vira o destino do ponto e o ponto é salvo num passo só, com "Abrir" para montar o mapa novo.
- **Dado** um ponto de submapa, **quando** o mestre o abre, **então** o cartão do ponto mostra o nome e a descrição com "Abrir <mapa>". Os pontos de batalha e de cena abrem o encontro de combate e a cena de RP (ver [MR-013](#mr-013-ordem-dos-turnos) e [MR-015](#mr-015-ações-da-cena-de-rp)); fora de uma sessão, mostram o cartão do ponto.
- **Dado** um mapa, **quando** o mestre o renomeia ("Renomear") ou o apaga ("Apagar mapa"), **então** a exclusão pergunta ali mesmo o que vai junto (os pontos e as peças; a imagem fica na galeria) e avisa quando ele é o mapa atual da sessão aberta. Depois de apagar, o app volta para a campanha.
- **Dado** um mapa que não pode ser apagado (um combate roda nele, um tesouro virou XP, um tesouro foi encontrado), **quando** o mestre olha "Apagar mapa", **então** o botão já está cinza, ainda clicável, com o primeiro motivo embaixo; se o servidor recusar mesmo assim, a pergunta diz o mesmo motivo.

#### No app
- No editor, escolher o tipo e clicar no mapa coloca o ponto; arrastar ou usar as setas o move; "Salvar ponto" salva o painel. No celular o mestre só move a vista, dá zoom e revela pelas listas. Ver [Design](../../design/session.md#images-shown-to-players) e [Arquitetura](../../architecture/maps/README.md#maps-points-and-tokens).

### MR-009: Mapa sem spoiler

**Como** jogador, **quero** ver no mapa só os pontos que meu grupo já conhece, **para que** eu não receba spoilers.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps

#### Critérios de aceite
- **Dado** um mapa com um ponto revelado e um ponto escondido, **quando** o jogador abre o mapa, **então** só o revelado aparece **e** a resposta do servidor não contém o escondido.
- **Dado** um mapa escondido, **quando** o jogador o pede, **então** a resposta é a mesma de um mapa que não existe ("Mapa não encontrado"); um jogador alcança só os mapas revelados e o mapa atual da sessão.
- **Dado** uma mudança só em coisas escondidas, **quando** ela acontece durante uma sessão, **então** ela chega só ao mestre pela sessão ao vivo.

#### No app
- A tela do jogador mostra a trilha de submapas, "Mapas revelados" e "Pontos deste mapa". As peças que o mestre não tornou visíveis nunca chegam.

### MR-011: Iniciar a sessão

**Como** mestre, **quero** iniciar a sessão, fazer os jogadores receberem um aviso no app e ter um link da sessão para enviar a eles, **para que** todos entrem juntos.

- Prioridade: MVP
- Regras: RN-06, RN-07
- Módulos: play, campaigns, characters

#### Critérios de aceite
- **Dado** uma campanha com três jogadores, **quando** o mestre inicia a sessão, **então** quem está com o app aberto vê o aviso **e** o mestre pode copiar o link da sessão.
- **Dado** o link da sessão, **quando** alguém que não é membro (ou é membro pendente) o abre, **então** vê "Peça um convite ao mestre", sem o nome da campanha, **e** não entra.
- **Dado** que é a primeira sessão da campanha, **quando** o mestre inicia a sessão, **então** as fichas dos jogadores travam.
- **Dado** personagens de jogador cuja ficha ainda tem escolhas em aberto (perícias, truques, magias conhecidas e preparadas), **quando** o mestre inicia a sessão, **então** o cartão "Sessão" os lista ("Ilaria: faltam 1 perícia") e pergunta "Iniciar mesmo assim?". As fichas travam como estão, e o mestre decide. Sem nenhuma ficha assim, a sessão começa na hora. Só o mestre vê essa lista.
- **Dado** um membro sem sessão aberta, **quando** ele abre a página da sessão, **então** vê "Nenhuma sessão em andamento", e a página abre a sessão sozinha quando o mestre a inicia.

#### No app
- O aviso "A sessão 4 de Mirathel começou." fica sob a barra do app com "Entrar na sessão", para quem joga na campanha, fora das páginas da campanha e da sessão. Fechá-lo vale só para a aba. O app verifica as sessões abertas a cada 30 segundos enquanto a aba está visível. O aviso na tela basta: não há notificação push do navegador (RN-06).
- Outras entradas: o link "Ao vivo" na barra, a etiqueta "Sessão ao vivo" em "Minhas campanhas" e a faixa da sessão na página da campanha.
  A faixa do mestre fica acima das três abas, então está em todas: "Encerrar sessão", "Copiar link da sessão" e "Entrar na sessão" (o único botão preenchido da página), ou "Iniciar sessão" enquanto não há nenhuma aberta. A lista das fichas com escolhas em aberto ("Iniciar mesmo assim?") abre embaixo dela.
  No celular a faixa mantém "Entrar na sessão" e um menu "Mais ações da sessão" com "Copiar link da sessão" e "Encerrar sessão".
  A página da campanha de um jogador mostra a faixa só enquanto há sessão aberta, com "Entrar na sessão".
- Na página da sessão, uma barra de uma linha no alto reúne a volta para a campanha, "Sessão 3", "Ao vivo desde 20:12" e "Dados" para todos. O mestre também tem "Iniciar combate" (fora de um combate) e o menu "Sessão" com "Copiar link da sessão" e "Encerrar sessão", que pergunta no lugar ("Encerrar a sessão 3? Ela não reabre depois.", "Cancelar" primeiro). No celular, a navegação "Cena · Ficha · Conversa" fica fixa no pé da tela.
- Ver [Arquitetura](../../architecture/play/live-session.md).

#### Relacionadas
- RN-07: os convites têm por padrão 1 uso e 7 dias; o mestre escolhe de 1 a 20 usos e de 5 minutos a 30 dias, e pode revogar (ver [MR-002](#mr-002-gerar-convite)).

### MR-012: Acompanhar a sessão

**Como** jogador, **quero** acompanhar minha ficha e o mapa atual durante a sessão.

- Prioridade: MVP
- Regras: RN-02, RN-10, RN-11, RN-20
- Módulos: play, maps

#### Critérios de aceite
- **Dado** uma sessão ativa, **quando** o mestre move uma peça ou o sistema aplica dano ao personagem, **então** o celular do jogador mostra a mudança sem recarregar a página **e** as anotações do mestre nunca aparecem (RN-11).
- **Dado** a sessão ao vivo, **quando** o jogador olha a metade da ficha, **então** vê os pontos de vida, os pontos de vida temporários, a classe de armadura, os dados de vida e os espaços de magia só do próprio personagem, nunca de outro jogador.
- **Dado** que o mestre abre "Ajustar" (painel inferior no celular, diálogo no desktop), **quando** ele corrige pontos de vida, pontos de vida temporários, espaços de magia ou dados de vida, **então** o dono e o mestre veem a mudança na hora, o valor nunca passa do máximo da ficha, e depois que a sessão acaba a ficha diz "A sessão acabou" (RN-02).
- **Dado** que a conexão cai, **quando** o jogador olha a página, **então** ela diz "Reconectando…" com a hora da última atualização, e os números ficam na tela.
- **Dado** que o mestre escolhe o mapa atual da sessão, **quando** um jogador está na sessão, **então** o mapa do jogador o acompanha; sem mapa escolhido, o aviso diz "O mestre ainda não escolheu um mapa."
- **Dado** que o mestre move uma peça visível, **quando** o jogador está com o mapa aberto, **então** a peça se move na tela do jogador sem recarregar. Uma peça que o mestre esconde (a de um NPC, por exemplo) chega só ao mestre. Se o jogador perde o mapa de vista, ele volta ao aviso.
- **Dado** a tela de sessão do mestre, **quando** o mestre olha o mapa, **então** há o seletor "Mapa atual", peças arrastáveis, "Pontos do mapa" e "Tokens no mapa" com "Revelar aos jogadores" e "Esconder". A criatura de um personagem tem a própria linha, sempre visível, e arrastar a peça dela move a criatura, não o dono.
- **Dado** um mapa com grade e sem combate, **quando** o mestre arrasta a peça de um personagem ou a move pela aba "Tokens", **então** a peça anda em direção ao quadrado e para antes de uma parede, de uma porta trancada ou de um quadrado marcado "Não dá para passar", a tela diz onde e por quê ("O movimento parou antes. O próximo quadrado está marcado como “Não dá para passar”. Tor andou 7,5 m."), e nada se move até o servidor responder. Várias peças escolhidas juntas andam em volta da primeira; se uma não couber, ninguém se move e a tela diz quem está no caminho.
- **Dado** que o interruptor "Os jogadores movem os próprios personagens" está ligado no mapa, **quando** um jogador arrasta a própria peça, **então** ela anda do mesmo jeito; um jogador nunca move a peça de outro personagem, e com o interruptor desligado só o mestre move peças. O jogador nunca descobre por que a peça parou num quadrado que ele não viu (RN-10): a tela diz só "O movimento parou antes.".
- **Dado** que o interruptor está desligado (o padrão) ou ligado, **quando** um jogador olha o mapa, **então** uma linha embaixo dele diz quem move: "O mestre move os tokens agora." sem nenhum controle, ou "Arraste Brisa no mapa ou use “Mover Brisa”. Os outros ficam com o mestre." com o botão "Mover Brisa" só para o próprio personagem. **Quando** o mestre libera ou trava o movimento enquanto o jogador olha, **então** a linha vira um aviso educado de uma linha por 8 segundos ("O mestre liberou: agora você move Brisa." ou "O mestre voltou a mover os tokens."), e travar fecha o painel de movimento aberto sem mover.
- **Dado** um combate, **quando** o jogador vê um inimigo, **então** vê uma palavra (Ileso, Ferido, Muito ferido, Derrotado), não pontos de vida (RN-20).
- **Dado** um ataque que acerta, **quando** ele causa dano a um NPC, **então** o dano é aplicado na hora (pontos de vida temporários primeiro; com 0 pontos de vida o NPC é derrotado e sai da ordem).
- **Dado** um ataque ou uma magia que causa dano ao personagem de um jogador, **quando** ele acerta, **então** o dano espera o mestre, que o aplica ("Aplicar 5 de dano", ou outro valor) ou o descarta. Quando aplicado, o dono e o mestre o veem ao vivo. Dano a quem está com 0 pontos de vida conta uma falha no teste contra a morte (RN-03). Uma cura é aplicada na hora e levanta quem está com 0.
- **Dado** o mestre, **quando** ele precisa corrigir algo, **então** pode editar os pontos de vida de um NPC ("Dano/Cura"), desfazer a última ação e ler o registro do combate. O jogador recebe o registro só com o que ele vê (RN-20).

#### No app
- Num combate, a tela do jogador é o mapa primeiro, com o movimento embaixo ("Restam 9,0 m · 6 quadrados" e "Mover"), depois o cartão da vez com os próprios pontos de vida, classe de armadura e espaços de magia, depois as ações em abas; os pontos de vida das outras criaturas são palavras, nunca números (RN-20).
- O alvo de um ataque, de uma magia ou de "Agarrar"/"Empurrar" se toca nesse mapa: a página o traz para a vista com a borda de destaque, as criaturas que o servidor oferece têm um anel, as fora de alcance ou atrás de cobertura total ficam apagadas e dizem o motivo ao toque ("Goblin 3: cobertura total"), e uma barra acima da navegação (um cartão no trilho a partir de 1024 px) diz "Escolha o alvo · Raio de Fogo" com "Escolher na lista", "Cancelar" e "Confirmar". Mísseis Mágicos e Raio Ardente pedem um toque para cada dardo ou raio ("2 de 3 dardos", "Desfazer"). "Escolher na lista" abre a lista da própria folha, o caminho do teclado e do leitor de tela.
- Fora do combate a aba "Tokens" do mestre (embaixo do mapa) tem o interruptor "Os jogadores movem os próprios personagens", "Pôr o grupo no mapa" (uma janela, uma folha no celular: quem vem, "Em fila", "Em bloco" ou "Em arco", e o quadrado do mapa para o canto do grupo) e "Selecionar" (um toque numa peça a soma à seleção). Uma caixa na linha de cada personagem o escolhe: um escolhido é "Mover Tor" (o teclado de direções, "Quadrados por toque" de 1 a 20, "Tor vai 3 quadrados para a direita · Distância: 4,5 m" e o botão), vários são "3 selecionados" com "Mover os 3 juntos" e "Limpar seleção". O "Mover Brisa" do jogador (embaixo do mapa, no cartão num computador e numa folha no celular) é o mesmo painel só para o próprio personagem, com "Brisa andou 2 (3 m)." ou "O movimento parou antes." e o motivo quando o servidor o dá; num computador o jogador também pode arrastar o próprio token. Um toque num quadrado do mapa define o destino; o mapa desenha um anel em cada peça escolhida, uma linha tracejada até o destino e, depois de um movimento que parou antes, a linha andada, um X onde ia e "7,5 m · parou antes".
- Ver [Arquitetura](../../architecture/play/live-session.md) e [Combate](../../architecture/play/combat.md#combat).

#### Relacionadas
- RN-02: o mestre pode corrigir pontos de vida e espaços de magia à mão durante a sessão, e tem a palavra final (ver [Regras de negócio](regras.md)).

### MR-013: Ordem dos turnos

**Como** jogador, **quero** ver a ordem dos turnos, onde cada um está e quanto posso me mover, **para que** eu planeje minha ação.

- Prioridade: MVP
- Regras: RN-19, RN-20, RN-21
- Módulos: play, rules

#### Critérios de aceite
- **Dado** um combate com iniciativa definida, **quando** o jogador abre a tela do combate, **então** vê a ordem dos turnos, onde cada combatente visível está e quanto ainda pode se mover neste turno.
- **Dado** um combate com grade, **quando** abro a economia do turno ou o mapa, **então** o movimento também aparece em quadrados ("7,5 m · 5 quadrados"; 1 quadrado = 1,5 m = 5 ft).
- **Dado** um mapa sem grade, **quando** o mestre inicia um combate, **então** o app o leva a "Grade do mapa", onde ele define de 4 a 200 quadrados de largura; as linhas seguem a proporção da imagem. Um combate não começa sem grade.
- **Dado** que o mestre inicia um combate ("Iniciar combate"), **quando** ele preenche o nome, o mapa, o grupo e como cada jogador rola, e os NPCs com quantas cópias e "Escondido no início", **então** cada cópia de NPC rola a própria iniciativa (RN-19) e o jogador rola no app ou digita o d20 físico (RN-18).
- **Dado** monstros do bestiário, **quando** o mestre os adiciona no mesmo diálogo ("Adicionar criatura", busca, quantos de cada e como começam), **então** eles entram junto com o grupo. "Adicionar combatente" faz o mesmo durante um combate.
- **Dado** a iniciativa, **quando** o mestre olha para ela, **então** os totais mostram a soma (`1d20 (15) + 4 = 19`), um empate aparece uma vez e o mestre o ordena, "Esperando …" tem "Digitar pelo jogador", e "Começar o combate" fica bloqueado com o motivo.
- **Dado** um combate em andamento, **quando** o turno passa, **então** a rodada sobe depois do último combatente, e o movimento, a ação e a reação de cada um voltam no início do turno dele. O mestre tem "Próximo turno" e "Encerrar combate" (que pergunta antes), o jogador tem "Encerrar turno".
- **Dado** um combatente que o jogador não pode ver, **quando** a vez dele chega, **então** a barra diz "Vez do mestre", e nenhum combatente ou número de NPC escondido chega ao jogador (RN-20).
- **Dado** um jogador cujo personagem tem criaturas (um familiar, um animal que o mestre deu, criaturas invocadas), **quando** a iniciativa está aberta, **então** ele também vê "Iniciativa de <nome>" para cada criatura que ainda não rolou (MR-037).
- **Dado** a vez do jogador, **quando** ele abre "Mover", **então** vê o alcance ("Mover 3 m. 2 quadrados para a direita e 1 quadrado para baixo. Depois restam 4,5 m."), "Longe demais: faltam 1,5 m" ou "Ocupado" para um caminho recusado, e no desktop pode arrastar a peça dentro do alcance. O app não deixa o jogador passar do movimento do turno (RN-21); o mestre pode.
- **Dado** que um combate termina, **quando** ele termina, **então** ambos veem "Combate encerrado".
- **Dado** combatentes lado a lado na ordem com o mesmo total de iniciativa, jogadores incluídos, **quando** a ordem dos turnos é mostrada, **então** eles formam um turno conjunto, numa caixa com as palavras "Turno conjunto" e o total. O mestre vê todos os grupos; um jogador vê só um grupo que tenha um personagem de jogador (um grupo só de NPCs é do mestre, RN-20).
- **Dado** um turno conjunto, **quando** o turno chega ao grupo, **então** o turno de cada membro começa de uma vez, cada um com a própria ação, ação bônus, reação, movimento e teste contra a morte; o jogador de cada membro encerra a própria parte, o mestre encerra a de qualquer um, e o turno passa só quando o último encerra (não há "encerrar o turno do grupo" nem reabrir uma parte). Reordenar um empate, trazer reforços ou esconder um membro não muda um turno que já começou, e um reforço com o mesmo total age a partir do turno seguinte.
- **Dado** que o jogador encerra a própria parte, **quando** ele toca em "Encerrar a minha parte", **então** o app pergunta antes ("Encerrar a sua parte?", com o que ainda resta e "Voltar" primeiro), porque uma parte não reabre. Um toque duplo encerra uma parte só.
- **Dado** um turno conjunto com um membro escondido, **quando** o jogador lê o turno, **então** o membro escondido nunca é nomeado.
- **Dado** um turno conjunto em que um membro morreu, **quando** a rodada seguinte traz o turno do sobrevivente, **então** a Ação, a ação bônus, a reação e o movimento dele começam novos, e a linha "A vez passou para <nome>" do turno anterior some.

#### No app
- A vez do jogador: "Sua vez, Pensantus" com "Rodada 3 · depois de você: Toren" e o que a vez ainda tem em formas (um círculo para a Ação, um triângulo para o Bônus, um quadrado para a Reação, um caminhante para os metros que restam; cheia enquanto existe, tracejada depois de gasta). As ações são abas montadas com as opções do próprio personagem ("Ataques", "Magias" para quem conjura, uma aba com o nome da classe quando ela dá ações, "Outras" para as ações padrão); um grupo gasto é uma linha dobrada ("Ação bônus · Usada") que abre para o que ele tinha. "Mover" é a linha embaixo do mapa no celular e parte da barra da vez a partir de 1024 px, com "Encerrar turno" (uma barra acima da navegação do celular diz "Ainda tem" e as formas que restam). Fora da vez a tela diz de quem é a vez e, a partir de 1024 px, "Reação · Disponível".
- O combate do mestre é a barra do combate (rodada, de quem é a vez, "Em seguida", o que espera, "Próximo turno"), a ordem, o mapa e a coluna "Turno / Conversa" (com "Mais do combate", uma dobra por painel), em três colunas no computador, e as abas "Turno · Ordem · Mapa" com uma barra fixa no celular (ver [Design](../../design/combat.md#the-masters-combat)).
- Telas: o cartão do mestre tem um bloco por membro e "Encerrar a parte da Brisa"; o jogador lê "Vez de Brisa e Toren", "Vez dos Goblins", "Vez das Espadas voadoras" (o plural de um nome de duas palavras põe o substantivo e o adjetivo no plural), "Você encerrou a sua parte" e "O que a Brisa ainda tem".
- A tela acompanha a sessão ao vivo sem recarregar.
- Ver [Combate](../../architecture/play/combat.md#combat) e [Design](../../design/combat.md).

#### Relacionadas
- Cada NPC rola a própria iniciativa (RN-19); o jogador vê o estado do inimigo por uma palavra, nunca pontos de vida nem classe de armadura (RN-20); cada quadrado da grade tem 1,5 m, diagonais incluídas (RN-21). Ver [Regras de negócio](regras.md).
- A velocidade disponível vem do motor de regras. Ver [ADR-0008](../../adr/0008-rules-content-as-data-pure-engine.md).

### MR-014: Sua vez

**Como** jogador, no meu turno, **quero** ver minhas ações, ações bônus e ataques possíveis.

- Prioridade: MVP
- Regras: RN-02, RN-03, RN-18, RN-20, RN-22, RN-35
- Módulos: play, rules

#### Critérios de aceite

**Ações e magias**
- **Dado** um combate, **quando** a vez de Pensantus chega, **então** o jogador vê a ação, a ação bônus, a reação e o movimento disponíveis **e** as magias sem espaço de magia aparecem desativadas, cada opção com o motivo em palavras ("Ação já usada", "Sem espaço de 2º nível ou maior"), ainda no lugar e ao alcance do teclado. Fora da vez tudo fica desativado ("Não é a sua vez").
- **Dado** a tela "O que você pode fazer", **quando** o jogador a lê, **então** ela agrupa Ação (Ataques, Magias, Ações padrão), Ação bônus, Reação e Movimento, cada um com uma palavra de estado ("Disponível", "Usada").
- **Dado** um personagem com magias, **quando** ele abre a lista de magias na própria vez, **então** as que pode conjurar agora vêm primeiro, depois as outras; em cada grupo os truques vêm primeiro, depois por nível da magia, depois por nome em português. Escudo Arcano na vez do próprio personagem fica entre as outras com "Só fora da sua vez".
- **Dado** uma magia na lista, **quando** o jogador toca no "?", **então** vê a descrição completa, como no editor.
- **Dado** que Pensantus conjura Mísseis Mágicos com um espaço de 1º nível, **quando** a ação é confirmada, **então** o sistema marca o espaço como usado e a ação como gasta na hora. Uma nova tentativa não gasta de novo.
- **Dado** o último espaço de um nível, **quando** o jogador conjura com ele, **então** a ficha avisa "É o seu último espaço de 1º nível" antes do botão.
- **Dado** uma magia sem efeito conhecido, **quando** ela é conjurada, **então** o app diz "A magia foi conjurada: o mestre resolve o efeito."
- **Dado** um ataque, **quando** o jogador ataca, **então** escolhe um alvo (estado e distância mostrados; "Longe demais: alcance de 36 m" fica desativado), rola o d20 e depois o dano no app ou digitado (RN-18), e lê o resultado (`1d20 (13) + 6 = 19`, "Acertou", "Crítico" ou "Errou") e o alvo derrotado, nunca a classe de armadura.
- **Dado** habilidades de classe, **quando** o jogador toca em "Usar" (Retomar o Fôlego, Surto de Ação), **então** o uso é gasto; sem usos o app diz "Sem usos: volta num descanso curto". Ataque Extra mostra "1 ataque restante".
- **Dado** que a ação ou a ação bônus está livre, **quando** o jogador olha "Encerrar turno", **então** ele fica contornado e, com a ação livre, pergunta ali mesmo; vira o botão preenchido quando as duas estão gastas.

**Vantagem e desvantagem**
- **Dado** um ataque, um ataque mágico ou teste de resistência, uma armadilha, uma dica de quebra-cabeça, um teste de cena ou uma busca por armadilhas, **quando** o servidor encontra uma circunstância que dá vantagem ou desvantagem (uma condição de qualquer das criaturas, uma criatura que não se vê, alcance longo ou um hostil a 5 ft num ataque à distância, Ataque Descuidado, Sentido de Perigo, Tática de Matilha, Esquiva, uma vantagem racial num teste de resistência contra o tipo de fonte do teste: Bravura contra medo, Ancestral Feérico contra encanto, Esperteza Gnômica em testes de Inteligência, Sabedoria e Carisma contra magia, Resiliência Anã contra veneno), **então** a rolagem usa dois d20, o mais alto ou o mais baixo vale (eles se anulam quando as duas valem), a tela lista os motivos que o jogador pode ver **e** mostra os dois dados, o que vale marcado "vale" e o outro "descartado"; com dados reais o jogador digita os dois. Um acerto a 5 ft de um alvo paralisado ou inconsciente é um acerto crítico (SRD 5.1, "Advantage and Disadvantage", Conditions).
- **Dado** um personagem de jogador com 0 PV e morrendo, **quando** um ataque com arma ou um ataque mágico o tem por alvo, **então** o alvo conta como inconsciente: o ataque tem vantagem, um acerto a 5 ft é um acerto crítico, e um acerto soma uma falha no teste contra a morte, ou duas se for crítico (SRD 5.1, Conditions, Unconscious; Damage and Healing, Death Saving Throws).
- **Dado** o modo sugerido, **quando** o jogador assume desvantagem, **então** dá um motivo de 1 a 120 caracteres **e** a rolagem vai com ele; **quando** quer vantagem que o servidor não sugeriu, **então** pede ao mestre com um motivo ("Pedir ao mestre"), e o mestre aprova, recusa ou define outro modo na fila. O mestre pode definir qualquer modo em qualquer rolagem, sempre com um motivo; no ataque de um NPC o cartão do mestre tem a mesma escolha "Como rolar o d20" (Normal, Vantagem, Desvantagem, a sugerida marcada), e os campos do d20 seguem o modo escolhido. Cada mudança é uma linha no registro; o motivo é lido só pelo mestre e pelo jogador do atacante (RN-20).

**Dano**
- **Dado** um acerto de arma com um extra disponível (Ataque Furtivo, Punição Divina com o espaço que ela gasta, Marca do Caçador, Matador de Colosso), **quando** o jogador marca os extras e rola, **então** o dano é rolado por parte (a arma, cada extra, as linhas automáticas de Fúria e Combate com Armas Grandes) **e** um acerto crítico dobra os dados de cada parte e nunca os números fixos; um extra que não vale fica desativado com o motivo; com dados reais o jogador digita uma soma para cada parte. O mestre vê o dano por parte e tira um extra com um motivo, o que devolve o uso uma vez por turno dele.
- **Dado** Punição Divina contra qualquer alvo, **quando** o dano é rolado, **então** o dado extra para um morto-vivo ou ínfero é rolado e mostrado como uma linha igual às outras **e** o total que o jogador lê é o dano como rolado, de modo que nem as linhas nem o total dizem o que é o alvo; só o mestre lê que o dado não contou.
- **Dado** um alvo com resistência, vulnerabilidade ou imunidade (o fogo de um tiefling, o veneno de um anão, a linhagem de um draconato, o dano físico de um bárbaro em fúria, a ficha de uma criatura), **quando** o dano acerta, **então** cada tipo passa pelos modificadores (a resistência divide ao meio, arredondando para baixo; a mesma resistência de duas fontes conta uma vez) **e** o mestre e o jogador do alvo leem cada passo, enquanto para um NPC os jogadores leem só o dano como rolado (RN-20); o mestre pode deixar uma fonte de fora quando aplica.

**Bárbaro**
- **Dado** um bárbaro que entra em fúria, **quando** a fúria está ativa, **então** "Em fúria" aparece ao lado do nome, as magias são recusadas, e quando o turno acaba sem ataque a uma criatura hostil e sem dano sofrido o jogador é perguntado "A sua fúria vai acabar?" ("Voltar e atacar" ou "Deixar a fúria acabar") antes de o turno passar; Marcado, Esquivando e Ataque descuidado são estados no mesmo lugar, cada um com o seu fim (SRD 5.1, Barbarian, Rage).
- **Dado** um Berserker de nível 3 ou mais que toca em "Fúria", **quando** a tela pergunta "Entrar em frenesi?", **então** "Fúria com frenesi" começa a fúria com "Em frenesi" nela, um ataque corpo a corpo com arma é uma ação bônus em cada turno seguinte do bárbaro, e o bárbaro ganha um nível de exaustão quando a fúria acaba ("Só fúria" entra em fúria como sempre, sem exaustão); qualquer outro bárbaro não é perguntado (SRD 5.1, Barbarian, Path of the Berserker, Frenzy).

**Sortudo do halfling (RN-35)**
- **Dado** um halfling cujo d20 de ataque (um ataque com arma ou um truque de ataque) mostra um 1, digitado ou rolado no app, **quando** a rolagem volta, **então** a ficha pergunta "Tirou 1: rolar de novo?" antes de qualquer resultado: "Rolar de novo" rola esse d20 outra vez (no app ou digitado, RN-18) e a nova rolagem vale, mesmo que seja outro 1; "Ficar com o 1" mantém o 1 natural, um erro; com vantagem ou desvantagem a pergunta vem para o dado que mostra o 1, mesmo o que não contaria, e só esse dado é rolado de novo; um dado de Inspiração de Bardo que o halfling tem é perguntado depois dela, na nova rolagem; o resultado e o registro dizem "Sortudo: 1 → 14" (SRD 5.1, Halfling, Lucky).
- **Dado** um halfling cujo jogador rola um d20 que mostra um 1 num teste de cena, num teste em grupo, numa busca por armadilhas (fora de um combate, ou a ação Procurar de um combate), numa tentativa de dica de quebra-cabeça, na jogada de qualquer lado de uma disputa, numa iniciativa antes de o combate começar, num teste contra a morte, no teste de Furtividade de um Esconder-se ou no teste de habilidade de uma Contramágica acima do nível do espaço, ou, num combate, num ataque mágico contra um alvo, num teste de resistência de um efeito ou num teste de concentração, **quando** a rolagem volta, **então** a ficha pergunta "Tirou 1: rolar de novo?" antes de qualquer resultado e nada é escrito, gasto ou fechado (a janela do teste de resistência continua aberta); a resposta funciona como num ataque (o novo d20 no app ou digitado, ou "Ficar com o 1"; um dado de um par; o dado de Inspiração de Bardo depois dela, na nova rolagem) **e** Talento Confiável conta o novo d20, não o 1; um recarregamento pergunta de novo; o personagem não rola mais nada que possa perguntar até ser respondido; uma rolagem que o mestre faz pelo halfling ali (um teste de resistência ou ataque mágico de um personagem de jogador, um teste em grupo rolado pelo jogador) pergunta também: o mestre é perguntado na própria ficha, o jogador lê a mesma pergunta e vale a primeira resposta ("Perguntas pendentes" abaixo).
- **Dado** o halfling de um jogador cujo teste de resistência o app rola dentro da ação de outra pessoa (o teste de uma magia, o de uma armadilha, o teste de Destreza contra Repreensão Infernal), **quando** o d20 mostra um 1, **então** ninguém é perguntado: o app rola esse dado de novo sozinho e a nova rolagem vale, mesmo que seja outro 1 (com vantagem ou desvantagem, só o primeiro dado que mostra um 1); o resultado e o registro dizem "Sortudo: 1 → 14" a quem vê os dados desse teste; um NPC com a ficha de um halfling mantém o 1.

**Perguntas pendentes (RN-35)**
- **Dado** uma jogada que espera uma pergunta ("Tirou 1: rolar de novo?" do Sortudo ou o dado de Inspiração de Bardo), **quando** o mestre responde por o jogador, **então** a jogada é resolvida como se o jogador tivesse respondido (o d20 já rolado, o dado novo no app ou digitado, o personagem do próprio jogador), o jogador lê o resultado no combate ou na cena e o dado é gasto uma vez só; **quando** o mestre tenta em menos de 10 segundos depois que a pergunta foi feita, **então** o servidor recusa com os segundos que faltam ("Ainda é a vez de Brisa responder. Você pode responder em 3 s.") para que um jogador presente tenha a primeira chance, e a resposta do próprio jogador nunca espera; **quando** o jogador e o mestre respondem ao mesmo tempo depois disso, **então** vale a primeira resposta e a outra recebe que a jogada já foi respondida.
- **Dado** jogadas que esperam, **quando** o mestre lê as perguntas pendentes, **então** lê todas as da mesa (as de um jogador, num combate ou fora dele, e as de um NPC), cada uma com o d20, o bônus e quem rolou; **e** o jogador lê só as suas; **e** outro jogador recebe que uma jogada que não pode responder não existe.

**Avisos de turno**
- **Dado** um combate na tela do jogador, **quando** o turno passa a um combatente que ele joga (o personagem ou uma de suas criaturas), **então** ele é avisado uma vez: o celular vibra onde pode, um toque curto soa (depois de qualquer primeiro toque na página), o painel do turno treme por meio segundo (um quadro parado com movimento reduzido) **e** o título de uma aba escondida diz "● Sua vez"; nada dispara ao carregar a página, na vez do mestre ou de um NPC, nem duas vezes no mesmo turno, e "Avisar na minha vez (som e vibração)" desliga o som e a vibração até a página recarregar.
- **Dado** uma sessão aberta, **quando** o mestre toca em "Chamar a atenção" ao lado do personagem de um jogador (na ordem do combate ou no painel do grupo), **então** só esse jogador recebe o mesmo aviso (vibração onde pode, o toque, um tremor de meio segundo, "● O mestre chama você" no título de uma aba escondida, nunca "Sua vez") e lê "O mestre está chamando você.", num combate ou fora de um e mesmo quando não é a vez dele; o mestre lê "Chamado." e o botão fica desligado por 10 segundos (o servidor também recusa uma segunda chamada ao mesmo personagem dentro de 10 segundos); um jogador não pode chamar, e nada é mostrado aos outros jogadores.

**Magias de área**
- **Dado** uma magia de área (Bola de Fogo, Mãos Flamejantes, Relâmpago, Onda Trovejante) num combate com mapa, **quando** o jogador a conjura, **então** ele a posiciona no mapa (um ponto para uma esfera, uma direção para um cone, uma linha ou um cubo) **e** o servidor diz quem está dentro, aliados e o conjurador incluídos, com a cobertura que cada um tem **a partir do ponto de origem**, que conta só para um teste de resistência de Destreza (RN-21).
- **Dado** que ninguém que o conjurador vê está dentro da área, **quando** ele confirma, **então** o app diz que a magia ainda gasta o espaço e a ação e pergunta de novo ("Conjurar mesmo assim" ou "Mudar o local"). Sem mapa a lista fica e o app só acrescenta a confirmação "Ninguém está marcado".
- **Dado** uma criatura escondida na área, **quando** a magia é conjurada, **então** ela é atingida como qualquer outra, e o que isso faz com o esconderijo dela é a regra da mesa "Criaturas escondidas atingidas por uma área" (RN-10, RN-24). Com "Perguntar a cada vez", toda magia de área que um jogador conjura segura o turno até o mestre responder ("Esperando o mestre"), haja ou não uma criatura escondida na área, de modo que a espera não revela nada; o mestre vê a área da última magia no mapa dele e responde com "Revelar", "Manter escondidas" ou, sem nenhuma atingida, "Sem escondidas".

**Magias que leem pontos de vida, e trazer os mortos de volta**
- **Dado** uma magia que lê pontos de vida (Sono, Leque Cromático, Palavra de Poder Atordoar e Matar, Estabilizar, Cura Completa), **quando** ela é conjurada, **então** o servidor a resolve com os pontos de vida reais dos alvos, com o total dos dados rolado no app ou digitado **e** o jogador continua vendo só "Ileso", "Ferido" ou "Muito ferido" (RN-20): o conjurador vê a própria rolagem e quem foi afetado ("O Goblin 1 adormeceu. O Capitão Goblin não foi afetado."), nunca os pontos de vida de um alvo. O mestre vê o total, os pontos de vida de cada criatura e a ordem, e pode mudar as condições.
- **Dado** uma criatura que morreu no último minuto (10 rodadas num combate) e está ao alcance do toque, **quando** um clérigo ou paladino conjura Revivificar nela com um espaço de 3º nível e marca que tem os diamantes, **então** a criatura volta com 1 PV **e** o espaço, a ação e os diamantes são gastos **e** a lista do jogador só mostra criaturas que a magia alcança agora, sem motivo para as outras (o mestre lê os motivos e tem a chave "Revivificar não funciona nesta morte"). Fora de combate o mestre confirma "Faz menos de 1 minuto" antes de qualquer coisa ser gasta.
- **Dado** um personagem morto, **quando** o mestre do jogo toca em "Reviver", **então** ele volta com 1 PV, sem estar inconsciente e com os testes contra a morte zerados, no combate mantém o lugar na ordem e age no próximo turno dele **e** o app recusa, nomeando o outro personagem, enquanto o jogador tem outro vivo (RN-03).

**Reações, condições e morte**
- **Dado** uma reação que um jogador pode tomar (Escudo Arcano, Esquiva Sobrenatural, Repreensão Infernal, Contramágica, Palavras Cortantes, Desviar Projéteis, Queda Suave, o ataque de oportunidade) ou um teste de concentração, **quando** a janela abre, **então** o turno espera e o jogador responde num diálogo ("Você foi atingido: usar Escudo Arcano?", "Concentração em risco…") com "Deixar passar" e o botão de uso; Escape passa e nunca decide um teste de concentração. Uma reação respondida no assento do próprio jogador entra no registro como do jogador, mesmo com "Usar pelo jogador" e "Deixar passar pelo jogador" à mostra; "(respondido pelo mestre)" aparece só quando o mestre respondeu. O jogador da reação de um NPC lê "Esperando o mestre" (RN-31). A fila do mestre fica sob o cartão do NPC, e "Próximo turno" fica bloqueado com o motivo escrito. O teste de concentração diz o modo dele em palavras e, com dados físicos, pede os dois d20 quando o modo tem dois. O ataque de oportunidade é a ação de texto sob "Sua reação" e gasta a reação.
- **Dado** disputas (agarrar, empurrar, escapar, Esconder-se, Ajudar, testes em grupo), **quando** o jogador as usa, **então** elas seguem o SRD: agarrar e empurrar substituem um ataque e são decididos por Força (Atletismo) contra Força (Atletismo) ou Destreza (Acrobacia); escapar é uma ação contra quem agarra ou uma CD fixa; quem agarra arrasta à metade da velocidade; Esconder-se compara o total de Furtividade com a Percepção passiva de cada criatura (um empate mantém o escondido notado); Ajudar dá vantagem a um teste ou a um ataque; um teste em grupo passa quando pelo menos metade dos pedidos passa. Uma disputa espera numa janela de reação ("Esperando o mestre" ou "Esperando <nome>"). Um jogador nunca lê os números de um NPC, a CD de escapar nem quem notou um escondido.
- **Dado** que o mestre pede a um, alguns ou todos os personagens um teste de perícia, um teste de habilidade ou um teste de resistência fora de um combate, **quando** ele o envia, **então** cada rolagem é julgada sozinha, a menos que ele escolha um teste em grupo, só os personagens pedidos leem o pedido, e o mestre marca quem está surpreso.
- **Dado** alguém com 0 PV, **quando** o jogador olha o turno, **então** vê "Brisa está caída", as marcas e "Rolar teste contra a morte"; na terceira falha o personagem morre só quando o mestre confirma ("Confirmar a morte" / "Ainda não") (RN-03).
- **Dado** condições (as 15 do SRD; "Derrubado" é Prone) e concentração, **quando** elas se aplicam, **então** aparecem como etiquetas sob o nome, na faixa do jogador e como um ponto na peça, e o jogador encerra a própria concentração. O mestre as define em "Condições…" no ⋮ da ordem. Quando o dano atinge um alvo que se concentra, o mestre que aplica **outro valor** ("Aplicar outro valor") lê "Teste de Constituição, CD 10". Condições e concentração são marcadas e lembradas; o mestre decide os efeitos (RN-22).
- **Dado** o personagem de um jogador com 0 PV, ainda fazendo testes contra a morte, no turno dele com o teste ainda não rolado, **quando** o jogador está ausente e o mestre do jogo lê "Teste contra a morte de Brisa", **então** ele o rola ("Rolar no app" ou "Digitar o resultado", pelo modo de dados da mesa) e as contagens andam, **e** o turno não passa antes de ele ser rolado ("Próximo turno" fica tracejado e a barra diz "Falta o teste contra a morte"; o servidor também recusa, MR-053); o cartão nunca aparece para um NPC nem para um personagem estável, e os outros jogadores nunca leem o resultado (RN-03, RN-24).

**Turno do mestre e o registro**
- **Dado** a vez de um NPC, **quando** o mestre a joga, **então** o cartão "Ações do Capitão Goblin" mostra pontos de vida, classe de armadura, velocidade e o ataque com o campo "Alvo", que começa na criatura hostil mais próxima ao alcance (um personagem de jogador, para um NPC; a primeira da ordem entre iguais), depois na hostil mais próxima fora do alcance, e nunca num aliado, a menos que não exista outra criatura; acompanha o NPC quando ele se move, até o mestre escolher um alvo à mão, e mantém a lista na ordem do servidor; "Rolar ataque" funciona no app ou digitado ("Acertou contra CA 18 do Toren"), e "Aplicar 5 de dano" / "Não aplicar" (que pergunta "Descartar o dano de 5?"). Enquanto há dano sem aplicar, "Próximo turno" pergunta "Há dano sem aplicar. Passar o turno mesmo assim?". A classe de armadura é só do mestre.
- **Dado** a vez de um personagem de jogador que ainda não agiu (sem Ação, sem ação bônus, sem movimento gasto), **quando** o mestre toca em "Próximo turno", **então** o app pergunta no lugar "Passar a vez de Toren, que ainda não agiu?" com "Cancelar" e "Passar a vez" (um turno passado não volta); a vez de um NPC, de um personagem que agiu e de um personagem a 0 pontos de vida não perguntam: o teste contra a morte é a vez inteira dele, então depois de feito "Próximo turno" passa na hora.
- **Dado** o registro do combate, **quando** qualquer pessoa o lê, **então** ele mostra as rodadas, as mais recentes primeiro, como cada público pode lê-las, com o cabeçalho "Em andamento desde 20:05". O mestre desfaz a última ação pelo nome ("Desfazer o ataque do Capitão Goblin ao Toren (5 de dano)?").

**Descansos e recursos de classe**
- **Dado** que o mestre faz um descanso curto ou longo, **quando** ele confirma a lista do que volta, **então** os recursos recarregam como o SRD diz; um jogador gasta dados de vida por tamanho de dado num descanso curto (com um bardo no grupo, o primeiro dado de cada descanso curto também traz o dado de Canção de Descanso).
- **Dado** Cura pelas Mãos, Conjuração Flexível, Metamagia e Inspiração de Bardo, **quando** o jogador os usa, **então** cada um tem a própria ação. O dado de Inspiração de Bardo é usado depois do d20 e antes do resultado, num teste de habilidade, num ataque com arma ou mágico contra um alvo, ou num teste de resistência que o jogador rola (incluindo a busca por armadilhas, a tentativa de dica de quebra-cabeça, o teste de resistência de um efeito e o teste de concentração); testes de resistência que o app rola por um alvo não perguntam. Cura pelas Mãos num morto-vivo ou constructo não faz nada e nunca diz o motivo.

#### No app
- O servidor calcula a economia, as opções e o motivo de cada opção desativada; o app só as desenha. Ver [Combate](../../architecture/play/combat.md#combat), [Regras e combate](../../architecture/rules-combat.md#spell-details), [Ataques e dano](../../architecture/play/attacks-and-damage.md), [Conjuração](../../architecture/play/casting.md), [Reações](../../architecture/play/reactions.md) e [Design](../../design/combat.md).
- No celular as folhas de ataque e de conjuração são painéis inferiores; no desktop, diálogos. A partir de 1024 px o turno é uma faixa compacta com "Encerrar turno" à direita.
- "Atacar" e "Conjurar" num combate com mapa pedem o alvo no mapa primeiro (os mesmos alvos, alcance, cobertura e desvantagem da lista da folha: "Longe: desvantagem" é dito no token, não apagado); "Confirmar" abre a folha na rolagem com o alvo nomeado ("Adaga em Goblin 1"), e "Escolher na lista" a abre na lista de alvos. Uma magia de área, sobre o conjurador ou sem alvo mantém a sua folha; as folhas do mestre e o ataque de oportunidade também.

#### Relacionadas
- RN-02: o mestre pode corrigir pontos de vida e espaços de magia à mão (ver [Regras de negócio](regras.md)).
- Dados (RN-18): o mestre escolhe como a campanha rola (cada jogador escolhe, todos no app, ou todos com os próprios dados) em "Regras da mesa" e vê o resultado em "Dados", na aba "Regras"; cada jogador mantém a própria preferência, na página da campanha. Com um dado físico o jogador digita a soma dos dados e o app soma o modificador. As rolagens de combate seguem essa regra.
- O jogador vê se acertou ou errou e o dano, não a classe de armadura nem a rolagem do NPC (RN-20).
- Quais ações, reações e recursos o sistema conhece vêm do motor de regras (regras como dados). Ver [ADR-0008](../../adr/0008-rules-content-as-data-pure-engine.md).

### MR-015: Ações da cena de RP

**Como** jogador, **quero** ver, numa lista simples, as ações que o mestre escolheu para a cena, **para que** eu saiba o que posso rolar e usar fora do combate.

- Prioridade: MVP
- Regras: RN-10, RN-18, RN-20
- Módulos: maps, play, rules

#### Critérios de aceite
- **Dado** uma cena de RP com as ações que o mestre escolheu para ela, **quando** o jogador abre a cena, **então** vê a lista de ações que o mestre escolheu, cada rolagem com o bônus do próprio personagem já calculado (por exemplo Investigação) **e** as habilidades que só funcionam em combate não aparecem. O jogador também lê o nome e a descrição do ponto e a Percepção, a Investigação e a Intuição passivas dele.
- **Dado** um combate (MR-013, MR-014), **quando** o jogador vê as ações possíveis, **então** o sistema decide essa lista, pelas regras de D&D, nunca o mestre. A cena de RP é o único lugar em que o mestre escolhe a lista.
- **Dado** que o mestre escolhe as ações no ponto de cena do mapa, **quando** ele acrescenta uma, **então** ela é uma perícia, um teste de habilidade ou um teste de resistência, com um nome opcional (até 60 caracteres) e uma CD opcional (1 a 30); uma cena guarda no máximo 20 ações e nada só de combate. A lista pode ser reordenada, editada e removida.
- **Dado** um ponto de cena, **quando** o mestre abre a cena na sessão, **então** qualquer ponto de cena abre, mesmo um sem ações ou um escondido (ele continua escondido no mapa), e ele a fecha sem ser perguntado. Todos veem a cena abrir ou fechar ao vivo.
- **Dado** que um jogador rola uma ação, **quando** ele rola no app ou digita o dado físico (RN-18), **então** o bônus é somado, até o limite de tentativas da ação. O mestre vê todas as rolagens, com o total e "passou" quando há CD. O jogador vê só as próprias, com "passou" só quando a cena mostra a CD.
- **Dado** uma cena, **quando** o mestre liga "Mostrar a CD aos jogadores" (por cena; desligado por padrão), **então** o jogador vê a CD das ações que têm uma e se as próprias rolagens passaram; desligado, vê só o total (RN-20). Uma ação sem CD não mostra nada a mais, e o mestre sempre vê a CD.
- **Dado** uma ação de cena, **quando** o mestre define "Tentativas por jogador" (1 a 5, 1 por padrão, ou "Sem limite") e pode dar a um jogador mais uma tentativa ("Dar mais uma tentativa"), **então** o jogador rola até o limite dele e vê quantas tentativas restam ("Restam 2 de 3 tentativas", "Sem mais tentativas"). Fechar e reabrir a cena zera as contagens; baixar o limite abaixo do que alguém já gastou o deixa sem tentativas, sem apagar nada.
- **Dado** o "Dar mais uma tentativa" do mestre, **quando** ele a oferece, **então** o botão aparece só na última rolagem do personagem naquela ação, pergunta ali mesmo com "Voltar" primeiro e, com a CD mostrada, só é oferecido se a rolagem falhou (ou quando a ação não tem CD). A tela do jogador anuncia.
- **Dado** um ponto de cena de RP, **quando** o mestre anexa a ele imagens da galeria (até 8, na ordem que escolher), **então** só o mestre vê essa lista (RN-10), **e** na sessão ao vivo a cena aberta mostra as imagens com "Mostrar aos jogadores", a ação própria da galeria (MR-019), **e** os jogadores só veem uma imagem depois que o mestre a mostra. Apagar a imagem da galeria ou o ponto remove o vínculo.
- **Dado** um combate na tela, **quando** uma cena está aberta, **então** os blocos da cena não são desenhados para ninguém; a cena volta à tela quando o combate termina.
- **Dado** o mestre que abre uma cena, **quando** ele escolhe os jogadores que estão nela ("Quem está na cena"), **então** só esses jogadores, e o mestre, recebem a cena: o nome, o ponto, a descrição, as ações, as rolagens, o palco, o teste em grupo pedido nela e todo aviso ao vivo sobre ela (RN-10). Sem ninguém escolhido a cena fica aberta para a mesa toda, **e** um jogador que entra na campanha depois está nela; com uma lista, a cena fica aberta para exatamente esses jogadores, e um jogador que entra depois está fora.
- **Dado** uma cena aberta para só alguns jogadores, **quando** um jogador que está fora lê a cena, rola uma ação dela ou pede o palco, **então** recebe o que recebe quando nenhuma cena está aberta: nenhuma cena, a mesma recusa de "nenhuma cena está aberta", nenhum retrato (404), e nenhum aviso ao vivo sobre ela, então nada diz a ele que há uma cena aberta.
- **Dado** a cena aberta, **quando** o mestre muda quem está nela ("Quem está na cena"), **então** um jogador que entra vê a cena na hora (e as rolagens que já tinha feito nela, se já estivera), **e** um jogador que sai a perde na hora, sem cena na tela dele; as rolagens já feitas ficam com a abertura. Os jogadores que ficam, e os que entraram ou saíram, ficam sabendo da mudança; mais ninguém.
- **Dado** a cena aberta, **quando** o mestre a lê, **então** ela diz desde quando está aberta e quem está dentro e quem está fora, com os nomes dos personagens ("Aberta desde 20:20 para Pensantus, Brisa"; "Fora da cena: Sálvia e Jabuti"); nenhum jogador lê essa lista.
- **Dado** uma cena aberta para só alguns jogadores, **quando** o mestre pede um teste em grupo ("Teste em grupo") sem nomear personagens, **então** ele pede só aos jogadores da cena, **e** nomear um personagem que está fora é recusado; um jogador tirado da cena deixa de ler o teste e não pode respondê-lo. O mestre dá uma tentativa só a um personagem que está dentro.
- **Dado** uma cena aberta para só alguns jogadores, **quando** a sessão termina, **então** os testes passados nela contam no resumo para o mestre e para os jogadores que estavam dentro, e para nenhum outro jogador.
- **Dado** uma rolagem que um jogador fez numa ação que o mestre escondeu depois, **quando** a sessão termina, **então** a rolagem continua no resumo dos jogadores que a viram ou a rolaram, porque aconteceu: o resumo conta tentativas e acertos por personagem, nunca a ação, então não diz nada dela; a lista de rolagens da cena deixa de mostrar a rolagem aos jogadores (RN-10).
- **Dado** uma cena aberta para só alguns jogadores, **quando** o mestre liga "Os jogadores podem entrar" (desligado por padrão, mutável com a cena aberta, volta a desligado quando a cena fecha), **então** um jogador que está fora lê exatamente duas coisas dela: o nome e que pode entrar, e nada mais (nem ponto, descrição, palco, ações, rolagens, imagens ou pistas); com o interruptor desligado não lê nada, como quando nenhuma cena está aberta. Todos os jogadores são avisados quando o interruptor muda.
- **Dado** uma cena que deixa os jogadores entrarem, **quando** um jogador toca "Entrar na cena", **então** ele vira um dos jogadores dela na hora, lê como qualquer membro e o mestre o vê marcado como quem entrou sozinho, contra os que ele escolheu; entrar de novo não muda nada. O mestre, um estranho e um membro pendente não podem entrar, e com o interruptor desligado a recusa é a de "nenhuma cena está aberta".
- **Dado** um jogador que entrou numa cena, **então** ele é um dos jogadores dela para o que vem depois: uma imagem mostrada aos "jogadores da cena" chega a ele na hora, e uma imagem mostrada a jogadores escolhidos não chega, a menos que ele tenha sido escolhido.
- **Dado** uma cena aberta para alguns jogadores, **quando** o mestre a abre para todos, **então** ela fica "descoberta" para o grupo e qualquer jogador pode rotular uma anotação com o nome dela; enquanto está aberta para alguns, só os jogadores dela a descobrem, cada um por si: podem rotular uma anotação com ela (também quando o mestre os põe dentro ou eles entram, e depois de saírem, pois o que leram, leram), e a lista de cenas ou de anotações de nenhum outro jogador conta o nome dela nem a aceita como rótulo (a recusa é a de uma cena que não existe).
- **Dado** uma cena aberta para só alguns jogadores, **quando** o mestre diz quem está dentro, **então** ele pode nomear qualquer jogador da campanha, também o jogador de um personagem morto: a audiência da cena é sobre jogadores, e esse jogador acompanha a cena como qualquer um dentro dela. Rolar uma ação continua pedindo um personagem vivo.
- **Dado** uma cena aberta para só alguns jogadores, **quando** o mestre revela uma pista a um personagem cujo jogador está fora, **então** é permitido (o mestre escolheu o personagem): a pista está nas anotações desse jogador e nada mais da cena chega a ele: nem cena, palco, ações, imagens ou nome da cena, e a pista não leva rótulo de cena para ele.
- **Dado** uma ação de cena, **quando** o mestre a marca "Escondida" (ao acrescentá-la, ou depois, também com a cena aberta), **então** nenhum jogador a recebe: nem o nome, a CD, as tentativas ou as rolagens, no mapa, na cena aberta, num aviso ao vivo ou no histórico que ele lê (RN-10); o mestre continua a vê-la, marcada, rola nela pela mesa e dá tentativas nela.
- **Dado** uma ação escondida, **quando** um jogador tenta rolá-la mesmo assim, **então** a recusa é exatamente a de uma ação que não existe (o mesmo erro, a mesma mensagem), para nunca dizer ao jogador que a ação está lá.
- **Dado** uma ação escondida na cena aberta, **quando** o mestre a mostra ("Os jogadores veem"), **então** os jogadores da cena a veem na hora e podem rolá-la; **quando** o mestre a esconde de novo, **então** ela sai da tela deles na hora, com as rolagens que fizeram nela. Nada é apagado: mostrá-la outra vez devolve as rolagens e as tentativas.
- **Dado** que o mestre edita, move ou remove uma ação escondida (e que continua escondida), **quando** os jogadores estão assistindo, **então** só a tela do mestre fica sabendo.
- **Dado** o teste em grupo ("Teste em grupo"), **quando** o mestre o pede, **então** ele pede à mesa uma perícia, nunca uma ação de cena, e por isso uma ação escondida não é oferecida nem nomeada por ele.

#### No app
- Editor: "Ações da cena" e "Imagens da cena" do painel do ponto (miniaturas em ordem com ↑ ↓ e "Tirar da cena", que mantém a imagem na galeria; "Escolher da galeria"; "Gerar imagem com IA", cuja imagem entra na lista; "N de 8").
- Sessão: o mestre abre a "Cena de RP" na aba "Cena" ("Abrir cena", "Trocar cena", "Fechar cena"; abrir uma cena leva para a aba). A cena aberta é um cartão só: "Em cena" (uma linha por NPC), "Imagens da cena" (miniaturas; um toque mostra uma, a imagem do mapa inteiro pergunta antes; a que está à mostra leva "À mostra"; o bloco tracejado "Da galeria" e "Parar de mostrar"), "Ações e rolagens" (uma linha por ação, cada rolagem num chip que abre a fórmula e o Passou / Não passou; "Teste em grupo" no cabeçalho), "Pistas" e os "Ganchos e anotações", dobrados. No celular as partes dobram. O jogador tem o bloco "Cena" com o bônus dele e "Rolar".
- Quem vê o quê (RN-10): a primeira linha da cena aberta diz "Aberta desde 20:20 para" com os jogadores que estão nela em chips (uma contagem e as iniciais sobrepostas a partir de quatro) e "Quem está na cena", que abre o seletor de jogadores (uma folha no celular); embaixo, quem está fora ("Toren não está nesta cena.") e, numa cena aberta só para alguns jogadores, o interruptor "Os jogadores podem entrar"; quando um jogador entra por conta própria a linha diz "Toren entrou na cena às 20:41.". Cada ação tem um olho ("Escondida": os jogadores nunca a recebem nem as rolagens dela).
- Tela do jogador, fora de uma cena aberta para só alguns jogadores: com o interruptor do mestre desligado nada da cena é desenhado (a página fica como sem cena aberta: o mapa e as imagens deixadas); ligado, um cartão compacto, "Há uma cena aberta", o nome da cena, "Você ainda não está nela. Entre para ver e participar." e um "Entrar na cena" preenchido. Entrar mostra a cena como a de qualquer jogador, sob "Você entrou na cena", com o título em foco; se o mestre desligou o interruptor nesse meio tempo o cartão some e a página diz "A cena não está mais aberta para entrar.". Veja [Design](../../design/session.md#the-players-scene-and-the-ficha-panel).
- Ver [Cenas e palco](../../architecture/play/scenes-and-stage.md).

#### Relacionadas
- Numa cena de RP o mestre escolhe as ações possíveis e o jogador vê o que pode fazer com o próprio bônus; em combate o sistema decide. Ver [Regras de negócio](regras.md) e [ADR-0008](../../adr/0008-rules-content-as-data-pure-engine.md).
- As cenas têm testes de perícia, testes de habilidade e testes de resistência; magias e habilidades não fazem parte delas.

#### Dúvidas
- Um pacote exportado com uma ação escondida a traz de volta escondida. É isso que o mestre quer quando compartilha uma campanha com outro mestre?

### MR-016: Dar XP

**Como** mestre, **quero** dar XP ao grupo por inimigos derrotados, por ouro ou por marcos, conforme a campanha, ou quando eu quiser.

- Prioridade: MVP
- Regras: RN-09, RN-12
- Módulos: progression

#### Critérios de aceite
- **Dado** uma campanha no modo por inimigos e dois goblins derrotados (50 XP cada), **quando** o encontro termina, **então** os 100 XP são divididos entre os quatro personagens do grupo, 25 para cada. Um resto é arredondado para baixo, perdido e informado ("2 XP se perdem na divisão").
- **Dado** uma campanha por inimigos, **quando** o combate termina, **então** o mestre usa o nível de desafio (ND) do NPC ou digita o XP (os dois jeitos contam igual) **e** escolhe quem recebe a divisão. Só personagens de jogador vivos recebem XP.
- **Dado** o fim de um combate, **quando** o resumo mostra "Experiência do combate", **então** "Dar 116 XP a cada um" o dá, e "Agora não" deixa a linha "XP do combate ainda não dado". O XP de um encontro não pode ser dado duas vezes.
- **Dado** o "Ao ser derrotado" do editor de NPC, **quando** o mestre define o ND, **então** ele preenche o XP, e "Usar 50 XP" volta ao valor da tabela. Um jogador nunca lê o XP de um NPC (RN-20).
- **Dado** uma campanha no modo por marcos, **quando** o mestre registra um marco, **então** os personagens que o mestre escolhe (todos os personagens vivos do grupo vêm pré-marcados) ganham "Pode subir de nível" **e** nenhum XP é contado. A marca some quando o nível sobe.
- **Dado** os modos por inimigos ou por ouro, **quando** o mestre dá XP ao grupo por conta própria ("Dar XP"), **então** o XP vai para as fichas **e** o histórico da campanha mostra quem deu, quando e por quê.
- **Dado** o modo por ouro, **quando** o mestre quer dar XP, **então** ele digita as peças de ouro (1 XP por 1 PO, RN-09); com os tesouros do mapa, "Voltar à cidade" converte em XP o que o grupo encontrou ([MR-041](#mr-041-tesouros-e-xp-por-ouro)), e digitar os PO continua valendo.
- **Dado** uma campanha por marcos com os marcos planejados de antemão, **quando** o mestre os escreve (até 120 caracteres cada, no máximo 100, em qualquer ordem: subir, descer, editar ali mesmo, remover perguntando ali mesmo), **então** só o mestre vê a lista.
  - Um marco alcançado uma vez e depois desfeito não pode ser removido, porque as concessões dele ficam no histórico; o app diz isso e deixa de oferecer "Remover".
  - **Quando** o grupo alcança um e o mestre o marca e escolhe quem sobe de nível, **então** os escolhidos ganham "Pode subir de nível"; o marco passa para "Marcos alcançados" com o dia, a hora e quem; a ordem não é obrigatória.
  - "Dar a mais alguém" dá o mesmo marco a um personagem que ficou de fora, como uma concessão nova no histórico.
  - "Registrar um marco fora da lista" registra um marco que não estava planejado.
- **Dado** a última concessão, **quando** o mestre toca em "Desfazer", **então** o app pergunta ali mesmo e desfaz só a última concessão; um marco planejado desfeito volta a planejado (se era a única marca) ou continua alcançado para os outros (se era um "Dar a mais alguém").
- **Dado** um jogador, **quando** ele olha os marcos, **então** vê só os marcos alcançados e o próprio personagem (a linha aparece só depois do primeiro marco, e o estado vazio nunca sugere que existam marcos planejados). Um jogador não pode escrever.
- **Dado** a página da campanha, **quando** qualquer pessoa abre "Experiência", **então** vê o histórico de concessões. A ficha mostra o XP como um bloco só de leitura e "Pode subir de nível", também na lista do grupo.

#### No app
- A página da campanha do jogador atualiza quando abre ou quando a aba volta ao primeiro plano; não acompanha a sessão ao vivo. A página do mestre acompanha enquanto uma sessão está aberta.
- Ver [Arquitetura](../../architecture/progression.md#progression-module-xp-and-milestones).

#### Relacionadas
- Como o XP é dado: o ND ou o XP digitado, o mestre decide quem recebe, o arredondamento para baixo, o aviso de subida de nível para mestre e jogador, e o histórico para todos. Ver [RN-09](regras.md) e [RN-12](regras.md).
- O que cada personagem ganha ao subir de nível vem do motor de regras. Ver [ADR-0008](../../adr/0008-rules-content-as-data-pure-engine.md).

### MR-018: Documento de campanha

**Como** mestre do jogo, **quero** um documento da campanha com texto, imagens e links para mapas e fichas que abrem num diálogo, **para que** eu guarde as anotações da campanha num só lugar.

- Prioridade: MVP
- Regras: —
- Módulos: campaigns (o documento); maps e characters (as imagens, os mapas e as fichas para onde os links apontam)

#### Critérios de aceite
Só o mestre do jogo vê o documento.

- **Dado** o documento da campanha, **quando** o mestre do jogo escreve texto, acrescenta uma imagem da galeria e um link para um mapa e para uma ficha, **então** o documento mostra a imagem **e** o link abre o mapa ou a ficha num diálogo, sem sair do documento.
- **Dado** um jogador, **quando** ele pede o documento, **então** o servidor recusa; o jogador vê "Só o mestre vê o documento da campanha", e a página da campanha não mostra o painel.
- **Dado** um não membro ou um membro pendente, **quando** ele pede o documento, **então** a campanha não existe para ele.
- **Dado** um documento, **quando** o mestre salva, **então** é um documento por campanha, em Markdown, de até 200 KiB. Se alguém salvou antes, o salvamento é recusado como conflito em vez de sobrescrever.
- **Dado** um link para um mapa, **quando** ele abre, **então** o diálogo mostra o mapa inteiro com todos os pontos (os escondidos marcados "Escondido"), a legenda, "Mapa atual da Sessão N" quando a sessão aberta está nesse mapa, e "Abrir no editor de mapas".

#### No app
- Links: `[texto](map:<id>)`, `[texto](character:<id>)` e `![legenda](image:<id>)`. O servidor guarda o texto como enviado; o app resolve cada link com a autorização de sempre.
- Telas: `/campaigns/:id/document` e a linha "Documento da campanha" na aba "Preparação" da página da campanha. Ver [Design](../../design/session.md#campaign-document-and-gallery) e [Arquitetura](../../architecture/campaigns.md#campaign-document).

### MR-019: Galeria de imagens

**Como** mestre, **quero** uma galeria de imagens, **para** usar as imagens nos documentos e nos mapas.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps

#### Critérios de aceite
Limites: JPEG, PNG ou WebP, até 10 MB por imagem, 300 imagens e 500 MB por campanha.

- **Dado** o mestre na galeria, **quando** ele envia uma imagem JPEG, PNG ou WebP de até 10 MB, **então** ela aparece na galeria **e** o arquivo guardado não tem nenhum metadado do original (EXIF). Uma imagem com pixels demais é recusada.
- **Dado** o envio, **quando** o mestre solta vários arquivos ou toca em "Enviar imagem", **então** eles são enviados um após o outro, cada um com o seu progresso e o seu "Cancelar envio". O app confere o tipo e o tamanho antes, e toda recusa vira um aviso em português com o nome do arquivo.
- **Dado** um jogador, **quando** ele pede a galeria da campanha, **então** o servidor recusa e a página diz "Só o mestre vê a galeria da campanha."; quem não é membro vê "Campanha não encontrada".
- **Dado** uma imagem usada num mapa, **quando** o mestre tenta apagá-la, **então** o app diz em qual mapa ela está ("Essa imagem está num mapa. Troque a imagem do mapa antes de apagá-la."); o cartão mostra "Usada em Mirathel e arredores".
- **Dado** a galeria, **quando** o mestre a usa, **então** as imagens aparecem da mais nova para a mais antiga, podem ser renomeadas no lugar e apagadas com uma confirmação no lugar, e abrem num diálogo com anterior e próxima.
- **Dado** um formulário que precisa de uma imagem ("Novo mapa", o editor de documento, "Mostrar imagem"), **quando** o mestre abre o seletor, **então** ele escolhe uma imagem ou envia uma nova e a escolhe na hora.

#### No app
- `/campaigns/:id/gallery` e o painel "Galeria" na aba "Preparação" da página da campanha ("12 de 200 imagens", as 6 imagens mais novas, "Enviar imagem" e "Ver todas as 12 imagens"), só para o mestre. Veja [Design](../../design/session.md#campaign-document-and-gallery) e [Arquitetura](../../architecture/maps/images.md).

### MR-024: Aprovar o personagem do convite

**Como** mestre, **quero** aprovar ou recusar o personagem que um jogador criou pelo convite, **para** que só fiquem na campanha os personagens que fazem sentido para a mesa.

- Prioridade: MVP
- Regras: RN-15
- Módulos: campaigns, characters

#### Critérios de aceite
Cada convite escolhe se exige aprovação, e uma recusa apaga o personagem e a participação.

- **Dado** que sou mestre de "Mirathel", **quando** gero um convite, **então** posso marcar "Exigir aprovação do mestre" **e**, desmarcado, quem aceita entra na hora.
- **Dado** um convite que exige aprovação, **quando** o jogador o aceita (já com sessão iniciada, ou entrando pelo convite), **então** ele vai direto para a criação do personagem, que nasce "Pendente de aprovação" **e**, enquanto espera, vê só o nome da campanha e o próprio personagem, que continua editando.
- **Dado** um personagem pendente, **quando** o mestre abre a campanha, **então** ele o vê em "Esperando aprovação" e pode abrir a ficha **e**, **quando** aprova, o personagem vira rascunho e o jogador passa a ser jogador da campanha.
- **Dado** um personagem pendente, **quando** o mestre o recusa, **então** o personagem é apagado, o jogador não entra na campanha **e** precisa de um novo convite para tentar de novo.
- **Dado** que sou jogador, ou jogador pendente, **quando** tento aprovar ou recusar um personagem, **então** o servidor recusa.
- **Dado** um jogador pendente que ainda não criou o personagem, **quando** o mestre abre a campanha, **então** ele vê quem está pendente sem personagem, com um botão para removê-lo; **e**, depois de 30 dias sem personagem, a participação pendente é apagada automaticamente.
- **Dado** um personagem pendente, **quando** o mestre toca em "Pedir ajustes" e escreve o que mudar (1 a 500 caracteres, obrigatório), **então** o personagem continua pendente, o convite continua válido **e** o jogador lê o motivo na ficha, edita e toca em "Enviar de novo".
  O mestre pode aprovar ou recusar a qualquer momento, e pedir de novo troca o motivo.
  Só o mestre e o dono leem o motivo.
  Ele é apagado quando o personagem é aprovado ou recusado, ou quando o personagem, a campanha ou a conta é apagada.
- **Dado** um jogador pendente de "Mirathel", **quando** ele aceita um convite simples (sem aprovação) para a mesma campanha, **então** ele vira jogador na hora, porque um convite simples vale como aprovação do mestre.
  Um personagem que ele tinha esperando também é aprovado, e o convite gasta um uso.
  Um convite que exige aprovação, ou que não é mais válido, não muda nada.

#### No app
- Um membro pendente só pode fazer o que o fluxo precisa; para todo o resto o servidor responde como se a campanha não existisse para ele.
- A campanha mostra "Esperando a aprovação do mestre" para o jogador pendente.
  A seção "Personagens" do mestre lista "Esperando aprovação", e a ficha tem "Aprovar personagem" e "Recusar personagem" (confirmada com "Confirmar recusa").
- Em "Membros", cada pessoa sem personagem mostra "Sem personagem", a data de entrada e a data em que sai sozinha; "Remover" pergunta no lugar, com o foco em "Cancelar".
  Se a pessoa criou o personagem nesse meio-tempo, a lista se atualiza e aponta para os personagens pendentes.
  Só o mestre vê isso.
- Veja [RN-15](regras.md), [Arquitetura](../../architecture/campaigns.md#pending-member) e [Modelo de dados](../../data/README.md#modules-and-tables).

#### Relacionadas
- Estende a [MR-003](#mr-003-entrar-pelo-convite): o personagem nasce pendente de aprovação (veja [Ciclo de vida do personagem](regras.md#ciclo-de-vida-da-ficha), RN-01).
- Estende a [MR-002](#mr-002-gerar-convite): o mestre escolhe, por convite, se ele exige aprovação.

### MR-028: Mostrar uma imagem aos jogadores

**Como** mestre, **quero** mostrar aos jogadores uma imagem da galeria durante a sessão, com uma ação própria, separada do mapa atual, **para** apresentar um retrato, uma carta ou uma cena.

- Prioridade: MVP
- Regras: RN-10
- Módulos: play, maps

#### Critérios de aceite
- **Dado** uma sessão aberta, **quando** o mestre mostra uma imagem da galeria, **então** todos na sessão a veem na hora, sem recarregar, **e** o mapa atual continua lá; **quando** o mestre para de mostrá-la, a imagem some da tela dos jogadores.
- **Dado** que a sessão acabou ou o mestre parou de mostrar a imagem, **então** os jogadores deixam de receber o ID da imagem, e um jogador não consegue baixá-la.
- **Dado** uma imagem que o mestre quer que os jogadores continuem vendo, **quando** ele liga "Deixar com os jogadores" e depois para de mostrar ou troca a imagem, **então** ela fica com os jogadores, numa lista "Imagens que o mestre deixou" na página da sessão deles, até o mestre removê-la ("Tirar"); a lista pertence à campanha e sobrevive ao fim da sessão.
- **Dado** um jogador, **quando** ele tenta mostrar uma imagem, ou o mestre tenta mostrar uma de outra campanha ou sem sessão aberta, **então** o servidor recusa.
- **Dado** uma imagem mostrada, **quando** o mestre olha quem a vê ("Quem vê a imagem"), **então** ela tem uma audiência de três tipos: a mesa toda; os jogadores da cena; ou jogadores escolhidos, qualquer conjunto de jogadores da campanha, escolhidos como o "Para" da conversa. A audiência é a mesa toda quando nenhuma cena está aberta ou a cena aberta está aberta para todos, exatamente como antes; enquanto uma cena aberta para só alguns jogadores está aberta, o padrão são os jogadores da cena, **e** o mestre pode escolher a mesa toda ou jogadores escolhidos ao mostrar a imagem.
- **Dado** uma imagem mostrada aos jogadores da cena, **quando** o mestre muda quem está na cena, **então** um jogador que entra vê a imagem na hora e um jogador que sai deixa de vê-la na hora; **quando** a cena fecha, outra abre, ou a cena abre para todos, **então** a imagem fica com os jogadores que estavam dentro naquele momento, até o mestre parar de mostrá-la ou mostrar outra: ela não chega a mais ninguém, e nenhum jogador a recebe retroativamente.
- **Dado** uma imagem mostrada, **quando** o mestre toca "Mostrar a toda a mesa", **então** todos os jogadores a veem na hora, como uma imagem mostrada sem cena; **quando** ele toca "Mudar quem vê" e escolhe os jogadores, **então** os escolhidos a veem, os outros a perdem na hora (a imagem some da tela deles, da leitura da sessão ao vivo e da rota da imagem, que responde 404) e os jogadores que ficam não ouvem nada.
- **Dado** que a imagem na tela é apagada da galeria, **então** só o mestre e os jogadores que a viram ficam sabendo que ela deixou de ser mostrada; um jogador fora da audiência dela não ouve nada, e as listas de imagens deixadas que ele lê não mudam (RN-10).
- **Dado** um jogador fora da audiência da imagem, **então** ele nunca a recebe: nem a imagem, nem o ID ou o nome dela numa leitura, nem num evento ao vivo (ele só fica sabendo que uma imagem que viu deixou de ser mostrada), nem pela rota da imagem, nem no resumo ou nos destaques (RN-10).
- **Dado** uma imagem com "Deixar com os jogadores" ligado, **quando** ela deixa de ser mostrada, **então** ela fica só com os jogadores que estavam na audiência dela (a mesa toda, ou esses jogadores); a lista que os outros jogadores leem não a tem, a rota da imagem responde 404 a eles e as telas deles não ouvem nada dela; "Tirar" funciona como antes. Deixar uma imagem de novo, com outra audiência, guarda a mais ampla: quem já a tinha continua com ela e os jogadores novos a recebem também; só "Tirar" a estreita. Um "Mudar quem vê" para a lista das imagens deixadas fica para depois: a audiência de uma imagem deixada não muda até ela ser tirada.

#### No app
- Uma imagem por vez, independente do mapa atual; uma sessão nova começa sem nenhuma.
  Apagar a imagem da galeria para de mostrá-la e a tira da lista.
  O nome da imagem é o nome do arquivo enviado e aparece para os jogadores como legenda, então o mestre pode renomeá-la antes de mostrar.
- Na sessão o mestre escolhe quem vê. Com a cena aberta para a mesa toda um toque mostra a imagem para a mesa; com ela aberta só para alguns jogadores, "Mostrar" numa miniatura é um menu ("Só quem está na cena", "Toda a mesa", "Escolher quem vê…"). Sob as miniaturas uma linha diz quem vê a imagem à mostra ("Pensantus e Brisa veem", "Só Brisa vê", "Toda a mesa vê", "Os jogadores veem"), com "Mostrar a toda a mesa" e "Mudar quem vê", que abre o seletor ("Quem vê “Vista do salão”?") e tira a imagem na hora de quem ficou de fora.
- Mostrar o fundo de um mapa com névoa mostra uma cópia dele ("… (névoa)"), para não entregar a névoa.
- Um jogador baixa a imagem só enquanto ela está sendo mostrada e ele está na audiência dela, enquanto está deixada com ele, ou enquanto é o fundo de um mapa que ele vê.
  Fora disso é um `404`, mesmo com o ID guardado.
  Veja [RN-10](regras.md) e [Servir imagens](../../architecture/maps/images.md#serving-images).
- Tela do mestre: com uma cena aberta, a linha "Imagens da cena" (o bloco tracejado "Da galeria", o interruptor "Deixar com os jogadores" com a palavra do estado numa linha, "Parar de mostrar" e a lista das imagens deixadas); sem cena aberta, o painel "Imagem para os jogadores" ("Mostrar imagem", o seletor da galeria com "Mostrar aos jogadores", "Trocar imagem", "Parar de mostrar"), o interruptor "Deixar com os jogadores" ("Ligado"/"Desligado", desligado sempre que uma imagem nova é mostrada) e a lista "Deixadas com os jogadores" com "Tirar".
- Tela do jogador: o bloco "O mestre está mostrando" com a imagem, o nome dela como legenda e "Ver em tela cheia", o primeiro da página no celular e com no máximo 40 % da altura da tela, que aparece e some ao vivo e é anunciado; e as miniaturas de "Imagens que o mestre deixou (n)", cada uma abrindo a mesma visão em tela cheia ("Ver <nome> em tela cheia"), uma dobra de uma linha no celular (a aba "Imagens" do cartão da cena enquanto há uma cena aberta) e aberta ao lado da cena no computador, escondida enquanto vazia.
  A lista fica só na página da sessão, não na página da campanha.
  Veja [Design](../../design/session.md#images-shown-to-players) e [Arquitetura](../../architecture/play/live-session.md#what-the-session-shows).
- A audiência faz parte da chamada de mostrar e tem uma chamada própria para mudá-la enquanto a imagem é mostrada (`SetShownImage`, `SetShownImageAudience`); o mestre a lê na imagem, e um jogador nunca.
- Fora do escopo: um "baú" por jogador com toda imagem já mostrada.
  A lista guarda só o que o mestre escolheu deixar, e um jogador lê as imagens deixadas com ele.

### MR-025: Cadastrar conteúdo da mesa

**Como** mestre, **quero** cadastrar raças, classes, subclasses, antecedentes, magias e regras que não estão no SRD 5.1, e definir as regras da minha mesa, **para** que a campanha use o material que a minha mesa joga e eu tenha controle total do produto.

- Prioridade: MVP
- Regras: RN-23, RN-24, RN-25
- Módulos: rules, characters, campaigns, play, maps

A história cobre classes **e** subclasses próprias, raças e sub-raças, antecedentes, magias, talentos, opções, características de classe, criaturas, o pacote de conteúdo (exportar e importar), os interruptores de opções, as regras da mesa e a grade da campanha.
A proposta de um jogador (MR-026) e a leitura de regras de um PDF (MR-027) são histórias à parte, para depois.
As regras de dados, os limites e o lado do servidor estão em [Conteúdo da mesa: regras](../../architecture/rules-table-content.md) e [Conteúdo da mesa: personagens](../../architecture/characters-table-content.md).

#### Critérios de aceite

**Conteúdo próprio.**

- **Dado** que sou mestre de "Mirathel", **quando** cadastro uma classe nova com o dado de vida, as perícias, a tabela de 20 níveis e as características, **então** a classe aparece no editor de personagem e na subida de nível só em "Mirathel" **e** a ficha calcula os números dela com isso (RN-23).
- **Dado** que sou mestre de "Mirathel", **quando** cadastro uma subclasse de uma classe do SRD (um colégio de Bardo, uma tradição de Mago), **então** ela aparece na escolha de subclasse daquela classe só em "Mirathel", com as magias sempre preparadas, se tiver.
- **Dado** que cadastro uma raça, uma sub-raça ou um antecedente, **então** ele aparece no editor de personagem com tudo o que o editor mostra (tamanho, deslocamento, visão no escuro, "+2 e +1 à escolha", idiomas, traços, perícias, ferramentas, equipamento e a característica) **e** o equipamento de um antecedente da mesa chega à ficha.
  Uma raça pode dar **grupos alternativos** de bônus de habilidade ("+2 e +1 ou +1, +1 e +1": escrevo em "Valores à escolha", separados por uma barra) **e** um **tamanho à escolha** (Pequeno ou Médio, com o tamanho da própria raça entre eles): o editor de personagem mostra os grupos na dica sob "Raça" e pergunta "Tamanho" só quando a raça de fato oferece dois; o jogador escolhe as habilidades em "Escolhas", como no meio-elfo: o grupo (quando há vários) e, para cada bônus, as habilidades que o recebem, nunca a mesma habilidade duas vezes, e os valores seguem as escolhas; o mesmo seletor serve a uma raça com um único "+2 e +1". A ficha de um jogador novo não pode ser salva antes das escolhas, e uma ficha que pôs os bônus em "Bônus manuais" antes de eles serem escolhas os mantém.
  O servidor recusa um bônus acima de +2 num grupo, mais de seis bônus, um tamanho repetido e uma raça que tem ao mesmo tempo uma lista única e grupos.
- **Dado** que sou mestre de "Mirathel", **quando** cadastro uma magia própria, **então** ela aparece para os personagens de "Mirathel" que podem aprendê-la **e** não em outra campanha.
  Se ela tem um ataque, ou um teste de resistência e dano, o combate a resolve como uma magia do SRD: o ataque, o teste com metade, a cura, várias criaturas (uma a mais por nível), uma área, o truque por faixa, a concentração e os espaços de magia.
- **Dado** que cadastro uma magia de área (um cone de 4,5 m, uma esfera de 6 m de raio), **quando** salvo, **então** a área aparece na descrição dela, **e** no combate quem conjura escolhe as criaturas atingidas, como numa magia de área do SRD.
  Uma magia com alcance "Pessoal" ou "Toque" é válida; "Pessoal" com "uma criatura" ou "várias" é recusada.
- **Dado** um conteúdo cadastrado em "Mirathel", **quando** abro outra campanha minha, **então** ele não aparece lá: o conteúdo é por campanha.
- **Dado** uma característica que o mestre cadastra, **quando** ele escolhe o efeito dela, **então** escolhe num menu fechado (modificador, proficiência, recurso, sentido, modo de rolagem, ação, ataque extra, ataque à distância, resistência, imunidade a condição, estado, regra de arma, arma da característica, gatilho num evento, escolha, dado de manobra, nota, magia concedida).
  Uma ação pode gastar um recurso que a entrada define (a de um talento, um que os efeitos do próprio talento definem: um talento da mesa com 3 usos por descanso longo), e uma característica de classe ou de subclasse pode ter uma **lista de opções** que o jogador escolhe (as manobras de uma mesa, cada uma com os seus efeitos; "Opções para escolher" no editor da característica).
  O conteúdo da mesa nunca executa código.
- **Dado** uma classe da mesa que conjura de uma lista própria, **quando** dou nome a magias do SRD nessa lista no editor da classe ("Magias do SRD na lista da classe"), **então** elas se juntam às magias da mesa que citam a classe: o personagem as prepara e aprende, o avanço de nível as oferece e a página "Magias" as lista para a classe.
  O servidor recusa uma magia desconhecida, uma magia da mesa, uma repetida e qualquer uma delas numa classe que reusa a lista de outra classe.
- **Dado** uma subclasse da mesa que cita magias, **quando** marco "As magias da subclasse entram na lista da classe, para escolher", **então** essas magias não vêm preparadas de graça: entram na lista da classe para quem tem aquela subclasse, que as escolhe como qualquer outra magia da lista, e contam no limite de magias conhecidas ou preparadas.
  Sem a marca elas são sempre preparadas. A marca só aparece quando a subclasse tem magias, e o servidor a recusa em caso contrário.
- **Dado** uma característica com um efeito de proficiência de nível "especialização" numa perícia, **quando** ligo "Só onde já tem proficiência", **então** a ficha dobra a perícia só para quem já é proficiente nela por outra fonte, e não dá proficiência sozinha.
  A especialização do próprio SRD funciona como antes.
- **Dado** uma característica ou talento da mesa com uma ação que gasta um recurso, **quando** defino o custo, **então** cada uso gasta essa quantidade (uma característica que gasta 2 de uma reserva para uma ação bônus), a ação fica desligada enquanto restam menos, **e** o servidor recusa um custo que o máximo do recurso nunca paga, um custo máximo menor que o custo e um custo sem recurso.
  Com um custo máximo, o jogador escolhe na hora quantos gastar, do custo ao máximo ou ao que restar ("quantos?"); o servidor lê os usos que restam e confere o número na mesma transação do uso.
  O cartão da ação mostra o custo e, com um custo máximo, "Usar" abre uma folha com "Quantos?" (um contador do custo ao máximo ou ao que restar), "Para quem" (o jogador ou outro personagem de jogador, numa ação que cura ou dá pontos de vida temporários), uma linha do que a escolha vai gastar e dar ("Gasta 3 · cura 3d6") e a rolagem dos dados; com menos usos do que o custo, a linha fica desligada com "Faltam usos".
- **Dado** uma ação que cura ou dá pontos de vida temporários (uma característica que gasta de 1 a 3 pontos para curar 1d6 por ponto; um talento que dá pontos de vida temporários iguais ao nível), **quando** o jogador a usa, **então** ela cura os dados mais o modificador da habilidade que escolhi, e cada ponto gasto acima do custo soma o valor por ponto, em quem usa ou em uma criatura escolhida na hora (quem usa e um aliado).
  As regras são as das magias: pontos de vida temporários não se acumulam (vale o maior), a cura nunca passa do máximo e um personagem morto não é curado.
  Uma cura sem dados é recusada.
- **Dado** um recurso, **quando** escolho como ele volta, **então**, além de descanso curto, descanso longo, amanhecer e nunca, ele pode devolver um uso quando o dono rola iniciativa (só se não restar nenhum), um uso no descanso curto (e todos no longo) ou um uso no descanso longo.
- **Dado** uma característica ou talento com um gatilho num evento (ao terminar um descanso curto ou longo, quando falha um teste contra a morte, quando o dono cai a 0 pontos de vida), **quando** o evento acontece com o dono, **então** o gatilho dá pontos de vida temporários, cura dados ou uma parte do máximo, devolve usos de um recurso da entrada e pode gastar um recurso para valer ("uma vez por descanso longo, levanta-se com metade do máximo quando um teste contra a morte falha"); um descanso terminado duas vezes nunca devolve um uso duas vezes.
  Só os gatilhos do dono rodam: um efeito sobre aliados que descansam por perto continua texto.
  Um gatilho num teste contra a morte que falha roda em cada rolagem que falha (um 1 natural, que conta duas falhas, roda uma vez só) antes de a falha ser somada; assim, o que levanta o personagem impede que a terceira falha o mate e zera as contagens; sem o recurso, não roda e a falha conta.
  Um gatilho ao cair a 0 pontos de vida responde a dano (um ataque, uma magia ou uma armadilha, em combate ou fora dele) e não ao "Ajustar PV" do mestre, nem a dano que mata na hora, e nunca levanta um personagem já morto ou morrendo; os pontos de vida temporários de um gatilho de descanso longo ficam depois de o descanso terminar os antigos.
  O que um gatilho dá aparece na linha que o descanso, o teste contra a morte ou o dano já escreve, com o público dessa linha, e desfazer devolve os usos que ele gastou.
  O servidor recusa um evento desconhecido, um gatilho sem resultado e um recurso que a entrada não define.
- **Dado** uma classe da mesa com uma subclasse que conjura (um conjurador de um terço), **quando** cadastro a subclasse com "Esta subclasse conjura", **então** a tabela do conjurador de um terço começa no nível em que ela começa, e as magias da subclasse vêm da lista de classe da qual ela conjura.

**Talentos.**

- **Dado** que sou mestre de "Mirathel", **quando** cadastro um talento com o pré-requisito (valores de habilidade — todos ou um deles —, uma proficiência, "consegue conjurar ao menos uma magia", uma raça ou sub-raça, ou um nível) e efeitos do menu fechado, **então** o talento aparece na lista de conteúdo como os outros tipos, em "Opções para os jogadores" e na subida de nível (se a mesa usa talentos), **e** a ficha de um personagem que o pegou o lista como "Talento" com os efeitos dele.
  Um talento cujo pré-requisito o personagem deixou de cumprir continua na ficha com um aviso e não vale até o pré-requisito voltar.
- **Dado** a regra "Talentos" de "Regras da mesa" ("permitidos" ou "não usados", padrão "não usados"), **quando** é "não usados", **então** a subida de nível oferece só o aumento de valores de habilidade; **quando** é "permitidos", **então** esse passo oferece "Aumentar habilidades" ou "Um talento", com os talentos para os quais o personagem se qualifica (o servidor confere o pré-requisito) e, para os outros, o motivo em português.
- **Dado** um talento que dá "+1 em uma de duas habilidades" (ou numa habilidade fixa), **quando** o jogador o pega na subida de nível, **então** a habilidade escolhida sobe 1 e nunca passa de 20, e o servidor recusa qualquer outro aumento.
  Um talento que aumenta uma habilidade já em 20 continua disponível: o aumento se perde e os outros efeitos valem.
  O resumo diz "Talento: <nome>" com o aumento, e a ficha lista o talento como "Talento · <classe> <nível>" no lugar do aumento que ele substituiu.
- **Dado** um traço, característica, característica de antecedente ou talento que concede um talento à escolha (uma `choice` do tipo "Talentos"), **quando** o jogador chega a ele, **então** a escolha é pedida onde as escolhas são pedidas (o editor, as escolhas pendentes, a subida de nível), o talento vale enquanto o dono está na ficha, um talento já pego não é oferecido de novo, e a regra "Talentos" não o barra.
- **Dado** que sou o mestre e abro o editor da ficha de um personagem, **quando** olho a seção "Talentos", **então** vejo os talentos da ficha, cada um com "Remover", e "Adicionar talento" com os talentos da campanha (os do SRD e os da mesa, sem os aposentados).
  - Quando adiciono um, o salvamento o mantém e os efeitos dele chegam à ficha, também quando a regra "Talentos" está desligada (a seção avisa).
  - Quando o personagem não cumpre o pré-requisito, o editor me avisa e posso mantê-lo; a ficha então lista o talento sem efeito, com um aviso.
  - Quando removo um talento pego no lugar de um aumento, o aumento volta.
  - Um talento que aumenta habilidades não pergunta quais: dou o aumento em "Bônus manuais".
- **Dado** que sou jogador, **então** o editor não tem essa seção e o servidor recusa um salvamento ou uma ficha nova cujos talentos difiram dos guardados: os talentos de um jogador vêm da subida de nível.
- **Dado** um talento da mesa com um efeito de ataque à distância ("Ataque à distância"), **quando** o dono dele ataca num combate, **então** o alcance, as listas de alvos, a classe de armadura da rolagem e o alcance longo o seguem.
  Para magias que pedem uma rolagem de ataque, o alcance é o dobro ou o triplo e a meia cobertura ou a cobertura de três quartos é ignorada.
  Para armas à distância (arma de arremesso a distância incluída), essa cobertura é ignorada e o alcance longo não dá desvantagem.
  A cobertura total ainda bloqueia o ataque, e a rolagem não mostra cobertura onde ela foi ignorada.
- **Dado** uma raça, um talento ou uma característica da mesa com um efeito de **resistência**, **quando** o escolho, **então** escolho os tipos de dano na lista do SRD (ou "Todos os tipos de dano", ou o dano contundente, perfurante e cortante não mágico) **e** o tipo em três cartões de escolha: resistência, imunidade ou vulnerabilidade; "Todos os tipos de dano, menos" e "Enquanto" (sempre, ou só em fúria) são os campos mais raros, em "Mais opções".
  Numa luta o dano que esse personagem sofre os segue: a imunidade leva a 0, a resistência divide por dois (arredondando para baixo), a vulnerabilidade dobra (depois da resistência), a mesma proteção de duas fontes conta uma vez só, e o limite a ataques não mágicos vale para um ataque com arma que não é marcado como mágico, nunca para uma magia nem para um dano que não é ataque.
  O mestre lê o passo na linha do dano; a ficha lista o efeito como uma nota.
  Uma recusa nomeia o campo: um tipo de dano que não existe, uma lista vazia, "menos" e uma lista juntos, "all" misturado com outros tipos.
- **Dado** uma entrada da mesa com **imunidade a condição** (uma ou mais das 15 condições), **quando** uma magia, um adicional, uma armadilha ou uma imobilização colocaria uma delas nesse personagem, **então** ele não fica com nenhuma: o acerto da magia diz "o alvo é imune à condição" para o mestre, uma agarrada falha e um empurrão não derruba.
- **Dado** um modo de rolagem "Vantagem ou desvantagem" da mesa com uma **condição** escolhida ("Só nos testes de resistência contra a condição"), **quando** o personagem rola um teste de resistência contra uma magia, um efeito, uma armadilha ou um adicional que dá essa condição, **então** a rolagem tem a vantagem ou a desvantagem e os motivos da rolagem dizem isso; sem condição, o efeito continua um lembrete da situação (as tags) e o mestre decide.
- **Dado** uma característica ou talento da mesa com um efeito de dano extra ("Dano extra") que tem dados (por exemplo, "uma vez por turno, +1d6 de fogo num acerto com arma corpo a corpo"), **quando** o dono dele acerta com uma arma que se encaixa, **então** o cartão de dano o oferece como um extra a marcar, e os dados são rolados junto com os da arma (no app, ou com os dados físicos que o jogador digita).
  O dano é uma linha de tipo próprio (o da característica, ou o da arma quando ela não diz nenhum), então uma resistência a esse tipo reduz só essa linha.
  Um acerto crítico dobra os dados, a menos que a característica diga o contrário; um número fixo nunca é dobrado.
  O extra fica desligado, com o motivo, no outro tipo de arma (corpo a corpo ou à distância), contra um alvo que não é maior quando a característica pede um maior, quando já foi usado neste turno (o padrão é uma vez por turno; "em todo acerto" não tem esse limite) e quando o recurso não tem mais usos.
- **Dado** uma característica ou talento da mesa com uma opção de magia ("Opção de magia") que serve a uma magia (por exemplo, "magias de evocação: +2 de dano de frio, 2 usos por descanso longo"; "dano de fogo ou de frio: trocar um pelo outro"; "um dado a mais por nível do espaço acima do nível da magia"), **quando** o dono dela abre a ficha de conjuração de uma magia que ela serve, **então** a ficha lista a opção; as que somam sozinhas aparecem como sempre ligadas, as outras são marcadas, e a troca de tipo pede o novo tipo de dano entre os que a magia pode ter.
  A conjuração rola o dano com a opção: o dano fixo é uma linha de tipo próprio quando o tipo difere do da magia, a troca muda o tipo do dano, e os dados extras vêm com o espaço escolhido.
  Uma opção com recurso gasta um uso junto com o espaço; sem usos ela fica desligada, com o motivo, e uma conjuração que a marca é recusada e não gasta nada. O desfazer do mestre devolve o uso.
- **Dado** uma opção da mesa com um dado nas rolagens do próprio personagem (por exemplo, "um d6 nos testes de Percepção", "um d6 nos testes de resistência de Constituição", "um d4 na iniciativa", de uma reserva de dois usos), **quando** o dono dela rola um teste de cena, um teste de concentração, um teste de resistência de efeito ou a iniciativa em que o dado serve, **então** a rolagem oferece o dado; escolhê-lo rola o dado junto com o d20 (no app, ou com a face digitada nos dados físicos), soma ao total, mostra pelo nome e gasta um uso.
  Um dado que não serve à rolagem (outra perícia ou habilidade), um que o personagem não tem e um sem usos são recusados e não gastam nada. Um número sempre ligado é um modificador da característica, não um dado.

**Deslocamentos e magias que a mesa concede.**

- **Dado** uma característica, um traço ou um talento da mesa com um modificador do deslocamento de voo, de natação, de escalada ou de escavação ("Deslocamento de voo", "de natação", "de escalada", "de escavação"; "somar", "pelo menos" ou "definir"), **quando** um personagem o tem, **então** a ficha lista esse deslocamento sob o deslocamento de caminhada, em metros e pés, **e** ele segue a condição do efeito (`when`).
  A fórmula pode ler `speedwalk()`, o deslocamento de caminhada já com os modificadores: uma raça que nada tão rápido quanto anda escreve "definir `speedwalk()`", e um talento que soma 3 m (10 pés) ao deslocamento de caminhada leva junto o de natação.
  No combate a criatura se move no deslocamento de voo quando ele é o melhor dos dois; nadar e escalar ficam só na ficha, porque a grade não tem custo para eles.
- **Dado** um efeito "Respira na água", **quando** um personagem o tem, **então** a ficha diz "Respira na água" na caixa do deslocamento; o app não conta fôlego nem afogamento.
- **Dado** um talento, um traço ou uma característica com o efeito "Magia concedida" (uma magia do SRD ou da mesa, o nível em que começa, a habilidade de conjuração — Inteligência, Sabedoria, Carisma ou "o jogador escolhe" —, o recurso que cada conjuração gasta, uma chave para conjurar também com espaço, uma chave para só ritual e uma chave para listá-la entre as conhecidas), **quando** um personagem o tem, **então** a ficha lista a magia em "Magias concedidas" com a origem, como se conjura e a CD e o bônus de ataque da concessão, **e** os números de magias conhecidas, truques e preparadas do personagem continuam como estavam.
  Uma magia de círculo é conjurada no próprio círculo, um truque cresce com o nível do personagem e os componentes materiais não são dispensados.
  Quando a habilidade é "o jogador escolhe", o jogador escolhe uma vez, nas escolhas da ficha, e até lá a concessão não tem números.
- **Dado** uma magia concedida, **quando** o jogador a conjura, em combate ou fora dele, **então** a ficha de conjuração a oferece como um jeito próprio, sem espaço de magia (espaço só quando a concessão permite), usando a habilidade da concessão para a jogada de ataque e a CD do teste de resistência.
  Uma concessão limitada gasta um uso do recurso: a segunda conjuração sem uso é recusada ("A concessão não tem mais usos."), o desfazer da primeira devolve o uso, e uma concessão à vontade nunca acaba.
  Uma concessão só de ritual conjura como ritual.
- **Dado** o editor de efeitos, **quando** cadastro uma magia concedida, **então** o servidor recusa uma magia que não existe, uma concessão só de ritual numa magia sem a etiqueta de ritual, um recurso que a entrada não define e uma habilidade que não é uma das quatro, cada um no seu campo.

**Opções para os conjuntos de opções do SRD.**

- **Dado** que sou o mestre de "Mirathel", **quando** cadastro uma opção para um dos conjuntos de opções do SRD (uma Invocação Mística, uma opção de Metamagia, um estilo de luta, uma Dádiva do Pacto, uma Presa do Caçador, Táticas Defensivas, Ataque Múltiplo ou Defesa Superior do Caçador) ou para a lista de opções de uma característica de classe ou subclasse da mesa (as manobras de uma mesa) com o texto, os efeitos do menu fechado e o que ela exige (um nível numa classe, outras opções, uma Dádiva do Pacto, uma magia que o personagem conhece), **então** a escolha desse conjunto a oferece, só em "Mirathel", junto das opções do SRD, **e** um conjunto com muitas opções da mesa não é cortado em 20, **e** uma opção para uma característica da mesa entra nas opções que ela lista, com os mesmos pré-requisitos.
- **Dado** uma opção da mesa, **quando** um personagem não cumpre o que ela exige, **então** a escolha a lista bloqueada com o motivo ("Exige o nível 5 de Bruxo. Você está no 3.", "Exige o Pacto do Tomo"), **e** o servidor recusa uma ficha que a escolhe mesmo assim; **quando** o personagem cumpre, **então** o jogador a escolhe e os efeitos dela (e a ação, se tiver) ficam na ficha.
- **Dado** a tela "Opções para os jogadores", **quando** o mestre desliga uma opção da mesa, **então** os jogadores não a recebem como oferta e o servidor a recusa como escolha nova; a ficha que já a tem a mantém.
- **Dado** o editor de opção, **quando** salvo uma opção com um conjunto desconhecido, uma classe, opção, Dádiva do Pacto ou magia que não existe, um nível fora de 1 a 20 ou outra escolha entre os efeitos, **então** o servidor recusa cada um no seu campo, **e** o conjunto de uma opção nunca muda depois de criada.

**Forma Selvagem e espaços de magia.**

- **Dado** uma característica da mesa que muda a Forma Selvagem, **quando** um druida com ela abre a lista de formas, **então** a lista e a escolha usam o maior nível de desafio, o menor custo em usos e ataques mágicos se alguma de suas características der; uma fera acima do nível é recusada ("WILD_SHAPE_BEAST_NOT_ALLOWED").
  Um custo 0 não gasta uso, e o desfazer devolve exatamente o que a mudança gastou.
- **Dado** uma ação da mesa que transforma usos em espaço de magia, **quando** um personagem a usa, **então** os usos são gastos e existe um espaço livre desse nível até o próximo descanso longo; o registro do combate lê os usos e o nível do espaço, e o desfazer tira o espaço e devolve os usos.
- **Dado** uma ação da mesa que gasta um espaço de magia para curar, **quando** o jogador escolhe um nível, **então** esse espaço é gasto e a cura cresce com os dados de cada nível acima do primeiro; um nível sem espaço livre é recusado e nada é gasto.
- **Dado** uma característica da mesa que recupera espaços de magia, **quando** o personagem recupera espaços depois de um descanso curto, **então** um uso do recurso dela é gasto e os espaços voltam; um espaço acima do nível máximo dela, um nível combinado acima do limite, um descanso que não é curto e um recurso sem uso são recusados e nada muda.

**Características de classe.**

- **Dado** que sou o mestre de "Mirathel", **quando** cadastro uma característica para uma classe (a do SRD ou uma da mesa) num nível de classe, com o texto e os efeitos, **então** todo personagem dessa classe a tem a partir desse nível, só em "Mirathel", como qualquer característica da classe.
- **Dado** uma característica de classe que substitui outra da classe (uma característica inventada de Patrulheiro no nível 1 que substitui Inimigo Favorito), **então** os personagens da classe deixam de ter a substituída a partir desse nível, **e** ela não é pedida nas escolhas deles; **quando** desligo a entrada, **então** a classe volta a ser como antes e a característica substituída volta.
- **Dado** o editor, **quando** salvo uma característica de classe com uma classe que não existe, um nível fora de 1 a 20 ou uma característica a substituir que a classe não tem nesse nível ou depois (ou que ela já tinha antes), **então** o servidor recusa cada um no seu campo.

**Criaturas.**

- **Dado** que sou o mestre de "Mirathel", **quando** cadastro uma criatura própria a partir de uma criatura do SRD (um lobo com 40 pontos de vida, classe de armadura 17, +2 no dano de cada ataque e um traço de texto), **então** ela vale onde vale uma criatura do SRD: eu a dou a um personagem de jogador, ela entra nos combates com esses números, e posso criar um NPC a partir dela.
  A criatura do SRD em si nunca muda, e a base da entrada não pode ser trocada por outra depois.
  O app web não tem editor de criaturas: elas são criadas pela API ou por um pacote de conteúdo.
- **Dado** que salvo uma criatura com uma base que o SRD não tem, pontos de vida fora de 1 a 9.999, classe de armadura fora de 1 a 40, bônus de dano fora de -10 a 30, ou mais de 10 traços, **então** o servidor recusa cada um no próprio campo, e um traço com o nome de um traço ou de uma ação da base, ou "Magic Weapons", "Spellcasting" ou "Innate Spellcasting", é recusado.
- **Dado** uma criatura sem fórmula já dada a um personagem, **quando** edito os pontos de vida e a classe de armadura da entrada, **então** a criatura mantém os pontos de vida com que foi dada **e** a classe de armadura e os ataques dela acompanham a entrada.
- **Dado** uma criatura cujos pontos de vida são `5 * level()` e cuja classe de armadura é `10 + prof() + mod("dex")`, **quando** a dou a uma maga de nível 3 e a um guerreiro de nível 5, **então** cada um tem os pontos de vida, a classe de armadura e os números de ataque do seu próprio dono.
- **Dado** uma criatura dada com fórmulas, **quando** o dono sobe de nível ou edito as fórmulas, **então** a criatura já dada mantém os números, e uma dada depois tem os novos.
- **Dado** uma criatura dada com fórmulas, **quando** acrescento, removo, renomeio ou reordeno os ataques próprios dela, **então** os ataques que ficam mantêm os números com que foram dados, um ataque novo tem os números fixos da entrada (um ataque renomeado é tratado como novo, porque o ataque é conhecido pelo nome, e também recebe os números fixos da entrada), um removido some, e os pontos de vida, a classe de armadura e os bônus da criatura não mudam.
- **Dado** uma criatura com mais de 8 ataques próprios, **então** o servidor a recusa (`limit` em `table_creature.attacks`; `TestACreatureFormulaRefusalsNameTheirField`).
- **Dado** uma criatura cujo terceiro ataque tem uma fórmula de acerto que não funciona, **então** a recusa nomeia esse campo (`table_creature.attacks[2].to_hit`, `TestMR578_TheFormulaFieldsOfACreatureHaveTheirPaths`).
- **Dado** uma fórmula que chama `slot()` em uma classe, uma característica, o dano de uma magia ou qualquer campo que não seja o de uma criatura ou o número de criaturas de uma invocação, **então** o servidor a recusa (`TestSlotIsLegalOnlyInACreature`).
- **Dado** que cadastro uma magia com invocação (uma magia de nível 2 só para quem conjura que traz o "Lobo de Prata" e um lobo, com número `1 + floor(slot() / 2)` e mais uma criatura por nível acima do da magia), **quando** um jogador a conjura com um espaço, **então** as criaturas entram no personagem, na iniciativa do combate e no mapa, o dono as joga nas abas de qualquer invocação, e uma criatura de fórmula tem os números do dono e do espaço usado.
  A magia passa pelas verificações das magias de invocação do SRD: o espaço, a concentração (uma nova conjuração dispensa as criaturas da antiga), a substituição das próprias criaturas quando peço, o limite de criaturas mantidas ao mesmo tempo, a dispensa e o desfazer, que devolve o espaço e remove as criaturas.
  As criaturas podem fazer uma rolagem de iniciativa para a conjuração ou uma cada.
  O servidor recusa agir logo depois do dono, uma montaria, uma magia paga com um recurso, um ritual, um número simples acima do limite de criaturas mantidas, uma criatura desconhecida, uma repetida, mais de 8 criaturas, um número que não funciona e uma magia que não seja só para quem conjura.
  Em um combate, criaturas que rolam cada uma a sua iniciativa pedem o rolador do app, porque um dado digitado é uma face só.
  Os jogadores leem as criaturas como em qualquer invocação: o nome e a chave no combate, e os números só o dono e eu.
  Um jogador que não tem nenhuma das criaturas lê a magia na lista da mesa sem as criaturas da mesa que não tem (as do SRD ficam) e sem o número e o limite; quem conjura lê as criaturas que pode escolher ao conjurar (`TestATableSpellThatSummonsKeepsItsCreatureAndCountFromThoseWhoHoldNone`).
  (`TestATableSpellSummonsThroughTheSRDChecks`, `TestATableSummonIsRefusedAtTheFieldThatIsWrong`, `TestMR578_ATableSpellSummonsItsCreaturesOutsideACombat`, `TestAFormulaCreatureThatAttacksKeepsItsNumbersFromAThirdPlayer`)
- **Dado** uma criatura dada com fórmulas, **quando** edito a entrada para não ter fórmula alguma, **então** a classe de armadura dela na lista e no combate continua a com que foi dada.
- **Dado** uma criatura com fórmulas que ponho no combate com "Pôr no combate" ou da qual faço um NPC, **então** ela tem os números fixos da entrada, porque não tem dono.
- **Dado** uma entrada de criatura, **então** um jogador a acha na lista de criaturas e lê a ficha dela só enquanto um personagem dele a tem.
  Isso vale com a entrada ativa, arquivada ou desligada.
  O mestre lê todas as entradas.
  Arquivar ou desligar não muda nada para as criaturas já dadas, e nada é apagado.
- **Dado** um monstro de uma criatura da mesa em combate, **quando** edito a entrada e ponho a criatura de novo no combate, **então** o novo combatente tem a classe de armadura e os ataques atuais da entrada.
  O monstro que já estava ali mantém os números com que entrou.
- **Dado** uma criatura da mesa sem fórmula que um personagem tem, em combate, **quando** edito a entrada, **então** a classe de armadura e os ataques dela acompanham a entrada na hora.
  Os pontos de vida continuam os que ela recebeu.
- **Dado** uma criatura da mesa em combate, **então** os pontos de vida dela são um número só para o mestre e o jogador dono, e a classe de armadura só para o mestre; os outros jogadores leem a palavra de estado.
- **Dado** uma criatura da mesa em combate, **então** todo jogador ali lê o nome e a chave dela, como numa invocação do SRD ou num NPC.
  A ficha de estatísticas, os pontos de vida e a classe de armadura ficam escondidos de quem não é o dono.
  Isso vale mesmo com a entrada arquivada ou desligada.
- **Dado** uma entrada de criatura arquivada, **quando** a dou a um personagem, **então** ela é dada.
- **Dado** uma entrada de criatura, **quando** a ligo ou desligo, **então** faço isso pela API, porque a tela "Opções para os jogadores" não tem grupo de criaturas.
- **Dado** as magias de invocação, a Forma Selvagem e as ferramentas de encontro (gerador, avaliador e trocas), **então** elas oferecem só criaturas do SRD, porque uma criatura da mesa não mantém o nível de desafio da base.

**Trocar uma escolha.**

- **Dado** que a regra da mesa "Trocar uma escolha no aumento de atributo" está ligada e um Guerreiro chega ao nível 4, **quando** a subida de nível chega à etapa de habilidades, **então** ela também oferece abrir mão de uma escolha anterior de um grupo (truque, estilo de luta, opção de Metamagia, invocação, Dádiva do Pacto) por outra do mesmo conjunto, com "Não trocar" como padrão, **e** a ficha mostra a escolha nova e não a antiga; a escolha nova ainda precisa dos pré-requisitos de qualquer escolha.
- **Dado** que a regra está desligada (o padrão), ou que o nível não tem Aumento de Habilidade, **então** a subida de nível não oferece troca e o servidor recusa uma (`SWAP_NOT_ALLOWED`).

**Mudar, arquivar e ligar e desligar.**

- **Dado** uma classe da mesa que uma ficha usa, **quando** o mestre muda um dos números dela, **então** a ficha mostra o número novo, as fichas travadas incluídas, **e** o que ficou fora das regras aparece na ficha como um aviso ("A classe mudou") com uma frase por mudança, sem bloquear nenhuma outra edição (RN-23).
  O aviso some sozinho quando os números voltam a bater.
- **Dado** um conteúdo da mesa que uma ficha usa, **quando** o mestre o arquiva, **então** a ficha o mantém **e** ele deixa de aparecer como escolha nova; nada do conteúdo da mesa é apagado.
  Uma entrada arquivada ainda pode ser editada e pode ser desarquivada, e então volta a ser uma escolha nova.
- **Dado** a tela "Opções para os jogadores", **quando** o mestre desliga uma classe, subclasse, raça, sub-raça, antecedente, magia, talento, opção ou característica de classe (do SRD ou da mesa), **então** os jogadores deixam de vê-lo e de escolhê-lo; o que está ligado eles veem por inteiro, com números e efeitos.
  Uma classe desligada leva as subclasses dela junto, e uma raça desligada, as sub-raças.
- **Dado** uma opção desligada ou arquivada, **quando** um jogador cria ou edita uma ficha ou sobe de nível, **então** o servidor a recusa como escolha nova, **e** uma ficha que já a tem continua funcionando e uma edição que não tem relação nunca é recusada.
  O mestre nunca é recusado.
- **Dado** uma escrita no conteúdo da mesa ou um interruptor, **quando** há uma sessão aberta, **então** as telas que mostram a mesa (a lista de conteúdo, as opções, "Magias", o editor de personagem, a subida de nível e a ficha aberta) se atualizam sozinhas.

**Cadastrar bem.**

- **Dado** que salvo uma entrada que quebra uma regra, **então** o servidor lista todas as violações de uma vez, cada uma no seu campo, e nada é salvo.
  Uma entrada é limitada a 60 características, e uma campanha a 1.000 entradas e 6.000 efeitos; uma fórmula é limitada a 1.024 bytes.
- **Dado** um jogador, **então** ele lê toda entrada jogável por inteiro, com "Da mesa", e nunca uma arquivada ou desligada, nem por referência, a não ser que a ficha dele a use.
  Ele nunca lê contagens nem estado, e não pode escrever.

**Pacotes.**

- **Dado** um conteúdo cadastrado em "Mirathel", **quando** aperto "Exportar" na lista de conteúdo, **então** o app baixa `<campanha>-conteudo.json`; **quando** escolho esse arquivo em outra campanha com "Importar pacote", **então** vejo a prévia (entradas novas, atualizadas, iguais e recusadas, com o motivo de cada recusa) **e** "Importar N entradas" escreve tudo ou nada.
  Uma entrada com a mesma chave é atualizada mantendo as chaves das características dela, a marca de arquivada e o interruptor; uma entrada nova vem ligada para os jogadores.
  Repetir a mesma importação escreve uma vez só.
  O pacote é um dado não confiável, lido de forma estrita e conferido pelas mesmas regras dos editores; os limites estão em [Pacotes de conteúdo](../../architecture/characters-table-content.md#content-packs-exporttablecontent-importtablecontent).
  Só o mestre exporta e importa.

**Criação de personagem.**

- **Dado** um personagem novo que entra no nível do grupo, **quando** o jogador o cria com mais de uma classe (Rafa cria Corvina, Mago 3 e Clérigo 1, no nível 4), **então** o editor tem um bloco por classe, com o nível e a subclasse de cada uma (as da mesa incluídas), **e** o servidor confere os pré-requisitos de multiclasse, como na subida de nível.
  Os pré-requisitos, os pontos de vida e os espaços de magia vêm do servidor como avisos e números.
- **Dado** um personagem com um antecedente de texto livre (o "Outro" do editor), **quando** o jogador o preenche, **então** ele escolhe, como na regra "Customizing a Background" do SRD 5.1, duas perícias, duas ferramentas ou idiomas no total, uma característica (com o texto dele) e o equipamento, **e** a ficha calcula com isso.
  O que falta é um aviso, nunca um erro.

**Regras da mesa (RN-24).**

- **Dado** que sou mestre de "Mirathel", **quando** escolho na página "Regras da mesa" como os pontos de vida são ganhos, como os valores de habilidade são feitos, o acerto crítico e quem vê os testes contra a morte, **então** o servidor segue a escolha em toda ficha e em todo combate; a distribuição padrão, a compra de pontos e o 4d6 aparecem com o rótulo "SRD 5.2.1 (regras de 2024)".
- **Dado** a regra dos pontos de vida na subida de nível, **então** a regra "rolar" recusa a média, a regra "média" recusa o dado, e o padrão aceita os dois; a subida de nível não mostra escolha quando a regra fixa o jeito.
- **Dado** os métodos de valores de habilidade que a mesa permite (distribuição padrão, compra de pontos com "Restam N pontos", 4d6 no servidor, digitar de 3 a 18), **quando** um jogador faz uma ficha nova, **então** o servidor aceita só esses métodos.
  Os NPCs e as edições do mestre continuam livres, e um jogador pendente também faz os valores dele.
- **Dado** uma campanha que permite 4d6 descartando o menor, **quando** o jogador rola os valores de habilidade de uma ficha nova, **então** o servidor rola e guarda, e rolar de novo devolve os mesmos, até uma ficha gastá-los.
  Com dados físicos, o jogador os digita uma vez e vale a regra de dados da campanha.
- **Dado** a regra do acerto crítico ("dados dobrados" ou "máximo mais uma rolagem"), **então** ela vale para todo acerto crítico: arma, magia, criatura e NPC, com mapa ou no teatro da mente.
  Com dados físicos, o passo do dano diz o que rolar ("role os dados duas vezes" ou "o máximo mais uma rolagem"), e o total mostrado antes de confirmar é o que o servidor guarda.
- **Dado** a regra "Os jogadores veem a CD dos testes de resistência" (desligada por padrão), **quando** está desligada, **então** o jogador cuja criatura faz um teste de resistência (uma zona, um efeito no início ou no fim de um turno, uma magia conjurada por um NPC ou por outro personagem, uma armadilha, o teste de Constituição de uma concentração, o teste de Destreza contra uma Repreensão Infernal que um NPC lança nele) lê a rolagem e se passou, nunca a CD; só o mestre a lê, e o jogador do conjurador a CD da própria magia.
  **Quando** o mestre a liga, **então** esse jogador lê a CD dos testes que as criaturas dele fazem, na pergunta, na resposta, no cartão do efeito, no lembrete do dano que ameaça uma concentração e no registro, a partir do próximo ("CD 14"); outro jogador, que não faz o teste, continua sem lê-la, e um conjurador ou uma armadilha escondidos continuam escondidos.
- **Dado** que os testes contra a morte são escondidos, **então** os outros jogadores leem só "Caído", "Estável" ou "Morto", sem marcas nem contagens, e o registro diz "Esta mesa só deixa o dono e o mestre verem os testes contra a morte".
  O dono e o mestre veem as marcas, com os rótulos "Só você e o mestre" e "Dono e mestre".
- **Dado** o modo de XP, **quando** o mestre o muda, **então** ele muda na hora se nenhum XP foi dado; senão o app pede confirmação com o número de prêmios e de XP, e nada é convertido.
- **Dado** os lembretes (até 20, de 1 a 200 caracteres), **então** o mestre os edita e os jogadores os leem.
- **Dado** apenas o mestre, **então** só ele escreve as regras; um jogador as lê só para leitura.
  Um estilo ("Estilo da mesa") preenche os dados, o combate e a névoa e deixa cada um editável; editar à mão dá "Personalizado".

**Grade e combate sem grade (RN-25).**

- **Dado** um mapa desenhado com quadrados de 3 m, **quando** o mestre diz "cada quadrado deste desenho vale 3 m" ("Calibrar o quadrado": 1,5 m, 3 m, 4,5 m, 6 m ou outro múltiplo de 1,5 m até 30 m), **então** o app conta quatro quadrados de 1,5 m em cada um, e o movimento, o alcance e a névoa seguem as regras de sempre.
  "Mudar a grade?" pergunta antes só quando a mudança apaga o que foi pintado.
- **Dado** um combate que o mestre começa sem mapa ("teatro da mente"), **quando** um jogador se move, **então** ele gasta movimento por número ("Restam 6 m"), sem posição na grade, **e** o mestre julga o alcance.
  - "Iniciar combate" pergunta "Como este combate é jogado": "Com mapa" ou "Sem mapa (teatro da mente)", com a regra da mesa "combate com mapa" como padrão ("Sem mapa" quando a sessão não tem mapa atual).
    Enquanto o app lê o mapa atual, ele espera ("Lendo o mapa atual…"); se a leitura falha, o botão fica desligado com "Não deu para ler o mapa atual" e "Tentar de novo".
  - O modo pertence ao combate e não muda até ele acabar.
  - O movimento nunca passa do que sobra no turno; a Disparada o dobra; fora do turno é recusado; o mestre gasta por um NPC e por um jogador que saiu; desfazer o restaura.
  - O mestre pode oferecer um ataque de oportunidade a um jogador ("Oferecer ataque de oportunidade"); a pergunta chega só a esse jogador, usa a única reação dele na rodada, e o mestre pode seguir sem esperar ("Seguir sem esperar") ou retirar a oferta ("Retirar a oferta").
  - Ataques, magias e ações funcionam sem alcance: a lista do jogador tem quem o personagem vê e as regras deixam atacar, sem distância ("O mestre decide quem está ao alcance"); o mestre define a cobertura dos alvos ("Cobertura dos alvos": nenhuma, meia +2, três quartos +5, total).
  - O que precisa de mapa não existe: mover, saltar e pôr no mapa são recusados com um motivo, e também as armadilhas, as portas, a névoa, o quadrado de uma criatura invocada e os olhos do familiar.
  - Agarrado e Impedido deixam o deslocamento em 0; Paralisado, Petrificado, Atordoado e Inconsciente não conseguem andar.
  - Um NPC escondido nunca é nomeado a um jogador.
  - O mestre vê o rótulo "Teatro da mente" e o cartão "Ações do …" com "Gastar movimento" (em passos de 1,5 m); o jogador vê "Combate sem mapa" e uma folha "Gastar movimento" com "Depois restam 3,0 m", que é só uma prévia do número do servidor.

#### As telas
- **"Conteúdo da mesa"** (`/campaigns/:id/content`, a partir do painel "Conteúdo da mesa" da campanha): o mestre, num notebook, vê os cinco tipos com contagens (arquivados incluídos), "7 de 1000 entradas", busca, "Mostrar" (Todas, Em uso, Sem fichas, Arquivadas) e linhas com o estado em palavras ("Em uso por 2 fichas", "Arquivado · 1 ficha usa").
  Uma campanha vazia diz "Nada cadastrado ainda." e que o SRD continua valendo.
  No celular o mestre só lê e arquiva.
  Um jogador lê toda entrada ligada, tipo por tipo, com "Da mesa".
- **Os editores** (uma página, um "Salvar"):
  - A magia: todos os campos, "Alvo" antes da mecânica, "Mais criaturas por nível", "Pessoal" e "Toque", as classes que a aprendem e uma prévia "Como os jogadores veem".
  - A raça e a sub-raça, e o antecedente.
  - A classe: nome, dado de vida, "Teste de resistência 1" e "2", quantas perícias e de quais, proficiências, o pré-requisito de multiclasse, conjuração (nenhuma, completa, metade ou pacto; preparadas ou conhecidas; a lista, e "Magias do SRD na lista da classe" para uma classe com lista própria) e o nível da subclasse.
    A tabela de 20 níveis começa nos números do SRD para a conjuração escolhida e o mestre a edita célula a célula; mudar a conjuração pergunta antes de refazer uma tabela editada.
    As características têm um nível e um efeito do menu, com o contador "N de 60".
  - A subclasse: características por nível, "Esta subclasse conjura", magias sempre preparadas por nível de classe e, quando tem magias, "As magias da subclasse entram na lista da classe, para escolher".
  - O talento: "Novo talento", "Dados do talento", "O que o talento pede", "Salvar talento"; "Aumento de habilidade" é oferecido só ali.
- **Recusas:** cada violação volta no campo que ela nomeia, em português, também numa célula da tabela e numa característica recolhida, que se abre.
  Um resumo no alto diz quantos campos precisam de ajuste, e o que o mestre digitou fica.
  "Esta entrada mudou enquanto você editava" oferece "Recarregar".
  Depois de salvar, "N fichas ficaram com aviso" lista os personagens que ficaram com um aviso.
- **"Importar pacote"** (escolher um arquivo `.json`; a prévia com contagens por tipo e o motivo de cada recusa; "Importar N entradas", desligado enquanto uma entrada é recusada) e **"Exportar"**.
- **"Opções para os jogadores"** (`/campaigns/:id/content/options`): um grupo por tipo com um contador ("Raças: 9 de 10 ligadas"), busca, "Ligar todas" e "Desligar todas", o interruptor de cada opção com quantas fichas a usam e a nota de um pai desligado, tudo salvo de uma vez.
  Os editores têm um interruptor "Disponível para os jogadores".
- **"Regras da mesa"** (`/campaigns/:id/rules`, a partir do painel "Regras da mesa" da campanha; o mestre edita, o jogador lê): o estilo, os pontos de vida, os métodos de habilidade (ao menos um), o acerto crítico, os testes contra a morte, "Os jogadores veem a CD dos testes de resistência" (desligada por padrão), os lembretes, o modo de XP, "Talentos" ("Não usados" por padrão, "Permitidos") e os links para a grade dos mapas e para o conteúdo da mesa.
  O modo de dados é editado só aqui.
  Veja [Design](../../design/maps.md#rules).
- **O editor de personagem e a subida de nível** mostram as classes, subclasses, raças, sub-raças e antecedentes da mesa entre os do SRD, com o rótulo "Da mesa".
  "Adicionar classe" faz um bloco por classe, com o nível total só para leitura.
  O passo "Magias" tem uma seção por classe (e por subclasse que conjura), mostra as magias sempre preparadas e tira uma magia fora das listas com o motivo e um link "Ver em Magias".
  O resumo da subida de nível mostra só as linhas que mudam, com "Da mesa" e "Novas características".
  O alvo e a área de uma magia vêm prontos do servidor, em metros ("Cone de 4,5 m").
  O passo "Habilidades" não mostra o modificador nem o bônus da raça; a ficha mostra.
- **Estados e regras de arma.** Uma característica pode definir um **estado** (uma postura, uma forma): um nome, uma duração em rodadas (uma fórmula; vazio ou 0 dura até acabar), as situações que o encerram além da duração (o jogador encerra, incapacidade, descanso curto, descanso longo), a ação que custa e, se quiser, um recurso de que gasta um uso.
  - A ficha e o combate mostram o estado como a carta de uma ação com "Ativar" (gasta a ação e o uso) e, enquanto ele está ligado, "Encerrar" (de graça). Ligar um estado que já está ligado, ou encerrar um que está desligado, é recusado e não gasta nada.
  - Os pontos de vida do jogador e a visão do mestre listam os estados ligados com as rodadas que faltam ("2 rodadas"); cada rodada do turno do dono desconta uma, e o estado acaba quando não resta nenhuma, ao chegar a 0 pontos de vida ou outra incapacidade quando ele diz isso, no descanso que ele nomeia e quando o combate termina (se tem duração). A correção do mestre encerra um à mão. Em um combate, o registro tem uma linha cada vez que um estado começa e cada vez que acaba, e diz por que acabou (por escolha, as rodadas acabaram, incapacidade, o mestre, o combate terminou); desfazer um "Ativar" ou um "Encerrar" tira a linha dele também. Fora de um combate não há registro de combate, então um descanso ou uma correção não escreve linha.
  - Os outros efeitos da entrada podem valer só enquanto um estado está ligado: resistência, modo de rolagem, dano extra, regra de arma e arma da característica trazem "Só enquanto o estado", e qualquer condição pode ler `active("nome")`. A resistência também pode usar `rage`.
  - Uma **regra de arma** muda as armas que o filtro escolhe (todas, corpo a corpo, à distância, acuidade, mágicas, uma pelo nome): outra habilidade para o ataque ou para o dano (vale a melhor), um bônus de -5 a 10 no ataque ou no dano, um alcance corpo a corpo de até 18 m.
  - Uma **arma da característica** põe na ficha e no combate um ataque próprio (nome, dados, tipo de dano, habilidade, alcance) que conta como uma arma em que o personagem tem proficiência, então as regras de arma também a mudam.
- **"A classe mudou":** o aviso na ficha (dono e mestre) e a folha "O que mudou", com as frases do servidor.
- A atribuição do SRD 5.2.1 está na página "Créditos".
- Veja [Design](../../design/sheet.md#editors), [Design: combate sem grade](../../design/combat.md#gridless-combat-teatro-da-mente) e [Arquitetura: regras da mesa](../../architecture/play/table-rules.md#table-rules-in-combat).

**Fora do escopo:** copiar conteúdo para outra campanha (MR-026), monstros e itens mágicos próprios, talentos fora do SRD, flanquear e a grade hexagonal.

#### Relacionadas
- [RN-23](regras.md), [RN-24](regras.md), [RN-25](regras.md); ADR-0017 (combate sem grade), ADR-0018 (conteúdo da mesa).
- [MR-040](#mr-040-subir-de-nível-pela-ficha) (subida de nível), [MR-045](#mr-045-consultar-as-magias).

### MR-010: Gerar masmorras

**Como** mestre, **quero** gerar uma masmorra (salas, corredores, portas e escadas, com opções de tamanho e estilo) e ter um mapa que eu possa editar, **para** não desenhar tudo na mão.

- Prioridade: MVP
- Regras: RN-10, RN-26
- Módulos: rules, maps

#### Critérios de aceite
- **Dado** que sou mestre de "Mirathel", **quando** peço uma masmorra com um tamanho (de 21 a 121 quadrados de lado, ou o que eu digitar), o formato (sem forma, anel, cruz, em L, elipse, losango), o tamanho das salas, o jeito dos corredores (labirinto, sinuosos ou retos), a quantidade e o tipo das portas e as escadas, **então** o app gera um mapa conectado com salas, corredores, portas e escadas: dá para chegar a todas as salas.
- **Dado** o mesmo tamanho, as mesmas opções e a mesma semente, **quando** gero duas vezes, **então** o resultado é o mesmo.
- **Dado** uma masmorra gerada, **quando** ela vira um mapa, **então** é um mapa da campanha como os outros, escondido dos jogadores (RN-10) e com a névoa de guerra ligada.
  A luz base dele é "Penumbra", que o mestre pode mudar: um mapa em "Claro" mostra aos jogadores a planta inteira, então uma sala atrás de uma porta secreta apareceria, enquanto em Penumbra, mesmo com uma imagem com textura, ela só aparece para quem a vê.
  A grade, as paredes e as portas já vêm pintadas nas camadas, as escadas são pontos de submapa, e a lista das salas fica ao lado do mapa, só para o mestre, com "Pôr uma cena nesta sala".
- **Dado** uma masmorra gerada, **quando** a edito no editor de mapas (paredes, portas, terreno, pontos), **então** "Redesenhar" redesenha a imagem a partir das paredes e portas como estão agora, sem apagar o que pintei.
- **Dado** uma porta fechada, **quando** um personagem anda até ela, **então** ela abre, se não estiver trancada; uma porta trancada impede o movimento; uma porta secreta é uma parede para os jogadores até eu revelá-la (RN-26).
- **Dado** a imagem gerada, **quando** os jogadores a veem, **então** ela tem só chão e paredes: as portas são desenhadas por cima, a partir da camada, e nenhum número de sala aparece.

#### No app
- O gerador é determinístico e foi escrito em sala limpa (ADR-0015): o código e os dados dele nunca são copiados de uma fonte com licença não comercial.
  Veja [O gerador de masmorras](../../architecture/rules-maps.md#dungeon-generator) e [O mapa de uma masmorra gerada](../../architecture/maps/dungeons.md#generated-dungeon-maps).
- **"Gerar masmorra"** (`campaigns/:id/maps/dungeon`, a partir do botão ao lado de "Novo mapa" no painel Mapas; só para o mestre, e no celular a página diz "Gerar masmorra é no notebook."):
  - Tamanho (Mínima 21, Pequena 31, Média 51, Grande 81, Enorme 121 ou "Outro" de 21 a 121; o outro lado é dois terços, ímpar), formato, o menor e o maior lado das salas, corredores, portas, becos sem saída, escadas (0 a 4) e a semente com "Outra semente".
  - A prévia é a do servidor, pedida um instante depois da última mudança; o app só a desenha, com a contagem "13 portas e 8 passagens".
    Uma opção fora do limite aparece no campo dela, com ícone e texto.
  - "Criar o mapa" mostra as etapas; o mapa nasce escondido, com a névoa ligada e a luz base "Penumbra", e abre no editor.
    "Parar de esperar" só para de esperar: o servidor termina o mapa de qualquer jeito, e a tela diz isso.
- **No mapa gerado.**
  - A lista "Salas" (só o mestre): número, tamanho, escadas, saídas com o tipo verdadeiro da porta, "Porta com armadilha: …", e a sala atrás de uma porta secreta.
  - "Pôr uma cena nesta sala" põe na hora o ponto de cena de RP na sala ("Sala N").
  - As escadas nascem reveladas, e um jogador só vê uma onde vê o quadrado; elas são desenhadas como um selo com uma seta e se chamam "Escada", nunca "Submapa".
- **"Redesenhar"** (oferecido só enquanto a imagem ainda é a do gerador) pergunta no lugar, primeiro envia os traços que estão esperando, e cada recusa tem a sua frase (imagem trocada ou grade mudada, grade removida, paredes mudadas enquanto desenhava, rajada de pedidos).
- **Portas na tela** (veja [RN-26](regras.md) e [Portas](../../architecture/maps/doors.md)):
  - No editor: a ferramenta "Porta" em "Pintar" (tipo, pôr, trocar, "Tirar a porta"), com a recusa própria da tela "uma porta precisa de chão dos dois lados" e uma pergunta no lugar antes de pôr uma porta fechada, trancada ou secreta onde há um token.
  - No mapa: uma marca por tipo de porta (fechada, aberta, trancada, grade, secreta) no editor, na sessão e no combate, e uma legenda feita das portas que o mapa tem.
    O cadeado e a porta secreta aparecem só para o mestre, com "(só você vê)" e o olho riscado; um jogador nunca recebe uma porta trancada ou secreta que talvez chegue (RN-10).
  - No combate: quem anda até uma porta fechada a abre e o registro diz "Toren abriu a porta."; uma porta trancada impede o movimento antes dela com "A porta está trancada.", e o mapa do jogador mantém "Porta fechada".
  - Na sessão, o mestre toca numa porta no mapa (com névoa, ou a do combate) e a abre, fecha ou tranca na folha da porta; sem névoa, as portas são mudadas no editor.
    Uma porta secreta tem "Revelar a porta secreta", que a pinta como fechada.
    Fora do combate só o mestre move tokens, então a porta abre pela folha do mestre.
- O gerador não põe monstros, armadilhas nem tesouro: para isso existem o bestiário, os encontros e o tesouro ([MR-042](#mr-042-bestiário) a [MR-044](#mr-044-gerar-tesouro)) e as armadilhas ([MR-035](#mr-035-armadilhas)).
  A [MR-039](#mr-039-imagens-geradas-para-masmorras-e-cenas) usa a masmorra gerada para fazer a imagem.
- Fora do escopo: masmorras de vários andares além das escadas ligadas, e a grade hexagonal.
  Desenhar uma masmorra à mão (paredes falsas, água, baús e mímicos) continua uma ideia para o editor de mapas.

### MR-029: Ganchos e pistas da cena

**Como** mestre, **quero** anotar, para cada cena de RP, os ganchos, as pistas e o que dizer, **para** conduzir a cena sem perder o fio.

- Prioridade: MVP
- Regras: RN-10, RN-20
- Módulos: play, maps, notes

#### Critérios de aceite
- **Dado** uma cena de RP no mapa, **quando** o mestre escreve nela os ganchos, as pistas e o que dizer, **então** só o mestre vê essas anotações.
- **Dado** uma pista numa cena, **quando** o mestre a revela para os jogadores que escolhe, **então** ela aparece para esses jogadores **e** nas anotações deles ([MR-030](#mr-030-anotações-do-jogador)).
- **Dado** uma pista não revelada, **quando** um jogador abre a cena ou a lista de anotações, **então** o servidor não envia nem a pista nem o nome dela (RN-10).
- **Dado** uma cena sem ações, **quando** o mestre a abre, **então** ela abre, e os jogadores a veem.

#### No app
- **Ganchos** ("Ganchos e anotações"): Markdown de até 4.000 caracteres, só num ponto de cena, salvo com o ponto ("Salvar ponto").
- **Pistas** ("Pistas"): de 1 a 500 caracteres cada, no máximo 30 por cena ("30 de 30" e a frase do limite), ordenadas com ↑ ↓, editadas no lugar e removidas com uma pergunta no lugar ("Voltar" com o foco).
  Cada uma mostra quem a tem ("Todos", "Só Brisa" ou "Ninguém ainda").
- **Revelar:** só os jogadores que o mestre escolhe recebem a pista, e ninguém vem marcado de antemão.
  Cada jogador a recebe uma vez; ela não pode ser desfeita nem escondida de novo.
  O jogador guarda uma cópia do texto, então editar ou apagar a pista depois não muda o que ele recebeu.
  Quem está offline a lê da próxima vez; os jogadores recontam o que acharam como quiserem, na mesa.
- **Na tela:** na cena aberta, a coluna de pistas e ganchos (aberta no notebook, recolhida no celular, o cadeado e "Só você vê" sempre visíveis) e "Revelar", que abre um diálogo (uma folha no celular) com ninguém marcado.
  O botão tracejado "Revelar a pista" diz por que não age, "Marcar todos" é uma ação de texto, e o botão cheio nomeia quem recebe ("Revelar para Brisa", "Revelar para 2 jogadores", "Revelar para todos").
  Depois, a pista diz "Revelada para todos às 21:20" ou "Revelada só para Brisa às 21:26", e "Revelar aos outros" aparece numa pista revelada em parte.
- Os links `map:` e `character:` do Markdown dos ganchos aparecem na cena aberta como no documento, mas não abrem nada ali.
- Veja [Arquitetura](../../architecture/play/scenes-and-stage.md#hooks-clues-and-discoveries-mr-029-mr-030).

### MR-030: Anotações do jogador

**Como** jogador, **quero** um bloco de notas sempre à mão, na página da sessão e na ficha, **para** anotar o que acontece sem sair do app.

- Prioridade: MVP
- Regras: RN-10, RN-20
- Módulos: notes, maps, play

#### Critérios de aceite
- **Dado** que estou na campanha, **quando** escrevo uma anotação na página da sessão ou na ficha, **então** só eu vejo a anotação **e** ela fica guardada para a próxima sessão.
  O mestre e os outros jogadores nunca a leem, e apagar a minha conta a apaga.
- **Dado** uma cena que já descobri (revelada ou aberta), **quando** marco a anotação com ela, **então** a anotação mostra a cena.
- **Dado** uma cena que ainda não descobri, **quando** abro a lista de cenas para marcar, **então** ela não aparece, e o nome dela nunca chega ao meu celular.

#### No app
- Uma anotação tem de 1 a 2.000 caracteres, no máximo 300 por jogador por campanha.
  Só um jogador ativo tem anotações.
- A lista também traz as pistas que o mestre revelou ("Pista do mestre", só para leitura, fora dos 300), da mais nova para a mais antiga, com um filtro por cena.
- Uma cena é descoberta quando o ponto dela é revelado no mapa ou a cena é aberta numa sessão (mesmo escondida), para o grupo todo, e continua descoberta se o ponto for escondido depois.
  Revelar é escolha do mestre, então uma cena revelada entra nas anotações do grupo todo mesmo onde nenhum personagem vê (MR-036); a névoa continua escondendo o mapa.
  Só uma cena descoberta serve de marca; uma não descoberta é recusada como se não existisse.
- **Na tela:** o botão "Anotações" na barra do app, em toda página da sessão do jogador; com uma pista nova ele diz "Anotações · 1 nova" ("Anotações · 1" abaixo de 360 px).
  Uma pista que chega abre um aviso ("O mestre revelou uma pista para você.", "Abrir anotações" e um ✕) que fica até ser aberto ou dispensado.
  A folha "Anotações" (uma folha no celular, um diálogo no notebook) tem a lista, o filtro por cena (só as cenas descobertas e "Sem cena", cada uma com a sua contagem), os dois estados vazios, "Nova anotação" com a marca de cena e o limite de 2.000, e editar e apagar (apagar pergunta no lugar).
- Na ficha do personagem (notebook), o painel "Anotações" é o primeiro bloco da quarta coluna, com as mesmas anotações; elas continuam editáveis com a ficha travada.
  O mestre nunca recebe o painel, nem na ficha de um jogador.
- Veja [Arquitetura](../../architecture/notes.md).

#### Relacionadas
- Uma pista revelada pela [MR-029](#mr-029-ganchos-e-pistas-da-cena) aparece nas anotações do jogador.

### MR-031: NPCs na cena

**Como** mestre, **quero** mostrar retratos de NPCs entrando e saindo da imagem durante uma cena, como numa visual novel, **para** que os jogadores vejam quem está "ali".

- Prioridade: MVP
- Regras: RN-10
- Módulos: play, maps

#### Critérios de aceite
- **Dado** uma cena de RP aberta, **quando** o mestre põe o retrato de um NPC (da galeria) na cena, **então** os jogadores o veem entrar, ao vivo.
- **Dado** um NPC na cena, **quando** o mestre o tira, **então** ele sai da imagem dos jogadores, ao vivo.
- **Dado** um NPC que ainda não entrou, **quando** um jogador consulta a sessão, **então** o servidor não envia nem o retrato nem o nome.
- **Dado** um NPC na cena, **quando** um jogador consulta a cena, **então** ele recebe só o nome, o retrato e se o NPC está falando: nunca o tipo, PV, CA, XP, ficha ou o ID do personagem (RN-20).
- **Dado** o retrato de um NPC, **quando** um jogador tenta baixá-lo, **então** só consegue enquanto o NPC está na cena; fora da cena, com a cena fechada ou trocada, é `404`.
- **Dado** um mestre que quer o retrato de um NPC, **quando** o escolhe na ficha, **então** só vale uma imagem da galeria da campanha, e só num NPC.
- **Dado** um retrato PNG com fundo transparente, **quando** o NPC entra na cena, **então** a transparência é mantida e o palco o desenha sem caixa atrás, só uma linha de base.
- **Dado** a cena aberta, **quando** um jogador toca num NPC no palco, **então** ele vê o NPC maior, com o nome e o retrato, e nada mais: a cena pertence inteira ao mestre.
- **Dado** uma cena aberta, **quando** o mestre põe um NPC no palco escondido ("Escondida dos jogadores"), **então** ele ocupa uma das 4 vagas e o mestre o lê marcado, mas nenhum jogador o recebe: nem o nome, o retrato ou o lugar no palco, nem na cena aberta, nem num aviso ao vivo, nem como alvo ou conjurador de uma conjuração, nem no histórico, no resumo ou nos destaques que lê (RN-10); os jogadores não são avisados quando ele entra ou sai do palco escondido.
- **Dado** um NPC no palco, **quando** o mestre o alterna entre "Os jogadores veem" e "Escondida dos jogadores", **então** mostrá-lo o faz entrar na imagem dos jogadores ao vivo, e escondê-lo o faz sair, exatamente como tirá-lo: nada diz aos jogadores que ele foi escondido em vez de tirado.
- **Dado** um NPC escondido, **quando** o mestre toca "Dar a fala", **então** ele é mostrado na mesma chamada e fala: os jogadores o veem entrar e falar na hora. Esconder o NPC que fala tira a fala dele.
- **Dado** um NPC escondido, **quando** o mestre fala na conversa como ele (MR-051), **então** a fala nomeia o NPC para os leitores que ele escolheu, sejam quais forem, e nada mais dele chega a eles: nem o retrato (a fala não leva nenhum e a rota da imagem responde `404`), nem um lugar no palco; falar na conversa não mostra o NPC, só "Dar a fala" mostra. Um jogador que não é leitor da fala nunca fica sabendo o nome.
- **Dado** o retrato de um NPC escondido, **quando** um jogador tenta baixá-lo, **então** é `404`, como fora do palco; o mestre sempre consegue.
- **Dado** um NPC que já está no palco, **quando** o mestre o põe de novo com o outro valor de "escondido", **então** nada muda: o alternador é o jeito de mostrar ou esconder. **Dado** um NPC escondido, **quando** o mestre dá a fala a ele, **então** ele é mostrado, de propósito: não há voz fora da tela.
- **Dado** uma cena aberta para só alguns jogadores, **quando** o mestre põe um NPC no palco, **então** só os jogadores da cena recebem o palco, os NPCs nele e os retratos deles, e os avisos do palco; um jogador que está fora não recebe palco nenhum, nem para mirar uma conjuração num NPC dele (MR-015, RN-10).
- **Dado** um NPC escondido, **quando** o mestre conjura uma magia por ele fora do combate, **então** a conjuração é só do mestre e nenhum jogador pode mirar nele, nem pelo lugar no palco; as conjurações que ele fez à vista saem da lista dos jogadores enquanto ele está escondido e voltam quando é mostrado.

#### No app
- O palco comporta até 4 NPCs por sessão, em ordem, com quem está falando; fechar ou trocar a cena o esvazia.
  Um NPC escondido no combate pode entrar na cena sem ser revelado na ordem do combate.
  Veja [Arquitetura](../../architecture/play/scenes-and-stage.md#npcs-on-stage).
- O editor de NPC tem "Retrato" (a ficha curta e o passo Básico de inimigo e chefe): iniciais sem imagem, "Escolher/Trocar retrato" no seletor da galeria, "Remover retrato" com a pergunta no lugar, tudo salvo com a ficha.
  O retrato aparece no cartão do NPC no combate e no cabeçalho da ficha.
- O mestre vê "Em cena" na cena aberta (um olho por NPC: "Escondida dos jogadores" deixa o NPC no palco só para o mestre, e um NPC escondido nunca fala; "Dar a fala"/"Fala agora", "Tirar de cena", "Pôr em cena" no lugar ou numa folha no celular, "4 de 4").
  Os jogadores veem o palco com recortes sem caixa e quem fala em destaque; o palco some quando vazio.

#### Relacionadas
- Usa as imagens da [galeria](#mr-019-galeria-de-imagens), como a [MR-028](#mr-028-mostrar-uma-imagem-aos-jogadores).

### MR-032: Destaques do combate

**Como** mesa, **quero** uma tela de destaques no fim do combate, **para** celebrarmos quem fez o quê: qual jogador causou mais dano, curou mais e levou mais dano.

- Prioridade: MVP
- Regras: RN-20
- Módulos: play

#### Critérios de aceite
- **Dado** um combate que termina, **quando** o mestre o encerra, **então** a mesa vê cinco categorias, "Mais dano causado", "Mais cura", "Tanque", "Golpe final" e "Acertos críticos", cada uma com o seu número e todos que empataram; uma categoria em que todos têm 0 fica de fora.
- **Dado** um golpe de 28 de dano num goblin com 7 PV, **quando** os destaques são calculados, **então** ele conta 7: só os PV que de fato saíram. Os PV temporários que absorveram dano contam como dano causado e sofrido.
- **Dado** um bruxo que acertou um goblin com um truque por 5 no seu turno e respondeu ao turno de outra criatura com Repreensão Infernal por 10, **quando** os destaques ou o resumo da sessão são calculados, **então** o dano causado dele é 15: o dano causado fora do turno da criatura (uma reação, um ataque de oportunidade, o dano posterior de uma zona ou de um efeito) conta para a criatura que o causou.
- **Dado** uma ação que o mestre desfez, ou um dano que ninguém aplicou, **quando** os destaques são calculados, **então** ela não conta.
- **Dado** que um NPC causou ou levou dano, **quando** os destaques aparecem para um jogador, **então** mostram só os números dos jogadores, nunca os PV, a CA ou as rolagens dos NPCs (RN-20).
- **Dado** um jogador, **quando** ele pede os destaques de um combate encerrado, **então** recebe só a linha do próprio personagem (zeros incluídos) e nunca a de outro jogador; só o mestre recebe a mesa inteira; um combate que não terminou é recusado.
- **Dado** uma sessão que terminou, **quando** o mestre abre o resumo, **então** ele vê a duração, quantos combates e cenas houve, "Testes passados fora do combate" ("9 de 12"), os destaques de todos os combates somados e a tabela por jogador ("Pensantus: 3 de 4"); **e** um jogador vê os vencedores de cada categoria com o número deles (sem o "de N") e o próprio resultado, nunca o de outro jogador (RN-20).
- **Dado** que a sessão teve testes de cena com uma CD, **quando** o resumo é mostrado, **então** ele tem o destaque "Mais testes passados fora do combate" (como enganação e intimidação), contando só rolagens de cenas que mostraram a CD aos jogadores e só de ações com CD: uma rolagem numa cena que escondeu a CD não conta em nenhuma categoria, para ninguém.
- **Dado** que a sessão teve tesouro encontrado ([MR-041](#mr-041-tesouros-e-xp-por-ouro)), **quando** o resumo é mostrado, **então** ele tem o destaque "Mais tesouro encontrado": as peças de ouro (PO) que cada personagem encontrou naquela sessão, com um tesouro encontrado por dois dividido entre eles, arredondado para baixo; um tesouro encontrado fora de uma sessão, ou desmarcado depois, não conta; todo membro recebe a categoria, em qualquer modo de XP.
- **Dado** uma sessão ainda aberta, **quando** alguém pede o resumo dela, **então** é recusado: só uma sessão encerrada tem resumo.
- **Dado** uma campanha com sessões encerradas, **quando** um membro abre a página da campanha, **então** "Sessões anteriores" as lista da mais nova, cada uma com o número, o dia e as horas em que rodou, quanto durou e "Ver resumo", que abre o resumo daquela sessão; **e** o mestre vê o painel com "Nenhuma sessão encerrada ainda" antes da primeira, enquanto um jogador não vê painel até haver uma.
- **Dado** a página de uma sessão encerrada, **quando** alguém que não é membro, ou um endereço com um número que não é número, a abre, **então** vê uma página genérica que não nomeia a campanha; **e** um membro que pede um número que a campanha não tem lê "Não há Sessão 9" com um link para a última sessão.

#### No app
- **Fim do combate.** O resumo do combate do mestre tem "Destaques do combate" e a tabela "Números de cada jogador".
  O jogador vê o cartão "O combate acabou", com "Você" e "Seu resultado" (os cinco números do próprio personagem, zeros incluídos).
- **Fim da sessão.** O mestre cai em "Sessão encerrada" com os números, "Resumo da sessão" e a tabela por jogador, e "Voltar à campanha".
  O jogador recebe o cartão "A sessão acabou" com os vencedores e "Seu resultado"; "Fechar" deixa o aviso simples "A sessão acabou", que é também o que uma leitura que falhou mostra.
- **"Mais tesouro encontrado".** O mestre recebe um bloco com uma linha por personagem que encontrou algo e a nota "Quem não encontrou nada não aparece. Um tesouro encontrado por duas pessoas divide o valor, arredondado para baixo."; o jogador recebe um bloco e "Tesouro encontrado" em "Seu resultado".
  Nenhum dos dois traz o nome nem a hora do tesouro.
- **Sessões anteriores.** A página `/campaigns/<id>/sessions/<number>` desenha o mesmo corpo da tela de fim de sessão; "Sessão 4 em andamento" leva à sessão ao vivo, e o mestre vê "Não houve combate nesta sessão." quando não houve combate.

#### Relacionadas
- [MR-041](#mr-041-tesouros-e-xp-por-ouro): o tesouro que alimenta "Mais tesouro encontrado".
- Veja [Arquitetura](../../architecture/play/highlights-and-summary.md#combat-highlights) e [Arquitetura](../../architecture/play/highlights-and-summary.md#session-summary).

### MR-033: Imprimir o mapa com a grade

**Como** mestre, **quero** imprimir o mapa atual, ou salvá-lo em PDF, com a grade na escala da mesa, **para** jogar com miniaturas.

- Prioridade: MVP
- Regras: RN-21
- Módulos: maps

#### Critérios de aceite
- **Dado** um mapa com grade, **quando** o mestre abre "Imprimir com a grade", **então** ele vê a tela "Imprimir o mapa" com o quadrado de 2,54 cm (uma polegada) e papel A4, e a contagem de folhas: 30 × 20 quadrados dão 76,2 × 50,8 cm, em 9 folhas A4.
- **Dado** a tela de impressão, **quando** o mestre muda o tamanho do quadrado (de 1 a 10 cm, com vírgula ou ponto) ou o papel (A4, A3, A2, A1, Carta ou Ofício, o brasileiro de 21,6 × 33 cm), **então** o resumo, a prévia das folhas e "As contas" (uma linha por papel, nas duas orientações) mudam na hora; a orientação é a que usa menos folhas, paisagem em caso de empate. Com 2 cm em A3, são 3 folhas em retrato.
- **Dado** um mapa maior que uma página, **quando** o mestre imprime, **então** o mapa é dividido em folhas com margem de 1 cm e sobreposição de 1 cm, com a grade alinhada entre elas, cada folha com o rótulo de onde colar ("Página B2 · cole à direita da B1 e abaixo da A2") e uma régua de 5 cm para conferir a escala.
- **Dado** um tamanho fora de 1 a 10 cm, **quando** o mestre o digita, **então** a tela mostra o erro, deixa a prévia vazia e o botão "Imprimir" tracejado, sem fazer nada.
- **Dado** uma impressão de mais de 16 folhas, **quando** o mestre a configura, **então** um aviso âmbar nomeia o papel que usa menos folhas; acima de 36, a prévia deixa de mostrar os rótulos. Nada é bloqueado.
- **Dado** um mapa sem grade, **quando** o mestre olha a página do mapa, **então** o botão "Imprimir com a grade" está desligado, com o motivo escrito ao lado.
- **Dado** um mapa escondido dos jogadores, **quando** o mestre imprime, **então** o mapa inteiro sai: a impressão é do mestre. O jogador nunca vê o botão nem a tela (a rota responde "Só o mestre imprime o mapa.").
- **Dado** a impressão, **então** saem só a imagem e a grade: pontos, marcas e tokens ficam de fora.

#### No app
- Roda só no navegador, com estilos de impressão e a impressão ou o salvar como PDF do próprio navegador. A tela pede escala de 100% e sem cabeçalhos e rodapés, porque o navegador pode encolher a página; a régua de 5 cm em cada folha deixa o mestre conferir.
- Veja [Design](../../design/maps.md#printing-pagesmapsmap-print).

#### Relacionadas
- [RN-21](regras.md).

### MR-034: Movimentos especiais

**Como** jogador, **quero** que o app cuide de saltar, terreno difícil e cobertura, **para** que as regras de movimento valham na tela como valem na mesa.

- Prioridade: MVP
- Regras: RN-21
- Módulos: play, rules

#### Critérios de aceite
- **Dado** um personagem com Força 16, **quando** ele salta, **então** o app mostra o alcance do salto em distância e altura, calculado a partir da Força (16 pés de comprimento e 6 pés de altura com corrida, metade de cada um parado), e desconta a distância do movimento.
- **Dado** um monge que usou Passo do Vento, ou um Ladino Ladrão com Andarilho de Telhados, **quando** ele salta, **então** a distância do salto é dobrada no turno, ou o salto em distância com corrida ganha o modificador de Destreza em pés.
- **Dado** um quadrado marcado como terreno difícil, **quando** o personagem entra nele, **então** cada quadrado custa o dobro do movimento, e a Disparada continua funcionando junto.
- **Dado** que o mestre pinta o mapa, **quando** usa "Pintar", **então** ele pode marcar na grade terreno difícil, paredes, cobertura (meia ou três quartos) e luz; os jogadores de um mapa que veem leem parede, terreno e cobertura, nunca a luz; mudar a grade ou a imagem apaga a pintura, e as duas coisas são recusadas enquanto há um combate no mapa.
- **Dado** um alvo atrás de uma cobertura marcada no mapa (meia ou três quartos), **quando** alguém o ataca ou ele resiste a um efeito de Destreza, **então** o servidor soma +2 ou +5 à CA ou ao teste, ao longo da linha entre os dois; atrás de uma parede ele não pode ser alvo. O mestre também pode marcar a cobertura de um combatente para o que o mapa não mostra.
- **Dado** outra criatura no caminho, **quando** o personagem passa pelo espaço dela, **então** o espaço de um não inimigo custa como terreno difícil, o de um inimigo não pode ser atravessado (a não ser que os tamanhos difiram em dois), e ninguém termina o movimento no espaço de outro.
- **Dado** o turno de um NPC num mapa, **quando** o mestre arrasta a peça dele, **então** ela anda como o movimento de um jogador: o registro diz o tamanho da linha em metros ("Espada voadora 2 anda 12 m", contado como o app conta todo movimento), o "Restam 15 m · 10 quadrados" do NPC desce pelo que o caminho custou (entulho incluído), ela para antes de paredes, criaturas e quadrados marcados "Não dá para passar", e um arrasto além do movimento que resta pergunta "Espada voadora 2 não tem movimento para tanto. Faltam 4,5 m do que resta. Mover mesmo assim, sem gastar o movimento?" com "Mover mesmo assim" (de graça, sem ataques de oportunidade, para uma corrida ou um empurrão) e "Cancelar". Fora do turno do NPC o arrasto continua sendo uma colocação livre.
- **Dado** um inimigo ao lado do personagem que o vê e tem a reação, **quando** o personagem sai do alcance dele sem ter usado Desengajar, **então** o app oferece um ataque de oportunidade a quem controla o inimigo, e o movimento espera a resposta: o ataque, "Seguir sem esperar" ou uma recusa. Um salto em distância também provoca; um movimento que o mestre marca como "Movimento forçado" não. Com 0 PV por causa do ataque, quem se movia volta ao quadrado de onde saiu, se estiver livre.
- **Dado** um movimento, **quando** o mestre o desfaz, **então** as ofertas que ele causou vão junto.

#### No app
- **Página de mover.** "Mover" mostra todo quadrado alcançável, o custo de cada quadrado, o motivo de um quadrado ser recusado e quem pode provocar; tem "Saltar" com a distância e a altura que o servidor permite.
- **Cobertura na tela.** A lista de alvos diz a cobertura e a origem dela ("Meia cobertura (do mapa)", "Três quartos (marcada pelo mestre)") e deixa de fora quem uma parede cobre por inteiro; a ordem do mestre tem "Marcar cobertura" e "Marcar como aliado".
- **Editor de mapas.** No notebook, "Pontos | Pintar" tem Terreno difícil, Parede, Cobertura (Meia, Três quartos), Luz (Claro, Penumbra, Escuro) e "Não dá para passar" (um xadrez: ninguém entra no quadrado, mas a visão e a luz passam), "Apagar" e um pincel 1×1 ou 3×3; "Camadas" a lista com o interruptor dela.
  Arrastar, clicar, Shift para apagar e o teclado pintam. "Camadas" diz "Tudo salvo", "Salvando" ou "Não salvou".
  Sem grade, as ferramentas ficam tracejadas ("Defina a grade para pintar e ligar a névoa.").
  Mudar a grade, trocar a imagem e "Esquecer o que foi visto" perguntam no lugar antes.
  No celular não há pintura ("Pintar só no computador"). O que está desenhado na imagem, os jogadores veem.
- Veja [Arquitetura](../../architecture/rules-maps.md#grid) e [Arquitetura](../../architecture/maps/editor.md#layers).

#### Relacionadas
- [RN-21](regras.md).
- Regras escolhidas: meia cobertura é um muro baixo ou caixotes, três quartos uma coluna ou uma seteira; cada quadrado de terreno difícil em que se entra custa 1,5 m a mais.

### MR-035: Armadilhas

**Como** mestre, **quero** pôr armadilhas escondidas no mapa, com a CD para notar e para achar, o gatilho e o efeito, **para** surpreender a mesa com as regras do jogo.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps, play

#### Critérios de aceite
- **Dado** um mapa, **quando** o mestre põe uma armadilha, **então** ele define a CD para notar (Percepção passiva, opcional) e para achar (Investigação), a área (1×1 a 4×4), o gatilho (ao entrar na área ou à mão) e o efeito em partes (um ataque, dano que sempre acontece, condições, um teste de resistência com o que acontece numa falha e num sucesso); só o mestre a vê. As oito armadilhas de exemplo do SRD são predefinições, e tudo continua editável.
- **Dado** uma armadilha escondida, **quando** um personagem (ou uma criatura dele) termina um movimento a até 3 m dela, vendo um quadrado da área, com Percepção passiva igual ou maior que a CD para notar (menos 5 em penumbra), **então** o jogador dele passa a conhecer a armadilha, e só esse jogador é avisado. Uma armadilha sem CD para notar nunca é notada assim.
- **Dado** uma armadilha escondida, **quando** um jogador usa "Procurar armadilhas" e passa num teste de Percepção (contra a CD para notar) ou de Investigação (contra a CD para achar), com o d20 do app ou um dado físico (RN-18), contra uma armadilha a até 3 m cujo quadrado o personagem vê, **então** só esse jogador passa a conhecê-la.
- **Dado** a busca, **quando** nada é achado ou o teste falha, **então** a resposta é igual e não traz CD. Num combate a busca é a ação Procurar: só no turno do personagem, e gasta a ação. Uma busca de Percepção em penumbra tem desvantagem.
- **Dado** "Outra perícia…", **quando** um jogador procura com qualquer outra perícia, **então** ela enfrenta a CD para achar só nas armadilhas cujo mestre listou essa perícia ("Também acham com"; nenhuma por padrão); em qualquer outra armadilha ela não acha nada, exatamente como uma busca que ficou aquém.
- **Dado** uma armadilha, **quando** um personagem entra na área dela (um movimento em linha reta para no primeiro quadrado da área; um salto só dispara onde pousa; um token que o mestre solta dentro dispara; um NPC nunca dispara uma) ou o mestre a dispara à mão, **então** o servidor rola o ataque, o dano e o teste de cada criatura apanhada, e a armadilha fica visível para todos como "Disparada". Um token só dispara uma armadilha num mapa que os jogadores podem ver.
- **Dado** o efeito, **quando** ele atinge, **então** o dano a um NPC ou a uma criatura é aplicado na hora e as condições vão para os combatentes; o dano a um personagem de jogador espera o mestre (RN-02), que pode mudar o valor, aplicá-lo ou descartá-lo, dentro ou fora do combate, e ele sobrevive ao fim do combate e da sessão.
- **Dado** um movimento que entraria numa armadilha que o personagem conhece, **quando** o jogador o planeja, **então** a tela pergunta "Isso entra no Fosso escondido. Mover assim mesmo?"; uma armadilha que ele não conhece nunca é nomeada, e um movimento cortado por ela só diz que parou.
- **Dado** um combate, **quando** o mestre desfaz o disparo, **então** a armadilha volta a ficar armada, os PV e as condições voltam e o dano pendente é apagado, e só depois o movimento.
- **Dado** uma armadilha disparada, **quando** o mestre a desarma depois do teste de ferramentas de ladrão da mesa, **então** ela nunca mais dispara.
- **Dado** uma armadilha ainda não achada, disparada ou revelada, **quando** um jogador consulta o mapa, **então** o servidor não envia nem a posição, as CDs, o efeito ou a predefinição dela (RN-10). O mestre pode revelar uma armadilha a jogadores escolhidos.

#### No app
- **Mestre, ao pôr.** "Armadilha" na barra de pontos abre o painel com as oito predefinições como botões de opção (cada uma com uma linha como "Queda de 6 m, 2d6") e "Começar do zero", o nome, "Descrição para você", "CD para notar (Percepção)", "CD para achar (Investigação)", a área, o gatilho, as partes do efeito, quem é apanhado e o estado (Armada, Disparada, Desarmada).
  O aviso "Não desenhe a armadilha na imagem: os jogadores veem a imagem." fica no formulário.
- **Mestre, na sessão.** "Armadilhas do mapa" tem um cartão por armadilha, com "Quem notaria" (cada personagem com a Percepção passiva, se está no mapa, a até 3 m, e se notaria: "Nota", "Não nota", "Se chegar a 3 m: nota"), "Revelar para…", "Disparar…", "Desarmar", o dano esperando ("Aplicar N de dano" ou "Não aplicar") e o "Registro".
  "Disparar…" sem ninguém marcado dispara para quem está na área, e a tela diz isso; um disparo que não apanhou ninguém deixa um aviso visível.
- **Jogador.** "Procurar armadilhas" oferece Percepção, Investigação e "Outra perícia…", com o mesmo menu para toda armadilha e o resultado "Achou" ou "Nada"; o aviso "Você notou uma armadilha." e a nota "Você caiu na armadilha…" no turno dele.
- Escolhas de projeto que não são regras do SRD: os 3 m são medidos entre centros de quadrados; o ataque da armadilha vai um a um às criaturas apanhadas; um teste ativo em penumbra não tem desvantagem.
- Veja [Arquitetura](../../architecture/maps/editor.md#layers).

#### Relacionadas
- [MR-010](historias.md#mr-010-gerar-masmorras), [MR-038](#mr-038-quebra-cabeças) (um movimento errado pode disparar uma armadilha do mapa).

### MR-036: Névoa de guerra pela visão

**Como** jogador, **quero** ver no mapa só o que o meu personagem enxerga, **para** que a exploração tenha o suspense da mesa.

- Prioridade: MVP
- Regras: RN-10
- Módulos: maps, play, rules

#### Critérios de aceite
- **Dado** um mapa com grade, **quando** o mestre liga "Ligar a névoa", **então** cada jogador vê só o que o próprio personagem enxerga agora ou já viu; o que foi visto fica escurecido, sem criaturas, até o mestre pedir "Esquecer o que foi visto" ou trocar a grade ou a imagem.
- **Dado** a luz de base do mapa ("Luz de base": Claro, Penumbra ou Escuro, escuro por padrão), **quando** ela é "Claro", **então** os jogadores veem a planta inteira, enquanto criaturas e pontos aparecem só onde os personagens deles enxergam.
- **Dado** uma fonte de luz (uma tocha, uma magia, um raio personalizado de até 36 m), **quando** o mestre a põe no mapa, **então** a área ao redor dela fica visível, cortada pelas paredes que o mestre pintou; o jogador vê a luz, nunca o ponto.
- **Dado** um personagem com 18 m de Visão no escuro na ficha, **quando** ele está numa área escura, **então** ele enxerga até 18 m, em tons de cinza.
- **Dado** uma luz que um personagem carrega, **quando** o jogador (ou o mestre) a define, **então** ela ilumina em volta desse personagem, e só o mestre e o dono ficam sabendo.
- **Dado** "Visão do grupo", **quando** ela está ligada, **então** todo jogador vê a união do que todos os personagens enxergam.
- **Dado** um personagem de jogador, **quando** outro jogador olha o mapa, **então** o token nunca some; o token de um NPC aparece só num quadrado que quem olha vê agora.
- **Dado** o mestre, **quando** ele abre o mapa, **então** vê tudo, e "Ver como" mostra o mapa como um personagem escolhido o vê. O que o jogador não vê nunca sai do servidor (RN-10): nem a imagem da parte não vista, nem os pontos, as camadas, as luzes ou os NPCs.
- **Dado** um combate num mapa com névoa, **quando** um jogador olha a ordem, o turno, os alvos, o registro e as ofertas de ataque de oportunidade, **então** vê só os NPCs que o personagem dele vê agora (com "Visão do grupo", os do grupo); personagens de jogadores e as criaturas deles nunca somem. O movimento é planejado com o terreno que o jogador conhece e cortado onde algo bloqueia, sem nomear parede nem criatura.
- **Dado** um druida em forma de lobo, **quando** ele está numa sala escura, **então** não vê nada além: a fera não tem visão no escuro, e os sentidos dela valem no lugar dos do personagem (SRD).
- **Dado** um mapa com névoa e luz de base "Escuro", **quando** um personagem de jogador sem visão no escuro está onde nenhuma luz alcança, **então** o mestre vê "No escuro" com o nome dele e os botões "Ver no mapa" e "Acender luz"; nenhum jogador vê o aviso, e ele some quando uma luz (a de um NPC escondido também) alcança a casa ou o personagem tem visão no escuro.
- **Dado** um jogador com um familiar no mesmo mapa, a 30 m ou menos, **quando** ele escolhe "Ver pelos olhos do familiar", **então** vê o que o familiar vê, com os sentidos dele (a visão no escuro da coruja), e o personagem fica cego e surdo até o jogador parar; em combate isso gasta a ação e termina no início do próximo turno do personagem. As outras criaturas aparecem no mapa, mas não enxergam por ninguém.
- **Dado** um mapa sem névoa, **quando** os jogadores o abrem, **então** ele funciona como sempre funcionou.

#### No app
- **Editor do mestre.** "Névoa de guerra" tem "Ligar a névoa" (precisa da grade), "Luz de base", "Visão do grupo" e "Esquecer o que foi visto", que pergunta no lugar; cada mudança vale na hora. "Luz" é um tipo de ponto com as fontes do SRD e "Personalizada". O mapa desenha dois anéis (luz forte contínua, penumbra tracejada) e avisa que as paredes só cortam a luz com a névoa ligada, o que "Ver como" mostra ("Você está vendo o mapa como Toren", "Voltar à sua vista").
- **Telas do jogador e do mestre.** O mapa com névoa mostra "Carregando o mapa" enquanto monta, "Seu personagem não está neste mapa", "Luz que você carrega"; o mestre tem "Luz dos personagens". O navegador desenha o que o servidor manda e conta só quadrados ("76 quadrados vistos"). O mestre também vê "No escuro: ..." sobre o mapa, e o selo "No escuro" em "Grupo", para personagens que não enxergam onde estão (sem luz e sem visão no escuro), com "Ver no mapa" e "Acender luz"; os jogadores nunca veem.
- **A imagem.** O jogador recebe a figura em pedaços feitos no servidor só com os quadrados vistos ou lembrados; duas imagens que diferem só em quadrados desconhecidos dão pedaços idênticos. Nas bordas da névoa a figura fica um pouco escurecida. Ver [Arquitetura](../../architecture/maps/fog.md#per-player-image-tiles).
- Ver [Arquitetura](../../architecture/maps/editor.md#layers) e [Arquitetura](../../architecture/play/creatures.md#wild-shape).

#### Relacionadas
- [MR-037](#mr-037-criaturas-do-personagem) (o familiar, a Forma Selvagem), [MR-034](#mr-034-movimentos-especiais) (paredes e camadas), [MR-035](#mr-035-armadilhas) (luzes e armadilhas como pontos).
- Regras escolhidas: as paredes que o mestre pinta bloqueiam a visão e a luz, como nas regras oficiais; um inimigo que o personagem não vê não aparece para ele, então atacar no escuro (até uma Bola de Fogo num canto suspeito) é decisão do mestre.

### MR-037: Criaturas do personagem

**Como** jogador, **quero** controlar uma criatura minha (a Forma Selvagem do druida, os mortos-vivos de Animar Mortos, um familiar), **para** jogá-la no combate com a ficha própria.

- Prioridade: MVP
- Regras: RN-02, RN-20
- Módulos: play, rules, characters

#### Critérios de aceite
- **Dado** um druida com Forma Selvagem, **quando** ele a usa, **então** o jogador recebe a ficha da criatura, tirada das criaturas do SRD, e age com ela no combate: CA, ataques, deslocamentos e sentidos da fera, PV da fera à parte e nenhuma magia. A lista de feras segue o nível (nível 2: ND até 1/4, sem voar nem nadar; 4: até 1/2, sem voar; 8: até 1). Usar a forma gasta um uso da Forma Selvagem e, em combate, a ação.
- **Dado** um druida em forma de fera, **quando** a fera cai a 0 PV, **então** ele volta à própria forma e o dano que sobrou passa para os PV dele; "Voltar à forma normal" é uma ação bônus, e a cura na forma cura a fera. O mestre pode encerrar a forma corrigindo os PV da fera para 0.
- **Dado** uma magia que convoca (Convocar Familiar, Animar Mortos, Conjurar Animais), **quando** o jogador a conjura, **então** as criaturas se juntam ao personagem: Convocar Familiar é um ritual que não gasta espaço de magia e um familiar novo substitui o antigo; Animar Mortos gasta o espaço, que decide quantos; Conjurar Animais oferece as quatro opções. Convocar Montaria não está incluída.
- **Dado** Conjurar Animais (ou Animar Mortos) em combate, **quando** as criaturas entram, **então** elas rolam uma iniciativa só e agem num turno conjunto (uma regra da casa para Animar Mortos, pois o SRD só diz isso de Conjurar Animais).
- **Dado** o mestre, **quando** ele dá uma criatura a um personagem, **então** qualquer uma das 334 criaturas do SRD pode ser dada; o jogador ou o mestre pode renomeá-la (até 40 caracteres) ou dispensá-la. Um personagem guarda no máximo 40 criaturas.
- **Dado** a criatura de um personagem, **quando** chega a vez dela, **então** ela entra na ordem do combate com a própria economia de ação, e o jogador a controla. O familiar não ataca, exceto o do Pacto da Corrente, com a reação dele.
- **Dado** a criatura de um personagem, **quando** ela sofre dano, **então** os PV dela são separados dos do personagem, mostrados como número só ao dono e ao mestre, e o mestre pode corrigi-los (RN-02). Com 0 PV ela é derrotada e dispensada.
- **Dado** que a concentração da conjuração termina, **quando** ela termina, **então** as criaturas dessa conjuração são dispensadas (RN-22); o desfazer do mestre as traz de volta.
- **Dado** que o mestre põe o token da criatura de um personagem num mapa, **quando** o grupo olha, **então** todos o veem (um token de grupo, nunca escondido, nem na névoa).
- **Dado** um jogador que não pode ter criatura, **quando** ele olha a ficha de outro personagem, **então** o painel "Criaturas" não existe para ele (RN-20).

#### No app
- **Ficha.** O painel "Criaturas" tem um cartão por criatura ("Corvo · Miúdo · Familiar de Pensantus", CA, PV, deslocamento em metros), "Ver a ficha do Nanquim", "Renomear", "Dispensar" (pergunta no lugar) e, para o mestre, "Corrigir PV" fora do combate. Convocar só é possível durante uma sessão ("Agora não há sessão aberta."). O mestre dá criaturas na lista de personagens da campanha com "Dar uma criatura".
- **Combate.** Um jogador que joga com mais de um combatente ganha abas na barra de turno e a página acompanha o turno. A página do turno das criaturas tem um bloco por criatura ("Atacar", "Mover o Lobo atroz 1") e "Encerrar a parte dos Lobos".
  A ordem do mestre mostra as criaturas como tokens redondos tracejados ("CA 14 · da Sálvia"), a caixa do turno conjunto e a etiqueta "Concentração"; "Perdeu a concentração" pergunta no lugar antes de dispensar. O jogador lê "Você perdeu a concentração em Conjurar Animais. Os 2 Lobos atrozes sumiram."
  A Forma Selvagem é "Transformar" na Ação ("restam 2 de 2 usos · volta no descanso curto ou longo"); o turno da fera tem o aviso "Na forma de Lobo", os dois conjuntos de PV e "Sem magias na forma de fera…". Quando a fera cai, o aviso "O Lobo caiu a 0 PV e você voltou à forma normal. 6 de dano passaram para você." fica até ser tocado.
- Ver [Design](../../design/sheet.md#editors), [Arquitetura](../../architecture/rules-creatures-items.md#creatures), [Arquitetura](../../architecture/play/creatures.md#what-the-combat-does-with-a-creature) e [Arquitetura](../../architecture/play/creatures.md#wild-shape).

#### Relacionadas
- [MR-036](#mr-036-névoa-de-guerra-pela-visão) (os olhos do familiar), [RN-02](regras.md), [RN-20](regras.md), [RN-22](regras.md).

### MR-038: Quebra-cabeças

**Como** mestre, **quero** criar quebra-cabeças que os jogadores resolvem no app, ao vivo, numa cena, **para** variar o ritmo da sessão.

- Prioridade: MVP
- Regras: RN-10, RN-18, RN-27
- Módulos: play, maps

#### Critérios de aceite
- **Dado** que sou o mestre de "Mirathel", **quando** crio um quebra-cabeça de um dos seis tipos ("Apagar as luzes", o cadeado de combinação, os símbolos giratórios, o enigma, a sequência para repetir ou a cifra), **então** ele é salvo na campanha e só eu o vejo; posso editá-lo até o primeiro "Mostrar", e arquivá-lo.
- **Dado** um "Apagar as luzes" de 5 por 5, **quando** o crio, **então** o servidor gera um começo que tem solução e nunca já vem resolvido, **e** vejo quantos toques bastam.
- **Dado** uma sessão aberta, **quando** o mestre mostra um quebra-cabeça, **então** os jogadores o veem e o resolvem ao vivo, todos no mesmo estado, com a pista que o mestre escreveu, **e** veem quem fez a última jogada (o nome do personagem).
- **Dado** um quebra-cabeça mostrado, **quando** dois jogadores jogam ao mesmo tempo, **então** as duas jogadas valem, e todos veem o novo estado. Uma jogada repetida depois de uma resposta perdida conta uma vez só.
- **Dado** um enigma, **quando** o jogador digita a resposta, **então** o servidor a compara com as respostas que o mestre aceita, ignorando maiúsculas, acentos e pontuação, **e** o jogador nunca recebe a resposta certa nem o quanto a tentativa chegou perto.
- **Dado** uma sequência para repetir (3 a 8 sinos, 3 a 12 passos), **quando** o mestre a toca, **então** os jogadores a assistem passo a passo, só com os passos já revelados, **e** depois a repetem; o servidor confere a ordem.
- **Dado** uma cifra, **quando** os jogadores a abrem, **então** veem a mensagem cifrada e a decifram com a chave que acharam como pista na aventura; o servidor a cifra e confere a mensagem decifrada.
- **Dado** um quebra-cabeça com dicas, **quando** o mestre libera a próxima dica, **então** os jogadores a veem; **quando** um jogador passa num teste de perícia contra a CD que o mestre definiu (Investigação, Arcanismo...), **então** recebe a próxima dica, só para ele (RN-18: no app ou com o dado físico). A CD nunca aparece.
- **Dado** um quebra-cabeça de informação dividida (até 8 partes, cada uma para um personagem de jogador), **quando** os jogadores o abrem, **então** cada um vê só a parte dele da pista, **e** eles precisam conversar para resolver.
- **Dado** um quebra-cabeça com consequências, **quando** acontece uma jogada errada (uma combinação ou resposta errada), **então** acontece o que o mestre escolheu em "Ao errar": uma armadilha do mapa dispara (MR-035), uma tentativa do jogador é gasta (1 a 10 para cada um), ou o limite de jogadas ou de minutos se aproxima; ao fim do limite o quebra-cabeça para e o mestre é avisado. A armadilha e a tentativa existem só para os três tipos que julgam uma jogada, e respostas erradas simultâneas são contadas com exatidão.
- **Dado** um quebra-cabeça mostrado, **quando** os jogadores o resolvem, **então** o servidor confere a solução (o jogador nunca recebe a resposta), o quebra-cabeça para, o mestre é avisado **e** acontece o que ele escolheu em "Ao resolver": só o aviso (padrão), abrir uma porta (RN-26), revelar um ponto do mapa ou revelar uma pista da cena a quem resolveu (RN-27).
- **Dado** um quebra-cabeça mostrado, **quando** o mestre o recomeça ou o fecha, **então** os jogadores veem o mesmo começo de novo, ou deixam de vê-lo; "Gerar outro começo" ("Apagar as luzes" e símbolos giratórios) é uma ação separada do mestre.
- **Dado** um jogador, **quando** o servidor responde sobre um quebra-cabeça, **então** a resposta nunca traz as respostas aceitas, o próximo passo da sequência, a mensagem sem cifra, a chave, uma CD nem a parte de outro jogador (RN-10).

#### No app
- **Mestre, lista e editor.** "Quebra-cabeças" na aba "Preparação" da página da campanha lista os quebra-cabeças com seus estados, "Editar", "Arquivar" (perguntado na linha; o que foi mostrado fica como histórico da sessão) e "Mostrar os arquivados". "Novo quebra-cabeça" e "Editar quebra-cabeça" têm o formulário de cada tipo, a pista, as dicas, "Dica por teste de perícia", "Informação dividida", "Ao errar" (um jeito por vez) e "Ao resolver". Cada recusa aparece no campo a que se refere.
- **Mestre, na sessão.** "Mostrar aos jogadores" e "Ver ao vivo" mostram o quebra-cabeça como os jogadores o têm, a última jogada e quem a fez, as dicas ("Mostrar a próxima dica"), "Gerar outro começo", "Recomeçar" e "Fechar", perguntados na própria tela. Só o mestre lê, com "Só você vê", as respostas aceitas, os passos da sequência, a mensagem às claras, as tentativas que sobram, os contadores e quem ganhou uma dica por perícia.
- **Jogador.** O aviso "O mestre mostrou um quebra-cabeça" abre o quebra-cabeça na página da sessão. O enigma responde "Não é isso." com "Tentativas restantes 2 de 3"; a sequência diz "Errou o passo 4. A tentativa recomeçou; a Lia errou."; os contadores dizem "Jogadas 7 de 10" e "Tempo 4:48 de 5:00"; "Tentar uma dica · Investigação" dá "Você conseguiu. Esta dica é só sua" ou "Não deu desta vez." O painel 7 × 7 cabe numa tela de 320 × 568 px.
- Ver [Arquitetura](../../architecture/play/puzzles.md), [RN-27](regras.md) e [Design](../../design/session.md#puzzles).

#### Relacionadas
- [MR-047](#mr-047-mais-quebra-cabeças): as ideias que ficam para depois do MVP.
- [MR-035](#mr-035-armadilhas) (a armadilha que uma jogada errada pode disparar), [RN-10](regras.md), [RN-18](regras.md), [RN-26](regras.md), [RN-27](regras.md).

### MR-039: Imagens geradas para masmorras e cenas

**Como** mestre, **quero** gerar uma imagem a partir de uma masmorra, de um mapa ou da descrição de uma cena (arte da cena, uma vista isométrica do mapa, ou o próprio mapa com textura, combinando com a grade) e pedir ajustes, **para** mostrar à mesa o lugar que estou descrevendo.

- Prioridade: MVP
- Regras: RN-10, RN-28
- Módulos: maps (galeria)

#### Critérios de aceite
- **Dado** uma masmorra gerada ([MR-010](#mr-010-gerar-masmorras)) ou a descrição de uma cena, **quando** o mestre pede uma imagem com o próprio texto e estilo, **então** o app gera a imagem e a guarda na galeria da campanha, escondida dos jogadores. Só o mestre gera.
- **Dado** uma imagem gerada, **quando** o mestre escreve um novo pedido ("mais escura", "com uma ponte"), **então** o app a edita conhecendo a cena, sem recomeçar, **e** guarda a nova ao lado da anterior.
- **Dado** que o limite mensal da campanha (200 imagens em produção) foi atingido, **quando** o mestre pede outra imagem, **então** o app recusa e diz por quê e quando o limite zera. O servidor tem um limite diário próprio.
- **Dado** que o serviço recusa um pedido, não devolve imagem ou falha, **quando** o mestre espera a imagem, **então** o app avisa em português ("O serviço não gerou esta imagem") sem gastar a vaga do mês.
- **Dado** que o serviço devolveu a imagem mas o servidor não conseguiu guardá-la, **quando** o mestre espera a imagem, **então** o app avisa que a imagem foi feita mas não deu para guardá-la ("A imagem foi feita, mas não deu para guardá-la"), e não que o serviço não respondeu, **e** a tentativa não conta no mês. Um pedido que saiu e estourou o tempo sem imagem continua contando, porque pode ter sido cobrado.
- **Dado** um mapa com grade (gerado ou desenhado), **quando** o mestre pede "O mapa com textura", **então** o app gera o próprio mapa visto de cima, com piso e paredes no lugar, recortado e ajustado para combinar com a grade, **e** o mestre pode defini-lo como imagem do mapa ("Usar como imagem do mapa") sem apagar as camadas nem o que os jogadores já viram. Um mapa cuja imagem passa de 16 megapixels não pode virar mapa com textura, e a tela avisa antes de pedir.
- **Dado** um mapa, **quando** o mestre pede "Vista isométrica", **então** o app gera a arte do mapa em perspectiva isométrica a partir do que os jogadores veem agora, **e** ela vai para a galeria como arte da cena.
- **Dado** uma arte da cena ou uma vista isométrica feita a partir de um mapa, **quando** o app monta o desenho de referência, **então** ele leva só o que os jogadores veem agora: os quadrados que algum personagem vê, as criaturas que eles veem e a porta secreta não revelada como parede; as outras salas e os inimigos escondidos ficam de fora, e a imagem não mostra o que ainda não descobriram. Num mapa sem névoa, vai o mapa inteiro, menos o que está escondido dos jogadores.
- **Dado** o mapa com textura, **quando** o app monta o desenho de referência, **então** ele parte do mapa inteiro, com a porta secreta não revelada como parede (a névoa já esconde o mapa de cada jogador), **e** a imagem não mostra a passagem (RN-10, RN-26).
- **Dado** um mapa com tokens, um combate ou NPCs na cena, **quando** o mestre pede arte da cena ou uma vista isométrica, **então** ele escolhe quais NPCs e inimigos aparecem (os retratos da galeria vão como referências), limitados aos que os jogadores veem agora, **e** só esses aparecem; ele também pode escolher outras imagens da galeria como referências (até 10 imagens de objetos e 4 de personagens). O mapa com textura não leva criaturas nem imagens de personagens. Nenhum NPC é oferecido enquanto um combate roda no mapa.
- **Dado** um pedido de imagem, **quando** o app o envia ao serviço de IA, **então** vão só o desenho do mapa, a lista de salas (só para o mapa com textura de uma masmorra gerada), o texto do mestre e as referências que ele escolheu, nunca dados pessoais (RN-28).
- **Dado** uma imagem que mostra o mapa inteiro (o mapa com textura e cada ajuste dele), **quando** o mestre tenta mostrá-la aos jogadores, **então** o app pergunta antes ("Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.") e nunca a oferece com um toque só (RN-10).
- **Dado** uma imagem gerada, **quando** ela recebe um nome, **então** o nome é um que os jogadores entendem: o do mestre, ou o nome do mapa e o jeito (só para um mapa que os jogadores veem), ou o jeito e o dia ("Arte da cena · 06/10"), ou o nome da imagem ajustada mais " (ajuste)"; nunca "Imagem N", o texto do pedido nem o nome de algo escondido (RN-10).

#### No app
- **"Gerar imagem".** Um diálogo (uma folha inferior no celular) aberto a partir do mapa, de um ponto de cena e da galeria. Ele oferece os três jeitos (os que não cabem ficam tracejados, com o motivo), mostra o desenho que o servidor monta, "Quem aparece na imagem", as referências da galeria, o que vai para o Google sob cada campo e a contagem do mês. Enquanto espera, oferece "Cancelar" e "Parar de esperar"; o resultado tem "Mostrar aos jogadores" ([MR-028](#mr-028-mostrar-uma-imagem-aos-jogadores)), "Pedir um ajuste" e a cadeia de ajustes.
- **As camadas continuam sendo a verdade.** O modelo devolve uma de 10 proporções fixas, então o servidor completa o desenho do mapa com rocha, recorta o mapa de volta e o ajusta à grade. Se a imagem se afastar um pouco de uma parede, o mestre gera de novo.
- O seletor de imagens da sessão ("Mostrar uma imagem aos jogadores") marca "Mapa inteiro".
- Ver [Arquitetura](../../architecture/maps/image-generation.md), [imagens feitas a partir de um mapa](../../architecture/maps/image-generation.md#images-made-from-a-map), [Design](../../design/session.md#generated-images), [Privacidade](../privacidade.md#o-que-as-imagens-geradas-mandam-ao-google-mr-039-rn-28) e [Operação](../../operations/reference.md#generated-images-the-gemini-api).

### MR-040: Subir de nível pela ficha

**Como** jogador, quando o meu personagem pode subir de nível, **quero** editar a ficha para acrescentar só o que o próximo nível dá, **para** não esperar o mestre aplicar o nível por mim.

- Prioridade: MVP
- Regras: RN-01, RN-12, RN-33
- Módulos: characters, rules

#### Critérios de aceite
- **Dado** um personagem que "Pode subir de nível" (RN-12), **quando** o jogador abre a ficha travada, **então** só o dono vê o bloco "Pensantus pode subir de nível" com o motivo e o botão "Subir para o nível 4"; sem essa marca a ficha continua só de leitura (RN-01) e a rota de subir de nível responde como a uma ficha travada.
- **Dado** o subir de nível guiado, **quando** o jogador o usa, **então** ele acrescenta um nível de classe e só as escolhas desse nível, em passos (Habilidades, Vida, Magias, Resumo, e "Escolhas" para a subclasse, as opções de características, as perícias e a aptidão); um passo sem escolha não existe. Nada é guardado até "Confirmar o nível 4".
- **Dado** o subir de nível guiado, **quando** o jogador tenta mudar qualquer outra coisa na ficha, **então** o servidor recusa: o resto continua travado.
- **Dado** uma edição que as regras de D&D 5e não permitem, **quando** o jogador a envia, **então** o motor de regras a recusa, dizendo o campo e o motivo.
- **Dado** o dado de vida de um nível, **quando** o jogador rola no app, **então** o servidor rola e guarda o resultado, e rolar de novo devolve o mesmo; numa campanha que força dados físicos, o app não rola e o jogador digita o resultado (RN-18). Uma mesa que permite só a média não tem dado.
- **Dado** o nível de um Bruxo, **quando** o número ou o nível dos espaços de pacto muda, **então** o resumo lista "Espaços do pacto" (por exemplo "1 de 1º nível → 2 de 1º nível").
- **Dado** um personagem cujos pontos de vida atuais estão definidos, **quando** o subir de nível aumenta o máximo, **então** o atual sobe na mesma quantidade: um ferimento continua sendo um ferimento e quem está no máximo continua no máximo; quem nunca teve os pontos de vida definidos continua cheio (RN-12).
- **Dado** que o jogador subiu de nível, **quando** a ficha é salva, **então** ela mostra o novo nível, "Pensantus subiu para o nível 4. O mestre foi avisado.", e "Pode subir de nível" some sozinho.
- **Dado** que o jogador subiu de nível, **quando** o mestre abre a campanha, **então** vê "Subiu para o nível N" e um aviso no topo (por 24 horas, ou até ser dispensado), e "O que mudou" abre as escolhas do jogador nesse nível (habilidade, pontos de vida e como, truques, magias, preparadas), com a hora. Não há aprovação nem veto: o mestre continua editando a ficha como sempre (RN-02).
- **Dado** um personagem que pode subir de nível, **quando** o jogador abre o subir de nível guiado com mais de uma classe disponível, **então** o primeiro passo é "Subir em qual classe?": as classes que o personagem tem ("Guerreiro, nível 5 → 6") e "Uma classe nova", que abre as outras classes com o que cada uma pede e o que o personagem tem; uma classe para a qual o personagem não se qualifica fica na lista, tracejada, com "Falta: Carisma 13 (você tem 9)." (SRD 5.1, "Multiclassing").
- **Dado** uma classe nova, **quando** o jogador confirma o resumo, **então** o rodapé primeiro pergunta "Subir em Mago 1? Isso acrescenta uma classe nova à ficha…" com "Voltar" em foco; a classe entra no nível 1 com os pontos de vida médios do dado dela, só com as proficiências da tabela de multiclasse (uma perícia, e o instrumento do Bardo, são escolhidos) e sem equipamento, e o resumo diz por que cada número mudou (nível total, bônus de proficiência, dados de vida à parte, espaços de magia pelo nível de conjurador).
- **Dado** uma classe cuja habilidade principal o personagem não tem, ou um personagem cuja classe atual não tem a própria, **quando** o jogador a pede, **então** o servidor recusa com a classe, a habilidade, o mínimo e o valor; o editor do mestre pode passar por cima, e a criação de uma ficha assim pelo jogador também é recusada.
- **Dado** um jogador que cria um personagem cuja classe ou raça pede escolhas, **quando** ele chega ao passo "Escolhas", **então** cada escolha lista as opções com a regra em uma linha, as ainda indisponíveis pontilhadas com o motivo, e "Criar personagem" diz o que ainda falta (RN-33).
- **Dado** uma raça, um traço ou um antecedente que deixa o jogador escolher idiomas (o idioma extra do Meio-elfo, os dois do Acólito), ou um traço, antecedente ou característica que deixa escolher ferramentas (as ferramentas de artesão do Anão, "um tipo de ferramenta de artesão" de um antecedente), **quando** o jogador chega ao passo "Escolhas", **então** cada um é uma escolha entre os idiomas ou as ferramentas do catálogo, uma de cada, com as que o personagem já tem pontilhadas e o motivo, e as escolhas aparecem nos "Idiomas" e nas "Proficiências" da ficha; o campo livre "Idiomas" fica para os do mestre, e o que está escrito ali também aparece na ficha. Enquanto uma está aberta, a ficha diz "Falta escolher N idiomas: faça a escolha no passo “Escolhas”" e o salvar não é impedido (RN-33).
- **Dado** uma ficha travada com escolhas em aberto, **quando** o dono a abre, **então** ela diz "N escolhas pendentes" e "Completar" leva a uma página só com elas; o mestre vê "N escolhas em aberto" na campanha (RN-33).
- **Dado** um subir de nível, **quando** o personagem tem escolhas que ficaram para trás, **então** "Escolhas que ficaram para trás" vem antes de "Próximo", e uma invocação que pede algo que a ficha não tem fica pontilhada com o motivo (RN-33).

#### No app
- Os números de "O que muda" e do resumo vêm do servidor, que também lista os ataques cujo acerto ou dano o nível move. O dado de vida usa o mesmo seletor de rolagem do combate e da cena (RN-18).
- "Cancelar" e "Voltar para a ficha" perguntam no lugar antes de descartar. Uma ficha desatualizada ou uma recusa do motor aparece no lugar, com o motivo, e "Ler a ficha de novo" mantém as escolhas que ainda cabem.
- Ver [Arquitetura](../../architecture/characters.md#leveling-up), [Design](../../design/session.md#experience-and-level-up) e [RN-01](regras.md).

#### Relacionadas
- [MR-017](#mr-017-subir-de-nível) (a tela completa de subir de nível, depois do MVP), [RN-01](regras.md), [RN-12](regras.md).

### MR-041: Tesouros e XP por ouro

**Como** mestre, **quero** pôr tesouros no mapa antes da sessão e converter o que o grupo achou em XP, **para** dar XP por ouro sem fazer a conta na hora.

- Prioridade: MVP
- Regras: RN-09
- Módulos: maps, progression

#### Critérios de aceite
- **Dado** um mapa, **quando** o mestre põe um tesouro (um baú, por exemplo) com um valor em PO (um número inteiro, de 0 a 1.000.000) e uma descrição, **então** o tesouro é guardado no mapa, escondido como qualquer outro ponto (RN-10).
- **Dado** um tesouro que o grupo achou, **quando** o mestre o marca como encontrado por um ou mais personagens, **então** todos que veem o mapa o veem, com a descrição, o valor e quem o achou, e ele conta como encontrado e ainda não convertido.
- **Dado** um tesouro marcado por engano, **quando** o mestre tira a marca, **então** ele volta a ficar escondido, a menos que já tenha sido convertido.
- **Dado** uma sessão aberta, **quando** um tesouro é marcado ou desmarcado, **então** ele conta no resumo dessa sessão e os jogadores leem "Brisa encontrou o Baú de moedas."; um tesouro marcado fora de uma sessão não conta em nenhum resumo, e o editor avisa.
- **Dado** tesouros encontrados numa campanha por ouro, **quando** o mestre usa "Voltar à cidade", **então** o app os converte em XP, 1 XP por PO (RN-09), dividido igualmente entre os personagens que o mestre escolhe (o resto se perde e a tela diz quanto), e cada tesouro é convertido uma vez só, mesmo que duas concessões disputem.
- **Dado** um tesouro convertido, **quando** o mestre tenta mudar o valor, o tipo ou a marca dele, ou excluí-lo, **então** o app recusa até a concessão ser desfeita.
- **Dado** que a última concessão foi um "Voltar à cidade", **quando** o mestre a desfaz, **então** os tesouros voltam a "encontrado, não convertido" e podem ser convertidos de novo; depois de uma concessão mais nova, a conversão deixa de ser a última, então só pode ser desfeita depois que as mais novas forem.
- **Dado** uma campanha que dá XP por inimigos ou por marcos, **quando** um tesouro é encontrado, **então** ele aparece no resumo da sessão e nunca é convertido: não há "Voltar à cidade".
- **Dado** que sou jogador, **quando** leio o histórico, **então** vejo só quantos tesouros foram convertidos e o ouro, nunca quais.

#### No app
- Editor: "Tesouro" na barra de pontos, com "Descrição para os jogadores", "Valor em ouro", "Marcar como encontrado" e "Desmarcar".
- Campanha por ouro: em "Experiência", na aba "Mesa", a faixa "Encontrado, ainda não convertido" e "Voltar à cidade" ao lado de "Dar XP".
- O diálogo "Voltar à cidade" lista os tesouros encontrados (no máximo 100 por vez) e mostra a conta antes de o mestre confirmar.
- O mestre ainda pode digitar as PO em "Dar XP por ouro" ([MR-016](#mr-016-dar-xp)).

#### Relacionadas
- Destaque "Mais tesouro encontrado": [MR-032](#mr-032-destaques-do-combate). Armadilhas e baús: [MR-035](#mr-035-armadilhas). Ver [Arquitetura](../../architecture/maps/editor.md#layers).

### MR-042: Bestiário

**Como** mestre, **quero** pesquisar as criaturas do SRD e pôr uma no combate, **para** montar um encontro sem fazer uma ficha para cada inimigo.

- Prioridade: MVP
- Regras: RN-20, RN-29
- Módulos: rules, play, characters

#### Critérios de aceite
- **Dado** que sou o mestre de "Mirathel", **quando** abro o "Bestiário", **então** vejo as 334 criaturas do SRD com nome em português, e pesquiso pelo nome em português ou em inglês e filtro por tipo, tamanho e ND.
- **Dado** uma criatura do bestiário, **quando** a abro, **então** vejo a ficha dela (CA, PV, deslocamento, habilidades, ataques e ações), com o texto do SRD em inglês.
- **Dado** um combate sendo preparado ou em andamento, **quando** ponho três Goblins do bestiário (de 1 a 10 por vez, nunca mais de 40 combatentes), **então** eles entram como NPCs numerados, escondidos por padrão, cada um com a própria iniciativa (RN-19) e PV médios, ou rolados se eu pedir, **e** o jogador vê só a palavra do estado deles (RN-20, RN-29).
- **Dado** que nenhum combate está aberto, **quando** ponho monstros do bestiário, **então** o app inicia um com eles e o grupo, com um mapa ou no teatro da mente, como em "Iniciar combate".
- **Dado** um combate com monstros do bestiário, **quando** ele termina, **então** o XP por inimigos conta o ND de cada um, como o de um NPC.
- **Dado** uma criatura do bestiário, **quando** o mestre escolhe "Criar NPC", **então** o app faz uma ficha básica de NPC (um capanga ou um NPC da história) com os números dela e até três ataques, que o mestre pode renomear e editar; o bestiário não muda.
- **Dado** uma criatura com vários ataques por ação ou com conjuração, **quando** leio o NPC feito a partir dela, **então** a descrição diz isso em português ("Ataques por ação: 2 (ataque múltiplo)"; a CD de resistência da magia, o bônus de ataque e as magias); o NPC conjura essas magias sozinho, com a CD de resistência e o bônus de ataque da criatura, e os espaços e usos por dia dele são contados (só o mestre os vê); um NPC feito à mão leva a contagem para o próximo combate até o descanso longo da mesa, e um monstro em combate começa cada luta com todos eles; um NPC feito a partir da criatura também guarda até lá os usos por dia das ações dela e os usos da Resistência Lendária.
- **Dado** um monstro com Ataques múltiplos no turno dele (a armadura animada: duas pancadas), **quando** abro o cartão de ataque, **então** ele mostra "Ataques múltiplos: 2 × Pancada" e faço os ataques da rotina um depois do outro, cada um com o próprio alvo; depois do primeiro a ação diz "1 de 2" (não "Usada"), o segundo é oferecido, e a ação conta como usada quando a rotina termina ou quando escolho "Encerrar os ataques" (uma escolha da página, que o servidor não guarda: depois de recarregar, o resto é oferecido de novo); um terceiro ataque é recusado, o registro diz cada ataque, e "Desfazer última ação" desfaz o último.
- **Dado** um monstro cujos Ataques múltiplos têm várias rotinas ou uma escolha entre elas, **quando** abro o cartão de ataque, **então** escolho a rotina a fazer; uma criatura sem Ataques múltiplos mantém o cartão e o turno como antes.
- **Dado** uma criatura com Resistência Lendária (3/dia) cujo teste de resistência eu rolo como mestre (o de um efeito ou o de concentração) e a rolagem falha, **quando** peço a Resistência Lendária junto com a rolagem, **então** o teste de resistência vira sucesso e um uso é gasto, o registro me diz a rolagem e a escolha ("restam 2"), e os jogadores leem só que ela teve sucesso; uma rolagem que tem sucesso não gasta nada, e uma criatura sem a característica ou sem usos é recusada sem nada mudar.
- **Dado** que um jogador lança uma magia com teste de resistência numa criatura com Resistência Lendária e um uso sobrando, **quando** o lançamento espera por mim (o teste de resistência dela passando ou falhando), **então** vejo a rolagem que falhou e "Usar Resistência Lendária (restam K)" para cada falha, e nada a oferecer quando todos passaram; as que escolho viram sucessos com os mesmos dados e gastam um uso cada, as outras ficam como estão, e os jogadores esperam em "Esperando o mestre" e depois leem o resultado comum do teste de resistência, o mesmo tenha ele passado ou falhado; nenhuma janela abre para uma criatura sem a característica ou sem usos, e "Desfazer última ação" no lançamento devolve os usos.
- **Dado** que o ataque corpo a corpo com arma de um jogador acerta uma criatura cuja ficha tem Aparar (Cavaleiro, Capitão Bandido...) e ela não usou a reação na rodada, **quando** o acerto espera por mim, **então** vejo "Aparar (+N CA)" com a rolagem e a classe de armadura; se uso, a rolagem guardada é comparada com a classe de armadura mais N e um acerto que fica abaixo vira erro, se deixo passar o acerto vale, e os jogadores esperam em "Esperando o mestre" e depois leem um acerto ou erro comum, sem nunca saber da reação, do bônus nem da classe de armadura; a reação volta no turno da criatura, um segundo acerto na rodada não abre janela, e "Desfazer última ação" no Aparar devolve a reação.
- **Dado** uma criatura cuja reação o app não resolve (Dividir, Grito, Pegar Pedra...), **quando** leio a ficha dela, **então** leio a reação como texto e nenhuma janela abre para ela.
- **Dado** uma criatura com ações lendárias (um dragão vermelho adulto: Detect, Tail Attack, Wing Attack), **quando** outra criatura tem o turno e gasto uma, **então** o custo sai das 3 que ela tem na rodada e os jogadores leem só a linha comum de ataque ou de movimento; não posso gastar no turno dela, enquanto está incapacitada, além do que resta ou enquanto uma reação espera resposta, as ações voltam no turno dela, uma opção que o app não resolve é oferecida como texto e conta do mesmo jeito, e "Desfazer última ação" devolve o custo.
- **Dado** que sou jogador, **quando** procuro o bestiário, **então** não acho nenhuma entrada, e nunca descubro qual criatura é um monstro, o ND dela nem os PV rolados.

#### No app
- A linha "Bestiário" da aba "Preparação" da página da campanha (só o mestre), uma lista com busca e filtros, e uma página por criatura com "Criar NPC" e "Pôr no combate" (a ficha pergunta "Quantos", o nome base, PV "Média" ou "Rolar" e "Escondidos no início").
- Na sessão, a ordem do mestre mostra o ND do monstro e o cartão do turno leva a "Ficha da criatura".

#### Relacionadas
- [MR-043](#mr-043-gerar-encontros), [MR-044](#mr-044-gerar-tesouro), [RN-20](regras.md), [RN-29](regras.md). Monstros próprios não fazem parte do MVP. Ver [Design](../../design/combat.md#monsters-creatures-and-encounters).

### MR-043: Gerar encontros

**Como** mestre, **quero** montar um encontro sabendo se ele é fácil ou mortal para o meu grupo, e gerar um quando estou sem ideias, **para** preparar a sessão mais rápido.

- Prioridade: MVP
- Regras: RN-29
- Módulos: rules, play, maps

#### Critérios de aceite
- **Dado** o grupo de "Mirathel" (os personagens de jogadores vivos da campanha, mais os NPCs que ponho no grupo no nível que eu disser), **quando** monto um encontro com criaturas do bestiário e quantidades, **então** vejo o XP total e a dificuldade ("Baixa", "Moderada", "Alta" ou "Acima de alta") contra o orçamento do grupo, com o rótulo "Guia de dificuldade do SRD 5.2.1 (regras de 2024)" e o aviso de que, com os monstros de 2014, o encontro tende a ser um pouco mais fácil.
- **Dado** um encontro montado, **quando** ele tem uma criatura acima do menor nível do grupo mais 3, um grupo vazio, ou mais criaturas do que um combate comporta, **então** leio um aviso.
- **Dado** um encontro montado, **quando** o guardo num ponto de batalha no mapa, **então** "Começar este combate" põe os monstros dele no combate, como em [MR-042](#mr-042-bestiário), com o modo de PV e a escolha de esconder, com um mapa ou no teatro da mente, e posso acrescentar no mesmo diálogo os NPCs próprios da campanha (um chefe, um vilão com nome), em "NPCs da campanha".
- **Dado** um ponto de batalha que já guarda um encontro, **quando** guardo outro, **então** o app pergunta antes de substituí-lo, e "Tirar o encontro do ponto" o remove.
- **Dado** uma dificuldade e, se eu quiser, um tipo de criatura, **quando** peço "Gerar encontro", **então** o app monta um com criaturas do SRD, um líder e um grupo, que nunca passa do orçamento nem traz uma criatura com ND acima do menor nível do grupo mais 3 (os NPCs do grupo contam, no nível que o mestre deu); "Gerar outro" faz um novo, e "Trocar criatura" troca uma criatura por outra do mesmo XP.
- **Dado** a mesma dificuldade, as mesmas opções e a mesma semente, **quando** gero duas vezes, **então** o resultado é o mesmo.
- **Dado** que sou jogador, **quando** abro o montador ou um ponto de batalha, **então** leio "Só o mestre monta encontros." e nunca recebo o encontro nem o que ele guarda (RN-10).

#### No app
- O painel "Encontros" da aba "Preparação" da página da campanha (só o mestre), com "Montar encontro", leva ao montador: o grupo em chips ("Pôr um NPC no grupo", nível 1 a 20), uma barra com as três faixas, a lista de criaturas com busca, e os botões de gerar, trocar e guardar.
- Os números vêm da tabela "XP Budget per Character" do SRD 5.2.1 (CC BY 4.0), creditada na página "Créditos". O SRD 5.1 não tem tabela de dificuldade; se ela serve para os monstros de 2014, cabe à mesa julgar depois de jogar.
- Na sessão, um ponto de batalha que guarda um encontro mostra a dificuldade de hoje e "Começar este combate"; a partir de um ponto de batalha num mapa com grade, o diálogo abre em "Com mapa".

#### Relacionadas
- [MR-042](#mr-042-bestiário), [RN-29](regras.md). Ver [Arquitetura](../../architecture/play/encounters.md#the-encounter-builder).

### MR-044: Gerar tesouro

**Como** mestre, **quero** gerar o tesouro de uma criatura ou de um covil, com moedas, gemas, objetos de arte e itens mágicos, **para** pô-lo no mapa sem inventar tudo na hora.

- Prioridade: MVP
- Regras: RN-09, RN-10
- Módulos: rules, maps, progression, characters

#### Critérios de aceite
- **Dado** o nível do grupo (por padrão o menor nível entre os personagens vivos, que pode ser mudado de 1 a 20), **quando** peço um tesouro "Individual" ou "De covil", **então** o app gera as moedas (individual) ou as moedas, gemas, objetos de arte e os itens mágicos do SRD 5.1 (covil), a partir de tabelas próprias em português, com o nome em português e o valor em PO. Um artefato nunca sai.
- **Dado** um item mágico gerado, **quando** abro "Ver descrição", **então** vejo a raridade, o valor com o rótulo "Valores do SRD 5.2.1 (regras de 2024)" e o texto do SRD em português, com "Ver em inglês" para o original. Um consumível vale metade do valor da raridade; um pergaminho de magia vale o valor inteiro.
- **Dado** um tesouro gerado, **quando** o ponho no mapa ("Pôr no mapa"), **então** ele vira um ponto de tesouro escondido (RN-10) com o ouro (moedas, gemas e arte) como valor em PO e tudo o que há no tesouro, em português, na descrição. Itens mágicos nunca viram XP. Numa campanha por ouro, o ouro vira XP em "Voltar à cidade" ([MR-041](#mr-041-tesouros-e-xp-por-ouro)).
- **Dado** um tesouro que já pus no mapa, **quando** aperto o botão de novo por engano, **então** nenhum segundo ponto é feito; só "Gerar outro" traz o botão de volta.
- **Dado** um mapa sem grade, um quadrado fora da grade, ou um mapa com 200 pontos, **quando** tento pôr um tesouro, **então** o app recusa e diz por quê.
- **Dado** o mesmo pedido e a mesma semente, **quando** gero duas vezes, **então** o resultado é o mesmo, desde que o conteúdo da mesa não tenha mudado; se mudou, o app me pede para gerar de novo.
- **Dado** que sou jogador, **quando** procuro a página de tesouro, **então** leio "Só o mestre gera o tesouro da campanha." e nunca vejo um tesouro posto até ele ser marcado como encontrado.

#### No app
- "Gerar tesouro" no painel "Mapas" da aba "Preparação" da página da campanha. O resultado também diz o que a campanha faz com o ouro (por inimigos e por marcos ele não é XP; por ouro, converte em "Voltar à cidade").
- O SRD 5.1 não tem valores de itens nem tabelas de tesouro aleatório: os valores vêm do SRD 5.2.1 e as tabelas de moedas, gemas e objetos de arte são nossas. Ver [Arquitetura](../../architecture/maps/treasure.md#treasure-generator-mr-044) e [Conteúdo de regras](../../rules-content.md).

#### Relacionadas
- [MR-041](#mr-041-tesouros-e-xp-por-ouro), [MR-042](#mr-042-bestiário), [MR-043](#mr-043-gerar-encontros). Itens mágicos próprios não fazem parte do MVP.

### MR-045: Consultar as magias

**Como** jogador, **quero** consultar todas as magias da mesa no app, **para** ler o que cada uma faz antes de escolher e durante a sessão.

- Prioridade: MVP
- Regras: RN-23
- Módulos: characters, rules

#### Critérios de aceite
- **Dado** que sou jogador de "Mirathel", **quando** abro "Magias", **então** vejo todas as magias disponíveis na mesa (as do SRD e as da mesa que o mestre deixou ligadas), com busca pelo nome (em português ou em inglês, sem importar os acentos) e filtros por classe, nível da magia e escola, **e** cada uma abre a descrição completa, com o alvo ("Cone de 4,5 m").
- **Dado** uma magia que o mestre desligou em "Opções para os jogadores", **quando** a procuro, **então** ela não aparece, a menos que esteja na minha ficha.
- **Dado** uma magia que o mestre desligou ou arquivou enquanto eu lia a lista, **quando** a sessão está aberta, **então** a lista se atualiza sem recarregar e o cartão aberto diz "Esta magia não está disponível.".
- **Dado** o meu personagem, **quando** ligo "Só as que posso aprender", **então** vejo só as magias da lista da minha classe até o nível que conjuro; numa ficha multiclasse, as de cada classe, pelo nível nela. O mestre não tem essa chave.
- **Dado** uma magia da mesa, **quando** a abro, **então** ela leva a etiqueta "Da mesa" e o texto do mestre; uma magia do SRD leva a tradução em português, com "Ver em inglês" e "Créditos". Uma magia arquivada chega só ao mestre, com "Arquivada".
- **Dado** um membro pendente, **quando** ele pede a lista, **então** não recebe nada: só membros ativos a leem.
- **Dado** a busca, os filtros e uma magia aberta, **quando** compartilho ou recarrego a página, **então** eles estão no link, e Voltar fecha a magia e traz a mesma lista de volta.

#### No app
- `/campaigns/:id/spells`, a partir do painel "Magias" da aba "Regras" do mestre (e da página da campanha de um jogador), para todo membro ativo, o mestre também. Três colunas a partir de 1100 px; abaixo disso, uma coluna, com "Filtros (n)".

#### Relacionadas
- Uma parte da [MR-020](#mr-020-consultar-o-livro-de-regras), só com as magias. Deixar o mestre aceitar uma magia fora da lista da classe fica para depois do MVP ([MR-046](#mr-046-o-estilo-da-mesa-recurso-por-recurso)). Conteúdo da mesa: [MR-025](#mr-025-cadastrar-conteúdo-da-mesa).

### MR-048: Conjurar magias fora do combate

**Como** jogador, **quero** conjurar minhas magias entre as lutas, **para** curar o grupo, preparar Armadura Arcana ou conjurar um ritual sem abrir um combate. **Como** mestre, **quero** confirmar que o tempo de uma conjuração longa passou, **para** que o espaço seja gasto e a magia faça efeito quando a história disser.

- Prioridade: MVP
- Regras: RN-32, RN-10, RN-18, RN-20
- Módulos: play, characters, rules

#### Critérios de aceite
- **Dado** um clérigo com um espaço livre e um aliado ferido, **quando** o jogador conjura Curar Ferimentos com "Conjurar" e escolhe o aliado, **então** o espaço é gasto, uma rolagem cura o aliado até o máximo, e o grupo vê a conjuração no registro.
- **Dado** um mago com Detectar Magia no grimório, **quando** o jogador a conjura "Como ritual", **então** nenhum espaço é gasto, a conjuração leva 10 minutos a mais e o mago se concentra nela desde o início.
- **Dado** uma conjuração que leva minutos ou horas, **quando** o mestre toca em "Concluir conjuração", **então** o espaço é gasto e a magia faz efeito. **E quando** a concentração do conjurador se quebra antes, **então** a conjuração falha e nenhum espaço é gasto.
- **Dado** uma magia que dura e um conjurador que se concentra nela, **quando** o conjurador conjura outra magia de concentração, **então** a tela pergunta antes "Isso encerra Bênção", e a primeira magia termina com a nova conjuração.
- **Dado** uma magia ativa, **quando** o mestre ou o conjurador toca em "Encerrar", **então** ela termina e o que ela mudou volta (a classe de armadura da Armadura Arcana).
- **Dado** um alvo que veste armadura, **quando** a Armadura Arcana é conjurada nele, **então** a conjuração é recusada com o motivo.
- **Dado** que o mestre conjura por um NPC com "Conjurar como NPC", **então** o NPC não gasta espaço, e uma conjuração sobre um NPC não muda nada nele.
- **Dado** um NPC no palco como alvo, **quando** um jogador lê a conjuração, **então** ele lê o NPC pelo lugar dele no palco e nunca pelo ID de personagem, e uma conjuração feita por um NPC que não está no palco nunca chega a ele (RN-10, RN-20).
- **Dado** um combate em andamento, **quando** o conjurador tenta conjurar fora dele, **então** a conjuração é recusada e a magia é conjurada no combate.
- **Dado** Bênção conjurada num personagem antes de uma luta, **quando** o combate começa, **então** o efeito entra nele com o tempo que lhe resta em rodadas (uma rodada tem 6 segundos, rodada parcial não conta), os testes de resistência do personagem recebem o d4, e quando o combate termina as rodadas que sobraram voltam para o tempo de jogo (RN-22).
- **Dado** Ajuda conjurada fora do combate, **então** a conjuração é registrada e o mestre narra; os pontos de vida máximos só aumentam com uma Ajuda conjurada em combate.

#### No app
- "Conjurar" lista as magias que o personagem pode conjurar, com os espaços, "Como ritual" quando a magia permite, o tempo que leva e os alvos. "Magias ativas" lista as conjurações em andamento ou duradouras, com "Concluir conjuração" (mestre) e "Encerrar".

#### Relacionadas
- [RN-32](regras.md#rn-32-conjurar-fora-do-combate). Num combate as magias são conjuradas lá. Ver [Arquitetura](../../architecture/play/casting.md#casting-outside-combat).

### MR-050: Pacote da campanha, exportar e importar

**Como** mestre, **quero** exportar minha campanha como um arquivo só e criá-la de novo a partir dele, neste servidor ou em outro, **para** que a campanha da mesa se mude sem ser digitada outra vez.

- Prioridade: MVP
- Regras: RN-10, RN-30
- Módulos: campaignpackage, campaigns, characters, maps, play

**Exportar** (o painel "Pacote da campanha" da aba "Regras", "Exportar campanha"):
- Só o mestre. A página mostra o progresso e pode ser fechada. O arquivo (`<campanha>.meurpg.zip`) fica 24 horas ("Baixar de novo") e depois é apagado; ele também é apagado com a campanha ou a conta.
- Ele leva o documento da campanha, as regras da mesa, a galeria (os arquivos de imagem), os mapas (imagem, grade, camadas, portas, luzes, armadilhas, pontos), os NPCs e as criaturas com as anotações do mestre, as cenas com suas ações, pistas e ganchos, os quebra-cabeças, os pontos de batalha com seus encontros, os pontos de tesouro, o conteúdo da mesa e as opções desligadas, e os personagens dos jogadores (como reservados, sem dono).
- Ele nunca leva as anotações privadas dos jogadores, o histórico e os registros das sessões, as contas (ids, e-mails, nomes), os donos e membros, os convites e links para assumir, nem o que cada jogador lembra da névoa. Um mapa volta como preparado: armadilhas armadas, tesouros não encontrados.
- Cada download confere de novo que quem pede é o mestre da campanha.

**Importar** (a página das campanhas, "Importar campanha"):
- O arquivo sobe em partes, então uma conexão que cai recomeça de onde parou. O envio pertence à conta do mestre, uma pessoa tem um por vez, e as partes inacabadas são apagadas uma hora depois da última.
- Uma prévia conta o que o pacote guarda e lista cada problema em palavras simples. Qualquer problema bloqueia "Criar campanha": tudo ou nada, e uma falha no meio não deixa nada para trás.
- Um pacote é recusado quando vem de um MeuRPG mais novo ("Este pacote é de uma versão mais nova do MeuRPG, e este servidor ainda não sabe abri-lo."), foi alterado, traz um campo que este servidor não conhece, passa de 200 MiB ou 2.000 entradas (10 MiB por entrada), está suspeitamente comprimido, tem um nome que sobe para fora de uma pasta, ou se refere a algo que lhe falta.
- Toda regra do app vale como se o mestre tivesse digitado os dados: nomes, tamanhos, CDs, ações de cena, armadilhas, quebra-cabeças, fichas (com o conteúdo de mesa do próprio pacote), os limites de uma campanha (300 imagens, 200 mapas, 200 pontos por mapa, 1.000 entradas de mesa, o limite de personagens e NPCs, que não conta os NPCs feitos para criaturas num combate; eles têm um limite próprio de 1.000) e o limite de campanhas da conta ("Você já tem 10 campanhas.") e a lista de criadores. As imagens passam pelo pipeline do envio (recodificadas, sem metadados).
- Os ids nunca são reaproveitados: cada coisa ganha um id novo e toda referência, inclusive os links no documento, o acompanha.
- Os personagens dos jogadores chegam reservados (MR-049): invisíveis aos jogadores até um deles assumir com o link do mestre.

Ver [Arquitetura](../../architecture/campaign-package.md).

### MR-051: Conversa da cena

**Como** mestre, **quero** um feed de texto ao vivo na sessão onde eu narro, falo como meus NPCs e sussurro para alguns jogadores, e onde os jogadores respondem no personagem ou fora dele, **para que** uma mesa com mestre remoto jogue sem chamada, e ninguém leia o que não era para a pessoa.

- Prioridade: MVP
- Regras: RN-10, RN-18, RN-36
- Módulos: play, maps

#### Critérios de aceitação
- **Dada** uma sessão aberta, **quando** o mestre envia uma linha como narrador, **então** todo membro a lê, na ordem de envio, na tela da sessão e no resumo da sessão.
- **Dado** que o mestre fala como um NPC da campanha, **então** a linha mostra o nome do NPC e o retrato de cena dele, quando tem; um id que não é de um NPC da campanha é recusado.
- **Dado** um jogador, **quando** envia uma linha como o próprio personagem, **então** ela mostra o nome do personagem; como o personagem de outro jogador, **então** é recusada (RN-18); fora do personagem, **então** mostra o nome de exibição dele.
- **Dado** que o mestre escolhe dois jogadores, **quando** a linha é enviada, **então** só esses dois e o mestre recebem o texto, a imagem, a existência e o aviso; o terceiro jogador não recebe nada na leitura, no stream, no resumo nem na voz (RN-36).
- **Dado** que um jogador escolhe "só o mestre", **então** só ele e o mestre leem a linha; um jogador não pode escolher outros jogadores.
- **Dada** uma linha, **quando** passa de 2.000 caracteres ou está vazia, **então** é recusada.
- **Dado** que o mestre anexa uma imagem da galeria, **então** os leitores da linha podem abrir essa imagem e ninguém mais por meio dessa linha; um jogador não pode anexar imagem.
- **Dado** que o mestre está digitando, **então** os jogadores veem "O mestre está escrevendo…" por alguns segundos; o aviso não leva texto e expira sozinho.
- **Dada** uma linha do narrador ou de um NPC, **quando** um leitor pede para ouvi-la, **então** o primeiro pedido faz a voz e a guarda com a linha, os seguintes saem do arquivo guardado, e quem não lê recebe `404`.
- **Dado** que a voz não está configurada, **então** a rota do áudio responde `failed_precondition` `OFF` e o app esconde "Ouvir".
- **Dado** que o mesmo envio se repete com a chave dele, **então** existe uma linha só; linhas demais em pouco tempo são recusadas (`resource_exhausted`).
- **Dado** que um jogador é apagado, **então** as linhas dele ficam e o autor aparece como "Jogador removido".

#### No app
- O painel "Conversa" ao lado da cena e do mapa (o do jogador, a partir de 1024 px, é uma aba da coluna da direita "Conversa · Ficha", ao lado da "Ficha" do personagem), e uma aba da navegação fixa no celular, onde "Ficha" é um painel dentro da sessão; o compositor do mestre tem "Falar como", "Para", "Imagem da galeria" e "Enviar"; o do jogador tem "Como <personagem>" e "Fora do personagem".

#### Relacionadas
- [RN-36](regras.md#rn-36-quem-lê-uma-linha-da-conversa-da-cena), [RN-10](regras.md), [RN-18](regras.md), [MR-028](#mr-028-mostrar-uma-imagem-aos-jogadores), [MR-039](#mr-039-imagens-geradas-para-masmorras-e-cenas). Ver [Arquitetura](../../architecture/play/live-session.md#the-conversation-of-the-scene) e [Privacidade](../privacidade.md#o-que-a-voz-envia-ao-google-mr-051).

### MR-052: O rolador de dados livre

**Como** mestre ou jogador, **quero** rolar quaisquer dados que eu pedir, na sessão, e escolher quem vê a rolagem se eu for o mestre, **para que** a mesa resolva as rolagens fora das regras do jogo (um boato, uma tabela de tesouros, uma aposta) sem um dado físico à mão.

- Prioridade: MVP
- Regras: RN-10, RN-18
- Módulos: play

#### Critérios de aceitação
- **Dada** uma sessão aberta, **quando** um membro da campanha rola um dado entre d4, d6, d8, d10, d12, d20 e d100, de 1 a 20 deles, com um modificador de -20 a +20, **então** o servidor os rola, e o total é a soma dos dados mais o modificador ("2d6 + 3" com 4 e 4 dá 11).
- **Dada** uma rolagem com outro dado, com 0 ou mais de 20 dados, ou com um modificador além de +20 ou -20, **então** ela é recusada e nada é rolado nem gravado.
- **Dado** um d20, **quando** quem rola escolhe vantagem ou desvantagem, **então** dois d20 são rolados, o maior (ou o menor) vale e o outro mostra "descartado"; com quaisquer outros dados a escolha é recusada.
- **Dado** que a mesa joga com dados físicos (RN-18), **quando** um membro rola aqui, **então** o app rola mesmo assim, o registro diz "Rolagem livre", e a folha diz "Esta é a rolagem do app": o modo de dados da mesa vale para as rolagens do jogo, não para esta.
- **Dado** que um jogador rola, **então** ele lê o cartão de resultado (o total em números e por extenso, a fórmula, cada dado), e a mesa toda lê "Rolagem livre · <nome>: 2d6 + 3 = 11 (4, 4)" no "Rolagens da mesa" e vê os dados na tela.
- **Dado** que o mestre rola "Para a mesa", **então** a mesa lê e vê como a rolagem de um jogador.
- **Dado** que o mestre rola "Só eu", **então** só o mestre a lê, com a marca "Só você"; nenhum jogador a recebe na leitura, no stream nem no resumo, e nada avisa um jogador de que aconteceu, nem uma contagem (RN-10). Um jogador que pede "Só eu" é recusado.
- **Dado** um 20 natural ou um 1 natural num d20 só, **então** o resultado diz isso por extenso ("20 natural", "1 natural"), não só pela cor.
- **Dado** que o mesmo toque se repete com a chave dele, **então** existe uma rolagem só, e a mesma chave com outro pedido é recusada; um toque duplo no botão rola uma vez.
- **Dado** que um membro rola 20 vezes seguidas e mais uma, **então** a última é recusada (`resource_exhausted`) e a folha diz "Tente de novo em N segundos".
- **Dado** que a folha está aberta, **então** ela lista as últimas 10 rolagens do membro na sessão (as secretas do mestre incluídas, com "Só você"), e "Repetir 2d6 + 3" preenche o formulário e rola os mesmos dados de novo.
- **Dado** que o stream ao vivo está reconectando, **então** "Dados" fica desabilitado e a tela diz por quê; **dado** que nenhuma sessão está aberta, **então** não há botão "Dados", e a chamada é recusada.
- **Dada** uma pessoa que não é membro ativo da campanha, **então** a chamada é recusada.

#### No app
- O botão "Dados" no cabeçalho da sessão abre "Rolar dados": uma folha de baixo no celular, uma janela a partir do tablet. Ela tem os atalhos (1d20, 1d6, 2d6, 1d100), o dado, "Quantidade (1 a 20)", "Modificador (−20 a +20)", "Como rolar o d20" para um d20 só, o "Quem vê" do mestre ("Para a mesa" ou "Só eu"), "Suas últimas rolagens nesta sessão" com "Repetir", e um botão preenchido ("Rolar 2d6 + 3"). O cartão de resultado tem "Mudar dados" e "Rolar … de novo".
- O "Rolagens da mesa" sob o palco lista as rolagens livres da sessão, da mais nova para a mais antiga; a animação dos dados, uma bandeja de feltro onde os dados rolam e param nas faces que o servidor sorteou, mostra a rolagem dos outros como "<nome> rolou".
- O botão "Som dos dados" da animação liga o som dos dados, baixinho; ele começa desligado e só vale para a aba aberta.
- Rolar tem limite de taxa por pessoa.

#### Relacionadas
- [RN-10](regras.md), [RN-18](regras.md), [MR-051](#mr-051-conversa-da-cena). Ver [Arquitetura](../../architecture/play/free-dice-roller.md) e [Design](../../design/session.md).

### MR-049: Personagens reservados e links para assumir

**Como** mestre, **quero** fazer personagens para os meus jogadores (ou trazê-los com a campanha) e mandar a cada jogador um link para pegar o seu, **para** que a mesa comece a jogar sem todo mundo montar uma ficha antes.

- Prioridade: MVP
- Regras: RN-01, RN-03, RN-10, RN-16
- Módulos: characters, campaigns, identity, play

#### Critérios de aceite
- **Dado** que sou o mestre, **quando** escolho "Criar personagem para um jogador" e salvo, **então** o personagem fica **reservado**: não tem dono, aparece em "Personagens reservados" e nenhum jogador o vê, em nenhuma lista, visão do grupo, ordem de combate, token de mapa, resumo de sessão ou imagem. Ele não pode ser marcado como morto.
- **Dado** um personagem reservado, **quando** escolho "Gerar link para o jogador" (válido por 1, 7 ou 30 dias, 7 por padrão), **então** recebo um link `<app>/claim#t=<token>` que o app mostra **uma vez**, inteiro, com "Copiar link"; gerar outro link revoga o anterior.
- **Dado** o link e nenhuma sessão, **quando** alguém o abre, **então** vê a mesma página de qualquer link, válido ou não (o que é um link para assumir e "Entrar com Google"), sem nada sobre a campanha nem o personagem, **e** entrar só o traz de volta ao cartão do personagem: entrar nunca assume.
- **Dado** o cartão, **quando** aperto "Assumir este personagem", **então** viro membro da campanha (sem etapa de aprovação; um pedido de entrada pendente é encerrado) e dono do personagem, e o link é gasto.
- **Dado** um link inválido, expirado, usado ou revogado, ou um assumir que perde uma corrida contra uma revogação ou contra outro assumir, **quando** qualquer pessoa o abre, **então** lê a mesma página, "Este link não pode ser usado".
- **Dado** um link de assumir que não pode ser usado, ou um link de convite inválido ou expirado, **quando** qualquer pessoa o abre, **então** a página também diz: "Se o mestre separou um personagem para você, peça o link de assumir."
- **Dado** que já tenho um personagem vivo na campanha, **quando** assumo outro, **então** sou recusado com o único motivo específico: leio quem é o personagem vivo e o que fazer (um morto não conta).
- **Dado** que sou o mestre e abro o meu próprio link, **então** leio "Este link é para um jogador" e nada muda.
- **Dado** um personagem reservado com um link, **quando** o revogo, **então** a linha pergunta no lugar e o link deixa de funcionar; **quando** um jogador o usou um instante antes, **então** leio "Este link já foi usado por …" e a lista é lida de novo.
- **Dado** um personagem que um jogador assumiu, **quando** escolho "Devolver à reserva" (uma pergunta no lugar), **então** o personagem fica sem dono de novo e nenhum jogador o vê, o jogador continua membro, e posso mandá-lo a outra pessoa.
- **Dado** um personagem reservado, **quando** escolho "Excluir" (uma pergunta no lugar), **então** o personagem e o link vivo dele são apagados.
- **Dado** um personagem reservado, **quando** uma sessão começa, **então** ele não é travado com os outros; trava com as sessões que começam depois que ele é assumido.

#### No app
- O painel "Personagens reservados" da aba "Mesa" da página da campanha, abaixo dos personagens dos jogadores: cada linha diz em que pé está o link dela e oferece "Gerar link para o jogador" / "Gerar novo link", "Editar", "Revogar o link", "Excluir" e, para um personagem assumido, "Ver ficha" e "Devolver à reserva".
- O mestre faz a ficha no editor de sempre, no modo do mestre (o método de valores de habilidade e a regra de pontos de vida da mesa não se aplicam), e salva com "Salvar como reservado".
- O cartão em `/claim` oferece "Assumir este personagem", "Voltar para minhas campanhas" e "Não é você? Entrar com outra conta".
- Assumir tem limite de taxa por pessoa e por endereço.

#### Relacionadas
- [MR-002](#mr-002-gerar-convite) e [MR-003](#mr-003-entrar-pelo-convite) (a outra entrada numa campanha), [MR-024](#mr-024-aprovar-o-personagem-do-convite) (um membro pendente), [MR-006](#mr-006-ficha-travada). O pacote de uma campanha ([MR-050](#mr-050-pacote-da-campanha-exportar-e-importar)) traz os personagens dela como reservados. Ver [Arquitetura](../../architecture/characters.md#reserved-characters-and-claim-links).

### MR-053: O mestre age pelo personagem de um jogador

**Como** mestre, **quero** jogar o turno do personagem de um jogador quando ele não está (ou não responde), **para que** o combate não pare e a mesa mantenha o personagem do jogador no jogo.

- Prioridade: MVP
- Regras: RN-03, RN-10, RN-18, RN-19, RN-20
- Módulos: play

#### Critérios de aceitação
- **Dado** o turno de um personagem de jogador, **quando** o mestre lê "Turno", **então** o cartão desse personagem mostra "Agir por <nome>" em toda vez de jogador: não há um modo "ausente" para ligar antes. Um personagem com 0 PV não tem "Agir por": o cartão dele é o teste contra a morte (abaixo).
- **Dado** que o mestre toca em "Agir por <nome>", **então** ele recebe o turno do próprio personagem: a economia, as abas de ação montadas pela ficha, os ataques, magias, recursos, Forma Selvagem, invocações e familiar, o movimento e a escolha de alvo, como o jogador os tem, e "Voltar ao mestre" (que devolve o trilho sem encerrar o turno); a barra do combate diz "Você age por <nome>" e o botão cheio dela vira "Encerrar o turno de <nome>" no lugar de "Próximo turno", de modo que a tela tem um só botão de passar a vez.
- **Dado** que o mestre age, **quando** ele move, **então** arrasta o token do personagem ou usa "Mover" no cabeçalho do mapa (não está no trilho) enquanto nenhum alvo está sendo escolhido.
- **Dado** que o mestre age, **quando** ele escolhe um alvo, **então** só vale o que o personagem enxerga (a lista de alvos do servidor para esse personagem: uma criatura escondida fica apagada, "<nome> não vê este alvo"), e o servidor recusa qualquer outro alvo do mesmo jeito (RN-10, RN-20).
- **Dado** que o mestre fez algo por um personagem (um ataque, uma magia, um recurso, um teste contra a morte, um movimento, o fim do turno), **então** o registro do combate diz "(pelo mestre)" no fim da linha, para quem a lê, o jogador também quando volta.
- **Dado** que o jogador volta no meio do turno, **então** qualquer um dos dois pode agir; nada trava, e a segunda resposta à mesma pergunta diz "Já respondido".
- **Dado** que o mestre age, **então** as ferramentas da ordem (dano, condições, as marcas do mestre) continuam disponíveis.
- **Dado** que o combate está na preparação, **então** "Começar o combate" espera todas as iniciativas; "Rolar por <nome>" rola a que falta com o bônus do personagem, "Digitar o resultado" digita para um dado físico, e a barra diz quem falta. O mesmo "Rolar por <nome>" fica ao lado de cada personagem que ainda não respondeu a um teste em grupo ou a um teste que o mestre pediu numa cena, com o resultado digitado quando a mesa joga com dados físicos.
- **Dado** uma reação opcional de um jogador (Escudo Arcano, Esquiva Sobrenatural...) que ninguém respondeu em 10 s, **então** a pergunta vai para o mestre, no topo de "Turno", com o que ela muda em números (o ataque contra a classe de armadura, com e sem a magia), o que custa, a etiqueta "10 s sem resposta", e "Usar" e "Não usar"; a aba diz "Turno · 2" e a barra do combate "Falta responder por <nome>". A primeira resposta vale (a do jogador ou a do mestre); a outra lê "Já respondido".
- **Dado** um personagem com 0 PV no turno dele, **então** o turno começa com o teste contra a morte (SRD 5.1) e não termina sem ele, nem para o mestre: o cartão abre com "<nome> está caído: teste contra a morte", os sucessos e falhas até agora, "Rolar por <nome>" (dados do app) e "Digitar o resultado" (dado físico) do mesmo tamanho; "Próximo turno" fica tracejado e a barra diz "Falta o teste contra a morte". O servidor recusa o fim desse turno até o teste ser feito; o antigo "Passar sem rolar" não existe mais.

#### No app
- "Turno" na vez de um jogador: o cartão "Vez do Pensantus" ("Mago 3 · o jogador joga na tela dele"), os pontos de vida, a CA e o deslocamento, as formas da economia, "Esperando Pensantus. Se o jogador não estiver, jogue este turno por ele." e um botão contornado, "Agir por Pensantus". Agindo, o trilho começa pela faixa "Você está agindo por Pensantus" com "Voltar ao mestre", depois as formas, "PV 23 de 23 · CA 12 · 1º ○○○● 2º ○○" e as abas do personagem (no celular sem as contagens); o alvo se escolhe no mapa ("Toque no alvo · por Pensantus") e a barra fixa do celular vira "Encerrar o turno de Pensantus", ou a barra do alvo do mapa.
- Uma reação sem resposta, uma rolagem retida e o teste em grupo mantêm os cartões no topo de "Turno"; as linhas da iniciativa na preparação têm "Rolar por Brisa" ao lado de "Digitar pelo jogador".

#### Relacionadas
- [MR-013](#mr-013-ordem-dos-turnos), [MR-014](#mr-014-sua-vez), [RN-03](regras.md), [RN-10](regras.md), [RN-18](regras.md). Veja o [Design](../../design/combat.md).

### MR-054: Usar a ação de uma criatura no turno dela

**Como** mestre, **quero** usar o sopro, o olhar ou outra ação de um monstro no cartão do NPC, **para que** o app role os testes e o dano que a ficha pede e eu não faça isso à mão.

- Prioridade: MVP
- Regras: RN-20, RN-22, RN-29, RN-38, RN-39, RN-41, RN-42
- Módulos: play, rules

#### Critérios de aceitação
- **Dado** um monstro do bestiário no turno dele, **quando** leio o cartão, **então** "Outras ações" lista as ações dele que pedem um teste de resistência ou não rolam nada, cada uma com o que o app leu (o atributo e a CD, a área, o dano e "metade se passar", a condição que uma falha dá) e um botão "Usar"; os ataques e o Ataque Múltiplo não estão ali, são feitos com o cartão de ataque (MR-042).
- **Dado** uma ação que o app lê só em parte ou não lê, **quando** a leio, **então** o cartão diz isso ("O texto diz algo que o app não calcula: leia o lembrete.") e mostra o texto do SRD, em inglês.
- **Dado** um combate num mapa e uma ação com forma (um cone, uma linha, uma esfera), **quando** aperto "Usar", **então** a posiciono no mapa como a área de uma magia e aperto "Usar <ação>"; nada é gasto antes disso, e o servidor pega quem está dentro. **Dado** um combate sem grade, ou uma ação sem forma, **então** nomeio os alvos (os inimigos já vêm marcados) e "Usar" espera ao menos um.
- **Dado** que usei a ação, **então** o app rola o teste de cada alvo contra a CD da ficha, a ação é gasta, e sob a lista leio para cada alvo "Falhou" ou "Passou: metade do dano", a rolagem contra a CD e a condição que recebeu; o dano que abriu está ali para aplicar ou descartar, e o personagem de um jogador espera que eu aplique.
- **Dado** que o teste da ação nomeia uma condição, **então** o alvo que falha a recebe, a menos que seja imune ou já a tenha, e o resultado anota a duração e o teste repetido para eu guardar; o app não conta o tempo.
- **Dado** uma ação sem rolagem (Invisibilidade, Mudar de Forma), **quando** a uso, **então** ela gasta a ação, o cartão mostra o texto do SRD com "Lembrete registrado no registro do combate. A ação foi gasta." e o registro diz "usou <ação>: lembrete, a ação foi gasta".
- **Dado** que a ação do NPC já foi gasta, **então** todo "Usar" fica desligado e diz "A ação deste turno já foi usada."; **dado** que não é a vez do NPC ou que não sou o mestre, o servidor recusa.
- **Dado** que desfaço a última ação, **então** o uso vai embora com o dano e a condição.
- **Dado** uma ação com limite ("Recarga 5–6", "3/dia" ou depois de um descanso) que usei, **então** ela é gasta, e o servidor a recusa de novo sem mudar nada ("Essa ação da criatura ainda não recarregou. Nada foi gasto." ou "Essa ação da criatura não tem mais usos. Nada foi gasto."); uma ação com usos por dia continua disponível até os usos acabarem.
- **Dado** uma ação de Recarga gasta, **quando** o turno da criatura começa, **então** o servidor rola um d6 para ela, na ordem da ficha, e uma face dentro do intervalo a deixa disponível de novo; leio o d6 e o resultado no registro, como mestre e como o jogador que controla a criatura (nenhum outro jogador os lê), uma criatura derrotada não rola, e uma ação com usos por dia ou depois de um descanso não volta no combate.
- **Dado** que desfaço o uso de uma ação com limite, **então** ela fica disponível de novo ou recupera o uso; a rolagem no começo de um turno não se desfaz.
- **Dado** um NPC feito a partir de uma criatura que gastou usos de uma ação com usos por dia ou da Resistência Lendária, **quando** um combate novo começa com ele, **então** leio os usos que lhe restam, até o descanso longo da mesa devolvê-los; um monstro do bestiário começa todo combate com todos os usos, e uma ação de Recarga gasta fica disponível de novo.
- **Dado** que sou um jogador que não controla a criatura, **então** não leio nada do que ela gastou, da rolagem nem da recusa, e um turno com rolagem parece o mesmo que um sem ela (RN-20).
- **Dado** que sou o jogador que controla a criatura, **então** leio o que resta de cada ação com limite, o d6 no registro e o motivo da recusa de uma ação gasta.
- **Dado** que sou jogador, **então** não encontro "Outras ações", leio a linha do registro com o nome da ação e, para os alvos que vejo, se cada um passou ou falhou e o dano que tomo, e nunca a ficha, a CD nem qual criatura é (RN-20).

#### No app
- O cartão do NPC do mestre ("Turno"): "Outras ações", com a lista "Quem a ação atinge" quando cabe, um bloco por ação (nome, uso, linhas, nota, texto do SRD, "Usar") e o resultado sob a lista com o dano pendente. Num mapa, o diálogo de posicionar é "<NPC> usa <ação>". Ver [Design](../../design/combat.md#a-creatures-other-actions).

#### Relacionadas
- [MR-042](#mr-042-bestiário), [MR-055](#mr-055-responder-ao-efeito-extra-de-uma-criatura), [RN-38](regras.md#rn-38-ações-de-teste-de-resistência-de-uma-criatura), [RN-41](regras.md#rn-41-ações-sem-rolagem).

### MR-055: Responder ao efeito extra de uma criatura

**Como** mestre, **quero** que o app me ofereça o efeito extra do ataque de um monstro quando ele acerta (a paralisia do carniçal, o agarrão de uma serpente), **para que** eu o aplique com o teste e a CD certos, ou o deixe de lado.

- Prioridade: MVP
- Regras: RN-20, RN-29, RN-34, RN-39, RN-40
- Módulos: play, rules

#### Critérios de aceitação
- **Dado** um monstro cujo ataque pede um teste do alvo ao acertar (as garras do carniçal), **quando** o dano de um acerto cai numa criatura do outro lado que ainda está de pé, **então** recebo uma oferta ao lado dos cartões de dano: "Pedir teste de resistência de <alvo> (CD 10, Constituição). Se falhar: Paralisado", com "Aplicar" e "Dispensar".
- **Dado** que aperto "Aplicar", **então** o app rola o teste do alvo contra a CD, abre o dano extra como dano pendente quando o ataque tem (metade se o alvo passa, quando a ficha diz), dá a condição a quem falha (RN-39), e o registro diz "usou <ataque> em <alvo>: <alvo> falhou no teste (1d20 (8) + 2 = 10 contra CD 10); ficou Paralisado".
- **Dado** um monstro cujo ataque agarra (uma constritora), **quando** acerta, **então** a oferta diz "Agarrar (CD de fuga 12)"; "Aplicar" dá a condição sem teste e a linha anota a CD de fuga.
- **Dado** que aperto "Dispensar", **então** nada é aplicado e o registro diz "dispensou o efeito de <ataque> em <alvo>".
- **Dado** que a oferta foi respondida, **então** ela não se desfaz; **dado** que desfaço o dano que a abriu, **então** a oferta some, e um alvo que o acerto derrubou nunca recebe uma. Um ataque oferece o efeito uma vez para cada dano.
- **Dado** que sou jogador, **então** nunca recebo a oferta e não posso responder uma; leio a linha da resposta com o nome do ataque e o alvo, e a rolagem do teste só num personagem meu, nunca a CD (RN-20).

#### No app
- No mesmo lugar das ofertas do monge, ao lado dos cartões de dano: um cartão com o ícone de pata, o nome do ataque e o alvo, uma linha do que ele faz, "Aplicar" e "Dispensar". Ver [Design](../../design/combat.md#a-creatures-other-actions).

#### Relacionadas
- [MR-054](#mr-054-usar-a-ação-de-uma-criatura-no-turno-dela), [RN-40](regras.md#rn-40-efeito-extra-no-acerto-de-uma-criatura).

## Prioridade: MVP (pré-requisito)

### MR-002: Gerar convite

**Como** mestre, **quero** gerar um link de convite, **para** que os jogadores entrem na campanha.

- Prioridade: MVP (pré-requisito)
- Regras: RN-07
- Módulos: campaigns

#### Critérios de aceite
- **Dado** que sou o mestre de "Mirathel", **quando** gero um convite, **então** recebo um link válido para uma pessoa por 7 dias **e** o servidor guarda só o hash do token.
- **Dado** que sou o mestre, **quando** escolho, **então** posso definir de 1 a 20 usos e uma validade de 5 minutos a 30 dias (a tela oferece 1, 7 ou 30 dias), e o link é mostrado uma vez, com um aviso e um botão de copiar.
- **Dado** um convite que não foi usado, **quando** o mestre o revoga, **então** o link para de funcionar **e** quem já entrou continua na campanha.
- **Dado** a lista de convites, **quando** o mestre a abre, **então** cada convite mostra seus usos, a validade, se exige aprovação e o estado: "Ativo", "Usado", "Expirado" ou "Revogado".
- **Dado** que sou jogador em "Mirathel", **quando** tento gerar, listar ou revogar convites, **então** o servidor recusa.

#### Relacionadas
- [RN-07](regras.md). A RN-15 (convite com aprovação) e a [MR-024](#mr-024-aprovar-o-personagem-do-convite) ampliam esta história: o mestre marca "Exigir aprovação do mestre" no convite, o jogador cria o personagem pelo convite e o mestre aprova.

### MR-005: Criar NPCs

**Como** mestre, **quero** criar NPCs de cada tipo (inimigo, chefe, capanga, da história), com ficha completa ou básica conforme o tipo.

- Prioridade: MVP (pré-requisito)
- Regras: RN-04
- Módulos: characters

#### Critérios de aceite
- **Dado** que sou o mestre de "Mirathel", **quando** crio um inimigo ou um chefe, **então** ele tem ficha completa, com os valores calculados; **quando** crio um capanga ou um NPC da história, **então** ele tem ficha básica. Uma ficha do tipo errado é recusada.
- **Dado** um NPC, **então** ele pertence ao mestre e fica na campanha onde foi criado.
- **Dado** que sou jogador em "Mirathel", **quando** abro a campanha, **então** não vejo nenhum NPC **e** o servidor recusa se eu tentar criar um.
- **Dado** um NPC criado em "Mirathel", **quando** o mestre abre outra campanha, **então** o NPC não aparece lá: usar o mesmo NPC em outras campanhas é a [MR-022](#mr-022-reutilizar-npcs).

## Prioridade: Depois

Propostas para depois do MVP, na ordem do [roadmap](../../roadmap.md).

### MR-007: Importar ficha em PDF

**Como** jogador, **quero** importar minha ficha de um PDF, escolhendo o formato (D&D Beyond ou uma ficha em português), **para** não digitar tudo de novo.

- Prioridade: Depois
- Regras: RN-08
- Módulos: characters

#### Relacionadas
- RN-08: nada de DOCX; só um PDF editável, no formato do D&D Beyond ou na ficha em português.

### MR-017: Subir de nível

**Como** jogador, quando subo de nível, **quero** escolher o que ganho da minha classe, subclasse ou de uma segunda classe, seguindo as regras de D&D 5e.

- Prioridade: Depois
- Regras: RN-12
- Módulos: progression, rules

#### Relacionadas
- A edição guiada da ficha já está no MVP: [MR-040](#mr-040-subir-de-nível-pela-ficha). Esta história é a tela completa de subir de nível.
- Ver [ADR-0008](../../adr/0008-rules-content-as-data-pure-engine.md).

### MR-020: Consultar o livro de regras

**Como** mestre, **quero** consultar o livro de regras com uma busca inteligente.

- Prioridade: Depois
- Regras: —
- Módulos: rules

As magias estão no MVP: [MR-045](#mr-045-consultar-as-magias).

### MR-021: Copiar personagem

**Como** jogador, **quero** copiar meu personagem para outra campanha, **para** jogar nas duas ao mesmo tempo ou numa continuação.

- Prioridade: Depois
- Regras: RN-03, RN-23
- Módulos: characters

#### Critérios de aceite (proposta)
- **Dado** uma ficha que usa conteúdo da mesa (uma classe, raça ou magia que o mestre cadastrou), **quando** o jogador tenta copiá-la para outra campanha, **então** o app recusa e diz por quê: o conteúdo da mesa vale só na própria campanha (RN-23, da MR-025).

#### Relacionadas
- [RN-03](regras.md).

### MR-022: Reutilizar NPCs

**Como** mestre, **quero** usar meus NPCs em várias campanhas, **para** não recriar o mesmo vilão.

- Prioridade: Depois
- Regras: RN-04, RN-23
- Módulos: characters

#### Critérios de aceite (proposta)
- **Dado** um NPC que usa conteúdo da mesa, **quando** o mestre o leva para outra campanha, **então** o app recusa e diz por quê (RN-23, da MR-025).

### MR-023: Passar ou dividir a campanha

**Como** mestre, **quero** passar minha campanha para outro mestre, ou ter um segundo mestre nela, **para** que a campanha continue mesmo se eu sair.

- Prioridade: Depois
- Regras: RN-13
- Módulos: campaigns

#### Consequência
Excluir a conta de quem criou a campanha apaga a campanha inteira (ver [Privacidade](../privacidade.md#excluir-a-conta)). Com mais de um mestre, ou depois de uma passagem, a campanha só seria apagada quando o último mestre saísse. Ver [ADR-0011](../../adr/0011-authorization-roles-per-campaign-checked-per-request.md), como proposta.

### MR-026: Propor uma raça ou classe nova

**Como** jogador, **quero** propor uma raça ou classe que não existe no app quando crio o personagem, com o PDF ou o link das regras, **para** que o mestre possa lê-lo e decidir.

- Prioridade: Depois
- Regras: RN-15 (a mesma ideia de aprovação do convite)
- Módulos: rules, characters

Ela se apoia na MR-025, e a proposta do jogador só vale depois que o mestre aprova.

#### Critérios de aceite (proposta)
- **Dado** que quero jogar com um cozinheiro, uma classe feita por fãs, **quando** crio o personagem e proponho a classe com o link do PDF, **então** o mestre vê o pedido **e** o personagem espera a decisão.
- **Dado** um pedido de classe nova, **quando** o mestre o aprova e cadastra as regras dela (MR-025), **então** o personagem usa a classe; **quando** o mestre recusa, **então** o jogador escolhe outra classe.

### MR-027: Ler as regras de um PDF

**Como** mestre, **quero** enviar o PDF com as regras e ver o app cadastrar sozinho as classes, raças e regras, **para** não digitar tudo.

- Prioridade: Depois
- Regras: —
- Módulos: rules

Ela se apoia no cadastro do próprio mestre (MR-025), e o mestre revisa tudo antes de valer.

#### Critérios de aceite (proposta)
- **Dado** um PDF de regras enviado pelo mestre, **quando** o app o lê, **então** mostra ao mestre o que achou **e** nada vale na campanha até o mestre revisar e aprovar.
- **Dado** um PDF enviado, **quando** o processamento termina ou falha, **então** o arquivo é apagado; um TTL curto no arquivo guardado garante a exclusão mesmo se o processamento falhar.

#### Dúvidas
- Ler um PDF de regras automaticamente exige um serviço de IA, que cobra por uso e recebe o PDF. Um livro oficial tem direitos autorais: o app não pode redistribuir o texto, e o resultado só pode aparecer para a mesa. Quando o PDF não pode ser lido, o cadastro fica com o mestre (MR-025). O app não guarda o PDF: ele fica só enquanto é processado (ver [Privacidade](../privacidade.md#a-definir)).

### MR-046: O estilo da mesa, recurso por recurso

**Como** mestre, **quero** escolher, recurso por recurso, o que o app faz na minha mesa, **para** usar o app do jeito que eu mestro, do tudo digital à mesa clássica.

- Prioridade: Depois
- Regras: RN-24
- Módulos: campaigns, play, maps, characters

O MVP tem os três estilos prontos da página "Regras da mesa" (RN-24); o resto das ideias fica guardado aqui.

#### Ideias (das personas: o mestre narrador, o da mesa física, o tático, o iniciante, o improvisador e a mesa sem celulares)
- O app avisa em vez de bloquear: movimento além do deslocamento, alcance, espaços de magia (o mestre decide).
- Esconder os mapas dos jogadores, ou o celular do jogador mostra só a ficha.
- Quem age no celular: os jogadores, ou o mestre digita tudo e os jogadores só acompanham.
- Quem sobe de nível: o jogador pela ficha ([MR-040](#mr-040-subir-de-nível-pela-ficha)) ou só o mestre.
- As palavras de estado dos inimigos ("Ferido"): mostrar ou esconder.
- O mestre aceita uma magia fora da lista da classe ([MR-045](#mr-045-consultar-as-magias)).
- Uma chave por recurso, com os estilos prontos como ponto de partida.

### MR-047: Mais quebra-cabeças

**Como** mestre, **quero** mais tipos de quebra-cabeça e mais jeitos de dar recompensa, **para** casar o desafio com a história da minha campanha.

- Prioridade: Depois
- Regras: RN-27
- Módulos: play, maps

O MVP já tem seis tipos, dicas, a dica por teste de perícia, a informação dividida, as consequências e "Ao resolver" ([MR-038](#mr-038-quebra-cabeças)); o resto das ideias fica guardado aqui.

#### Ideias
- Peças deslizantes.
- Placas de pressão na grade do mapa (pisar nos quadrados na ordem certa).
- Balanças e pesos.
- Um raio de luz e espelhos na grade (usando a visão do mapa).
- Símbolos tirados da galeria do mestre, e um texto para cada estado do quebra-cabeça.
- Recompensas: XP, um item posto como tesouro no mapa, uma anotação da história.

## Ver também

- [Regras de negócio](regras.md)
- [Visão do produto](visao.md)
- [Glossário](glossario.md)
- [Roadmap](../../roadmap.md)
