[English](../privacy.md)

# Privacidade e proteção de dados

O MeuRPG guarda só o que a mesa precisa para jogar, guarda tudo em São Paulo e trata cada direito do titular como uma funcionalidade.
Este documento diz quais dados pessoais existem, por quê, por quanto tempo, quem os trata e o que toda mudança precisa respeitar.

> **Isto não é parecer jurídico.**
> É orientação de engenharia, escrita por desenvolvedores.
> A decisão por trás dela é a ADR-0010 (repositório privado): "privacidade desde a concepção, LGPD como base e GDPR como régua".
> O aviso de privacidade para quem usa o app é a página `/privacy`, e os termos de uso são `/terms` (ver [O aviso de privacidade e os termos](#o-aviso-de-privacidade-e-os-termos)).
> As duas dizem, logo no começo, que nenhum advogado as revisou.

## Em resumo

- **A lei que vale é a LGPD** (Lei 13.709/2018).
  O GDPR não se aplica, porque o app não é oferecido à União Europeia.
  Em cada tema, ainda assim, seguimos a mais rigorosa das duas.
- **O MeuRPG é o controlador.**
  O canal do encarregado pelo tratamento de dados pessoais é `privacidade@meurpg.app`, o contato das duas páginas públicas.
- **Todos entram com o Google.**
  O app recebe o identificador da conta e o e-mail verificado, e nada mais.
  Um login de jogador sem o Google, sem e-mail nem nome real, é uma proposta ([RN-17](produto/regras.md)).
- **A base legal é o contrato.**
  O app precisa desses dados para funcionar.
  A segurança e os logs usam o legítimo interesse.
  Nada depende de consentimento.
- **Sem cookies de terceiros, analytics, pixels, fontes de CDN nem scripts de terceiros.**
  Os únicos cookies são o de sessão e um cookie de login de vida curta, ambos estritamente necessários, então não há banner.
  O app não guarda mais nada no navegador.
- **O app é para maiores de 18 anos** (ver [Menores de idade](#menores-de-idade)).
- **Ele atende uma mesa**, por convite, e não cobra nada.
  Abri-lo para outras mesas muda o porte do agente de tratamento, então essa decisão volta antes.

## Checklist de privacidade para PRs

Copie no PR que mexe em dados, logs, telas ou fornecedores:

- [ ] **Logs:** só IDs e enums fechados.
  Nada de header, query, corpo, IP, token, e-mail, nome ou texto livre ([o que o log nunca guarda](../architecture/logs.md#what-never-goes-in-the-log)).
- [ ] **URLs:** nenhum segredo nem dado pessoal em path ou query string.
  Token vai no fragmento (`#t=`), que nenhum servidor recebe.
- [ ] **GET do Connect:** só um método cuja requisição não leva ID nem dado pessoal pode usar GET.
  Todo o resto fica em POST ([API](../architecture/api.md)).
- [ ] **Respostas com dado pessoal** saem com `Cache-Control: no-store`.
- [ ] **Coluna ou tabela nova com dado pessoal:** entrou no [inventário](#inventário-de-dados-pessoais) com finalidade e retenção.
- [ ] **Só o necessário:** cada campo novo tem um motivo.
- [ ] **Texto livre novo:** a tela avisa "É ficção: não escreva dados reais de pessoas."
  Texto livre nunca vai para um log nem para um evento da sessão.
- [ ] **Coisas escondidas:** uma resposta para um jogador nunca leva as notas do mestre (RN-11) nem o que o mestre não revelou (RN-10).
- [ ] **Imagens:** só entram pelo envio da galeria, que codifica a imagem de novo e descarta os metadados.
  Nada de URL de imagem externa.
- [ ] **Navegador:** nenhum script, fonte, pixel ou iframe de terceiros, e nenhum `localStorage`, `sessionStorage`, IndexedDB, cookie gravado pelo JavaScript ou worker guardando token ou dado pessoal.
  O único armazenamento no aparelho são os cookies `__Host-` `HttpOnly` ([Web](../architecture/web.md#nothing-in-the-browser-besides-the-session-cookie)).
- [ ] **Fornecedor novo, ou dado saindo do servidor:** atualizar a [tabela de operadores](#operadores-e-onde-os-dados-ficam) e abrir a pergunta de contrato e de transferência internacional.
- [ ] **Segredos:** só no Secret Manager.
- [ ] **Gatilho de impacto:** responder às [perguntas de impacto](#perguntas-de-impacto).

## Inventário de dados pessoais

"Enquanto a campanha existir" quer dizer até a campanha ser apagada, o que só acontece quando a conta do mestre dela é apagada (ver [Excluir a conta](#excluir-a-conta)).
Depois de qualquer exclusão, as cópias ainda somem dos backups e dos logs em até 30 dias.
As tabelas estão descritas no [Modelo de dados](../data/README.md).

### Conta e login

| Dado | Finalidade | Base legal | Retenção |
| --- | --- | --- | --- |
| Identificador da conta Google (`sub`) e seu emissor | Reconhecer a conta no login | Contrato | Até a conta ser apagada |
| E-mail do Google | Contato de segurança (um incidente, um pedido sobre dados) e a lista de quem pode criar campanhas. Nunca aparece para ninguém. Só é gravado quando o Google diz que foi verificado, e é atualizado ou apagado a cada login | Contrato; legítimo interesse | Até a conta ser apagada |
| Nome e foto do Google | Nunca são pedidos e nunca são gravados | — | — |
| Nome de exibição: digitado no app, de 1 a 40 caracteres, nunca vindo do Google. Aparece para os membros das campanhas da pessoa | Mostrar quem é quem na mesa | Contrato | Até a pessoa apagá-lo no perfil ou a conta ser apagada |
| Sessão: o hash do valor do cookie, as datas de criação e de validade, a hora do login e o último uso (gravado no máximo a cada 10 minutos). Sem IP e sem dados do aparelho | Manter o login | Contrato | Vale no máximo 30 dias e termina depois de 14 dias sem uso ("Sair dos outros dispositivos" encerra as outras na hora). A linha vencida é varrida pelo banco em até um dia |
| Estado do login: o hash do `state`, o verificador PKCE, o nonce, para onde voltar (sem fragmento) e, quando a pessoa veio de um convite ou de um link de personagem, o tipo dele e o hash do token (nunca o token) | Concluir o login no Google e depois aceitar o convite ou assumir o personagem | Contrato | 10 minutos; apagado no callback, ou varrido pelo banco |
| O link de personagem com que a pessoa entrou (só o hash) | Mostrar o cartão do personagem depois do login, sem o segredo no endereço | Contrato | 10 minutos; apagado quando o personagem é assumido |
| Convite: hash do token, usos, validade, se exige aprovação, quem criou | Entrar numa campanha (RN-07, RN-15) | Contrato | Vale no máximo 30 dias; a linha é varrida 30 dias depois de vencer, ou na hora com a campanha ou com a conta de quem o criou |
| Link para assumir um personagem reservado (MR-049): hash do token, quem fez e quando, validade, quando foi usado ou revogado e por quem | Entregar um personagem que o mestre fez ao jogador que o assume | Contrato | Varrido 30 dias depois que acabou (usado, revogado ou expirado), ou na hora com a campanha, o personagem ou a conta do mestre. O "usado por" é limpo se a conta desse jogador for apagada |
| E-mail de contato: uma mensagem enviada a `privacidade@meurpg.app` ou a outro endereço `@meurpg.app` (endereço e nome de quem envia, assunto, texto) | Responder a um pedido sobre dados ou a um contato | Obrigação legal (art. 18) para um pedido sobre dados; legítimo interesse nos outros casos | O ImprovMX, que a encaminha, registra remetente, destinatário e assunto por 7 dias, nunca o texto. Nas caixas de e-mail de quem responde: 1 ano depois da resposta, e então é apagada |

### O que a mesa escreve e joga

Campanhas, membros, personagens, mapas, combates e o resto do jogo são o conteúdo da mesa.
As linhas abaixo dizem o que cada tipo guarda e quem pode lê-lo.
As linhas que guardam só IDs, números e chaves de conteúdo não dizem nada sobre uma pessoa.

| Dado | Quem escreve, quem lê | Finalidade | Base legal | Retenção |
| --- | --- | --- | --- | --- |
| Campanhas: nome, modo de XP, modo de dados, regras da mesa (inclusive até 20 regras da casa nas palavras do mestre) | O mestre escreve; todo membro ativo lê | Jogar | Contrato | Enquanto a campanha existir |
| Membros e papéis: quem, em qual campanha, a preferência de dados e se o membro ainda espera a aprovação do mestre | O mestre e o membro | Controlar o acesso (RN-05, RN-15) | Contrato | Enquanto a campanha existir. Um membro pendente sem personagem é removido depois de 30 dias, ou na hora quando o mestre recusa o personagem ou remove o membro |
| Ficha e história do personagem: nome, equipamento, idiomas, características, o antecedente "Outro", personalidade, aparência, história, aliados. Texto livre, escrito como ficção | O jogador escreve; o mestre e o dono leem; os outros jogadores nunca leem | Jogar. Não há campo para o nome real nem para o e-mail do jogador | Contrato | Enquanto o personagem existir. Personagens não são apagados no fluxo normal (RN-03). Um personagem pendente que o mestre recusa é apagado na hora. Quando a conta do jogador é apagada, o personagem fica com a campanha e o mestre, sem vínculo com a conta (RN-16); o que ficar sem jogador e sem campanha é varrido pelo banco em até um dia. Um NPC some com a conta do mestre |
| Notas do mestre sobre um personagem | Só o mestre (RN-11) | Preparar o jogo | Contrato | Enquanto o personagem existir; notas vazias apagam a linha |
| O motivo do mestre ao pedir mudanças num personagem pendente ("Pedir ajustes"), até 500 caracteres | Só o mestre e o dono; uma lista leva o estado, nunca o motivo | Dizer ao jogador o que corrigir | Contrato | Até o personagem ser aprovado ou recusado, ou o personagem, a campanha ou a conta do jogador serem apagados |
| Documento da campanha, ganchos de uma cena de RP, pistas que o mestre prepara e o encontro guardado num ponto de batalha | Só o mestre | Preparar o jogo | Contrato | Enquanto a campanha existir |
| Pistas reveladas a um jogador (com uma cópia do texto) e as cenas que o grupo descobriu | O mestre e o jogador que as recebeu | Deixar o jogador reler o que foi dito, mesmo que o mestre edite a pista | Contrato | Enquanto a campanha existir, ou até a conta do jogador ser apagada. Se o mestre apaga a pista, a cópia fica com o jogador |
| As anotações particulares do jogador (de 1 a 2.000 caracteres, com uma etiqueta de cena opcional) | Só o autor: nem o mestre, nem outro jogador | Deixar o jogador anotar o que acontece | Contrato | Até o autor apagar a anotação, a conta do autor ser apagada ou a campanha ser apagada |
| A conversa da cena (MR-051, RN-36): o autor de cada linha (ID da conta), o falante (narrador, um NPC, um personagem ou fora do personagem), o texto (de 1 a 2.000 caracteres, texto livre), uma imagem opcional da galeria, a hora, os leitores quando não é para todos, uma chave de idempotência e a voz de uma linha do narrador ou de um NPC depois que alguém pediu para ouvir | O mestre lê todas as linhas. Um jogador lê as linhas para todos, as que o escolheram e as próprias: uma linha para outros leitores nunca é enviada a ele (RN-10) | Deixar a mesa conversar durante a sessão, também com um mestre remoto | Contrato | Enquanto a campanha existir. Quando a conta do autor é apagada, a linha fica e o autor passa a ser "Jogador removido" |
| Quebra-cabeças: o texto do mestre (nome, pista, dicas, "Ao resolver", "Ao errar", informação dividida) e a solução; por sessão, cada jogada com o ID da conta, o personagem e **o que o jogador digitou** (uma resposta de enigma, uma mensagem decifrada), e cada tentativa de ganhar uma dica | O mestre escreve. Um jogador nunca lê a solução, as dicas ainda não soltas nem o texto de outro jogador; o mestre lê o que cada jogador digitou | Jogar quebra-cabeças ao vivo (MR-038, RN-27) | Contrato | Os quebra-cabeças, enquanto a campanha existir (são arquivados, não apagados). As jogadas e as tentativas, enquanto a sessão existir. Apagar uma conta tira o ID dela das jogadas e das tentativas; o texto digitado fica, anônimo, até a sessão sumir |
| Motivos e nomes que o mestre ou um jogador escrevem no jogo: o nome de um combate, o motivo de um modo de rolagem ou de um prêmio de XP (até 120 caracteres), marcos planejados, o nome-base de um grupo de NPCs, os nomes que um jogador dá a um familiar ou outra criatura, o conteúdo da mesa (classes, magias, antecedentes, criaturas e assim por diante) | O mestre (e o jogador dono, para o nome de uma criatura e uma criatura da mesa do personagem dele); o motivo de um prêmio de XP é lido por todos os membros | Jogar | Contrato | Enquanto a campanha existir. O histórico de XP registra quem deu o prêmio; o vínculo é limpo se essa conta for apagada |
| Imagens da galeria (mapas, retratos, ilustrações): os pixels codificados de novo sem metadados, a miniatura, o nome tirado do arquivo (editável) e quem as enviou | O mestre envia e lista; um jogador só baixa uma imagem enquanto ela estiver visível para ele (RN-10) | Jogar (MR-019) | Contrato | Até o mestre apagar a imagem: a linha e os arquivos somem na hora, e o Cloud Storage guarda o arquivo mais 7 dias no soft delete. Quando uma campanha é apagada, as linhas somem; os arquivos, não (ver [Excluir a conta](#excluir-a-conta)) |
| Pedidos de imagem por IA: quem pediu, o texto do mestre, o estilo, as referências, o modelo, o estado e, num pedido feito a partir de um mapa, o mapa e o tamanho dele | O mestre | Gerar imagens (MR-039, RN-28); contar o limite mensal | Contrato | Enquanto a campanha existir |
| Mapas, pontos de interesse, camadas pintadas, tokens, portas, armadilhas, tesouros e quem os achou, luzes, ações de cena, o palco e as imagens mostradas e deixadas da sessão | O mestre escreve; um jogador lê só o que lhe foi revelado (RN-10). Nomes e descrições são texto livre do mestre | Jogar (MR-008 e seguintes) | Contrato | Enquanto a campanha existir |
| O que os personagens de cada jogador viram de um mapa com névoa de guerra: um bitmap de quadrados por jogador e por mapa | Só o servidor. O jogador recebe o estado de cada quadrado; o mestre recebe o mesmo em "Ver como" | Mostrar, escurecido, o que um personagem já viu (MR-036) | Contrato | Até o mestre limpar ("Esquecer o que foi visto"), trocar a grade ou a imagem do mapa, ou o mapa, a conta do jogador ou a campanha serem apagados |
| Combates, combatentes, rolagens, dano pendente, reações, testes contra a morte, PV, espaços de magia, dados de vida, registros de subida de nível, valores de habilidade rolados, condições e o resto do estado do jogo ao vivo | Conforme a RN-20, um jogador lê só os próprios números e a palavra de estado dos inimigos | Jogar | Contrato | Enquanto a campanha existir. O ID da conta do jogador num combatente é limpo se a conta for apagada. Os valores de habilidade rolados e ainda não usados vão embora com a conta |
| Histórico da sessão (`session_events`): quem fez a mudança (ID da conta), os IDs do personagem e do combate, números antes e depois, uma chave de idempotência, a hora. Nunca um nome nem texto livre. Num combate num mapa com névoa de guerra, os IDs das contas que viram um NPC agir; numa cena aberta para só alguns jogadores, os IDs das contas para as quais ela foi aberta e das que estavam nela quando um teste foi rolado | O mestre e, filtrado, os jogadores | Auditoria, desfazer e o registro do combate | Contrato | Enquanto a campanha existir. O ID de quem agiu é limpo se a conta for apagada, e também o ID em toda lista de quem viu dentro de um payload (ver [Excluir a conta](#excluir-a-conta)) |
| Assinaturas do stream ao vivo: ID da conta, se é o mestre, a campanha | Memória do servidor | Entregar cada mudança só a quem pode vê-la | Contrato | Enquanto o stream estiver aberto (no máximo 10 minutos); nunca no banco nem no log |

### Pacote da campanha (MR-050)

| Dado | Finalidade | Base legal | Retenção |
| --- | --- | --- | --- |
| Um arquivo de exportação e as partes de um envio | Levar uma campanha de um servidor para outro | Contrato | Uma exportação, 24 horas depois de pronta (uma hora, se falhou); as partes de um envio, 1 hora depois da última parte. Os dois também são removidos, com as linhas, pela próxima varredura (em até 10 minutos) quando a campanha ou a conta que pediu deixa de existir |

### Logs, memória e infraestrutura

| Dado | Onde | Finalidade | Base legal | Retenção |
| --- | --- | --- | --- | --- |
| ID da conta, ID da campanha e outros IDs de jogo, caminho e resultado da requisição, no log da API (sem IP, e-mail nem nome) | Cloud Logging, São Paulo (produção); stdout no ambiente local | Achar a causa de um erro e seguir uma sessão | Legítimo interesse | 30 dias em produção; ver [Os logs da API](#os-logs-da-api) |
| IP, user agent e URL de cada requisição, com a decisão do Cloud Armor | Logs do Cloud Run e do balanceador de carga no Cloud Logging, São Paulo | Operar e proteger a plataforma | Legítimo interesse | 30 dias |
| IP do cliente nos limitadores de taxa (login e toda a API) | Memória do servidor | Frear força bruta e abuso | Legítimo interesse | Minutos; nunca no banco nem no log |
| Uma tarefa da fila de trabalho em segundo plano: o tipo de trabalho e IDs (campanha, pedido, exportação), nunca um nome nem texto livre | Cloud Tasks, São Paulo (a região do serviço) | Gerar uma imagem, uma exportação ou uma varredura depois que a chamada que pediu já respondeu | Contrato | Até o trabalho rodar. Uma tarefa que continua falhando é descartada no máximo 1 hora depois da primeira tentativa (a duração máxima de novas tentativas da fila, [Operação](../operations/runbook.md#11-cloud-tasks)) |
| Backups do banco | Cockroach Labs, São Paulo | Recuperação de desastre | Legítimo interesse | No máximo 30 dias ([Operação](../operations/reference.md)) |

### Cookies e navegador

| Cookie | Finalidade | Base | Duração |
| --- | --- | --- | --- |
| `__Host-meurpg_session` | Manter o login | Estritamente necessário | Até o logout (que apaga a sessão no servidor), 14 dias sem uso, ou 30 dias |
| `__Host-meurpg_login` | Amarrar o login ao navegador que o começou (contra login CSRF) | Estritamente necessário | 10 minutos; apagado no callback |

Os dois são `HttpOnly` e nunca lidos pela página.
O app não guarda mais nada no navegador, e `web/src/no-web-storage.spec.ts` quebra o build se o código-fonte tocar numa API de armazenamento.

### Os logs da API

Os logs da API guardam o ID da conta (um UUID, um pseudônimo) e IDs de jogo, e nada que identifique uma pessoa por si só.
Cada linha leva um `request_id` e, quando existem, `user_id` e `campaign_id`.
As linhas de rpc, de stream e de evento dizem o que foi feito só com IDs e enums.
Os campos e os níveis estão em [Logs](../architecture/logs.md).

| Dado no log | Por quê | Retenção |
| --- | --- | --- |
| `user_id` | Seguir uma sessão e achar a causa de um erro. Só vira uma pessoa com o banco, e não aponta para ninguém depois que a conta é apagada | A do log: 30 dias no bucket `_Default` do Cloud Logging ([Operação](../operations/logs.md)); 14 dias no ELK de ensaio |
| `campaign_id` e outros IDs de jogo | O mesmo | A do log |
| `request_id` | Juntar as linhas de uma requisição; volta no header `X-Request-Id` | A do log |
| Caminho da requisição (sem a query) e código do resultado | Operar o serviço | A do log |

O log da API nunca guarda o IP, o user agent, cookies, headers, a query da URL, e-mail, qualquer nome (de exibição, de personagem, de NPC, de campanha), qualquer texto digitado nem corpos de requisição e de resposta.
Uma trava no handler de log descarta atributos com nomes como `email`, `token`, `name` ou `text`.
O IP e o user agent aparecem só no log de requisições que o próprio Cloud Run escreve, a linha da plataforma acima.
Apagar uma conta não reescreve os logs: a linha vence com a retenção.
Um log nunca é fonte para responder a um pedido do titular.

## Como o app protege o que guarda

- **O login pede ao Google só `openid` e `email`.**
  Um login que falha registra um motivo, nunca um token, código, state, cookie, e-mail ou ID.
- **O e-mail não sai do banco para lugar nenhum.**
  A resposta do perfil leva o ID da conta e o nome de exibição.
- **Os segredos viajam no fragmento.**
  O token de um convite ou de um link de personagem vai no corpo de um POST, é guardado só como hash e nunca chega a uma query de URL, a um log nem ao armazenamento do navegador.
  Quem abre um convite sem estar logado o leva pelo login dentro do estado do login, que guarda só o hash dele.
- **Respostas com dado pessoal levam `Cache-Control: no-store`.**
  As rotas de login também levam `Referrer-Policy: no-referrer`.
- **Quem não é membro não descobre uma campanha.**
  A resposta é o mesmo `not_found` de uma campanha que não existe.
  Um membro pendente vê só o nome da campanha e o próprio personagem.
- **Um jogador lê só o que as regras permitem.**
  As notas do mestre, o documento da campanha, os pontos escondidos do mapa, a solução de um quebra-cabeça, as notas de outro jogador e o PV exato de um inimigo nunca chegam a um jogador.
  Toda leitura e todo evento do stream são filtrados no servidor, e os testes de vazamento provam isso ([Testes](../testing.md)).
- **Um personagem reservado não tem dado de jogador.**
  Enquanto não tem dono, nenhum jogador o lê; um link de personagem mostra só o nome, a raça, a classe e o nível dele, o nome da campanha e o nome de exibição de quem mandou o link, e só a uma pessoa logada que tenha o link.
  Toda recusa de um link lê igual, então um estranho não distingue um link usado de um que nunca existiu.
- **As imagens não levam metadados.**
  O servidor decodifica cada envio e o codifica de novo a partir dos pixels, o que descarta EXIF (posição de GPS, modelo do celular, data), XMP, perfis ICC e qualquer coisa escondida depois da imagem.
  O arquivo original nunca é gravado.
  Ao lado de todo "Enviar imagem", a tela avisa: "Use imagens do jogo. Não envie fotos de pessoas sem a autorização delas."
- **As imagens ficam em armazenamento privado.**
  No ambiente local, num volume do Docker; em produção, num bucket privado do Cloud Storage em São Paulo, sem URL pública.
  Só a API o lê, e ela confere a sessão e a participação a cada requisição.
  O navegador de um jogador pode guardar uma imagem, mas pergunta de novo a cada uso, então ele deixa de vê-la assim que ela deixa de estar visível.
  O que já foi desenhado numa tela, salvo ou fotografado está fora do nosso alcance.
- **As imagens da névoa de guerra são pessoais.**
  Um jogador nunca recebe a imagem inteira de um mapa com névoa, só pedaços com os pixels dos quadrados que ele viu ou lembra e preto opaco no resto.
  Os pedaços vivem só na memória do servidor e não são dado pessoal.
  Ver [Mapas](../architecture/maps/images.md).
- **O stream não guarda nada.**
  A cada 60 segundos ele confere a sessão e a participação de novo; sair, uma sessão revogada ou deixar a campanha o encerra.

### O que o pacote da campanha faz (MR-050)

O pacote da campanha deixa um mestre exportar uma campanha como um arquivo e criar uma campanha a partir de um arquivo assim ([Pacote da campanha](../architecture/campaign-package.md)).

- **O que ele leva:** o que o mestre preparou (documento, notas, regras e conteúdo da mesa, imagens da galeria codificadas de novo, mapas, NPCs, cenas, quebra-cabeças, pontos) e os personagens dos jogadores com as histórias deles, sem dono.
- **O que ele nunca leva:** as anotações particulares dos jogadores, o histórico da sessão e os logs, contas (IDs, e-mails, nomes de exibição), membros, convites, links de personagem, a memória de névoa de cada jogador e o que só existe durante um combate.
  O formato do arquivo não tem campo para nada disso, e um teste planta um marcador em cada um e o procura no arquivo.
- **Onde o arquivo fica:** no bucket privado, em `campaigns/<id>/exports/`, pelo tempo do inventário.
  O bucket tem uma regra de ciclo de vida como segunda rede ([Operação](../operations/reference.md#campaign-package)).
- **Quem pode baixá-lo:** só o mestre da campanha, conferido de novo a cada requisição.
  A resposta é um anexo com `no-store`.
- **Quem pode enviar:** a conta que começou o envio; qualquer outra conta recebe o `404` de um envio que não existe.
- **Ao importar:** o arquivo é lido como não confiável.
  Toda imagem é decodificada e codificada de novo, e todo texto passa pelas verificações do app.
  Dado de uma pessoa real digitado num campo de texto livre é responsabilidade do mestre.

## Direitos do titular e como atendemos

O que a pessoa pode fazer sozinha, ela faz no app.
Todo o resto passa pelo canal do encarregado e é respondido em até **15 dias**.

| Direito | LGPD | Como |
| --- | --- | --- |
| Confirmação e acesso | Art. 18, I e II | E-mail para `privacidade@meurpg.app` |
| Portabilidade | Art. 18, V | E-mail para o mesmo endereço. Um mestre pode exportar o conteúdo de uma campanha no app ([MR-050](produto/historias.md)); esse arquivo nunca leva dados de conta |
| Correção | Art. 18, III | O nome de exibição, no perfil. O texto de um personagem depois da trava da ficha (RN-01): o jogador o edita quando o mestre destrava, senão por e-mail |
| Anonimização, bloqueio ou eliminação do que for desnecessário | Art. 18, IV | E-mail |
| Eliminação | Art. 18, VI | E-mail; ver [Excluir a conta](#excluir-a-conta) |
| Com quem compartilhamos | Art. 18, VII | Os operadores deste documento e do aviso de privacidade |
| Oposição e outros pedidos | Art. 18, §2º | E-mail |
| Revogar o consentimento | Art. 18, IX | Nada depende de consentimento. Um recurso opcional acrescentado depois terá um botão de desligar |
| Reclamar na ANPD | Art. 18, §1º | Em gov.br/anpd, a qualquer momento |

Uma pessoa também pode encerrar todos os outros logins com "Sair dos outros dispositivos", no perfil.

### Excluir a conta

**A pessoa pede por e-mail: não há tela nem exclusão automática.**
Exportação e exclusão em autoatendimento, e a exclusão por inatividade, são propostas na [RN-16](produto/regras.md#rn-16-exclusão-de-conta-e-inatividade).
Nada é apagado por inatividade.

O que apagar uma conta faz, pelas regras do próprio banco:

| O quê | Resultado |
| --- | --- |
| Identidade, e-mail, nome de exibição, sessões, participações, as anotações particulares da pessoa, cópias de pistas, motivos de revisão e valores de habilidade rolados | Apagados |
| As campanhas de um mestre, com tudo o que elas têm: membros, convites, personagens da campanha, NPCs, mapas, sessões, histórico, quebra-cabeças, pedidos de imagem | Apagadas |
| Os personagens de um jogador | Ficam com a campanha e o mestre, sem vínculo com a conta. Um personagem que ficar sem jogador e sem campanha é varrido em até um dia |
| O que a pessoa digitou num quebra-cabeça (uma resposta de enigma, uma mensagem decifrada), guardado nas jogadas de uma sessão | Fica, anônimo: as respostas digitadas ficam sem a conta, e o mestre continua lendo, até a sessão sumir. Nada no texto é procurado nem apagado |
| Linhas da conversa da cena (MR-051) | Ficam, com o ID do autor limpo: aparecem como "Jogador removido". Uma linha que era para leitores escolhidos continua para o mestre e para os leitores que restam, nunca para todos |
| Histórico da sessão, combatentes, prêmios de XP, envios, jogadas de quebra-cabeça | Ficam, com o ID da pessoa limpo. Os IDs dos jogadores que viram um NPC agir, ou que estavam numa cena aberta para só alguns jogadores, guardados dentro de alguns payloads do histórico, saem dessas listas no mesmo comando que apaga a conta |
| Os arquivos das imagens de uma campanha apagada, no bucket | **Não são apagados pelo banco.** O operador apaga o prefixo `campaigns/<id>/` à mão; isso não é automatizado |
| Backups e logs | Somem em até 30 dias |

Uma campanha só é apagada pela conta do mestre dela: o app não tem exclusão de campanha nem um jeito de um membro aceito sair.

### A tensão entre manter o personagem e apagar a identidade

Manter com o mestre o personagem de um jogador apagado (RN-16) não pode virar um jeito de manter a identidade dessa pessoa.
O vínculo com a conta some, mas o texto livre do personagem (história, aparência, personalidade, aliados) foi escrito pela pessoa e pode ter dados pessoais dela ou de outros, apesar do aviso "É ficção".
Esse texto não é filtrado nem editado.
É conteúdo do jogo, e o mestre decide o que fazer com ele, como com um NPC.
Isto é uma escolha de produto, não uma garantia legal de que nenhum dado pessoal sobra, então continua sendo uma pergunta para um advogado (ver [A definir](#a-definir)).
Quem preferir pode pedir, por e-mail, que os personagens também sejam apagados.
Ver a ADR-0010, seção 4.3.

## Operadores e onde os dados ficam

Os dados ficam em São Paulo (`southamerica-east1`).
Um fornecedor fora do Brasil que consiga alcançá-los ainda faz uma transferência internacional, então cada um precisa de um contrato e de um mecanismo de transferência (Resolução CD/ANPD nº 19/2024).

| Fornecedor | O que faz | Dados | Contrato |
| --- | --- | --- | --- |
| Google Cloud | Cloud Run, o balanceador de carga e o Cloud Armor na frente dele, Cloud Logging, Cloud Storage, Cloud Tasks, Secret Manager | Tudo, em São Paulo | Adendo de Processamento de Dados com as cláusulas-padrão brasileiras |
| Cockroach Labs (CockroachDB Cloud) | O banco gerenciado e seus backups, no Google Cloud em São Paulo | O banco de dados | **A definir:** o contrato atual cobre o GDPR |
| Google (login) | Entrar com o Google | O Google controla a própria conta; o app recebe só o identificador e o e-mail | Não é nosso operador |
| Google (Gemini API) | Gera as imagens que o mestre pede e a voz das linhas do narrador e dos NPCs, pelo servidor | Ver abaixo. O Google pode guardar dados por pouco tempo ou em cache **em qualquer país**: uma transferência internacional | [Termos dos serviços pagos da Gemini API](https://ai.google.dev/gemini-api/terms). **A definir:** o mecanismo de transferência |
| ImprovMX | Encaminha o e-mail enviado a `@meurpg.app` para quem responde ([Operação](../operations/runbook.md#dns-certificates-and-e-mail)) | A mensagem em trânsito. O log dele guarda remetente, destinatário e assunto por 7 dias, nunca o texto. Ele não diz onde ficam os servidores: conta como transferência internacional | **A definir:** o contrato e o mecanismo de transferência |

### O banco do app antigo

A hospedagem do app anterior (o Render para o servidor, o GitHub Pages para as páginas) e o cluster CockroachDB separado dele não fazem parte deste sistema.
Eles não guardam nenhum dado do app atual.
Desligá-los, e apagar esse cluster depois da importação única de personagens ([Modelo de dados](../data/README.md)), é tarefa do dono do projeto; os backups dos provedores somem em até 30 dias.

### O que as imagens geradas mandam ao Google (MR-039, RN-28)

Só o mestre gera imagens.
A chamada vai à Gemini API com `store=false`, então nada é guardado na nossa conta do Google; pelos termos dos serviços pagos, o Google ainda pode guardar dados por pouco tempo ou em cache, em qualquer país.

- **O que vai:** o texto que o mestre escreve (como foi escrito), o estilo, a proporção, as imagens da galeria escolhidas como referência (já codificadas de novo) e, num ajuste, a imagem anterior com o texto do primeiro pedido e os ajustes até ali.
- **O que nunca vai:** o nome, o e-mail, a ficha, o nome de jogador ou o ID de uma pessoa.
  O pedido não tem campo para eles, e um teste confere o corpo.
  O texto livre do mestre vai como foi escrito, então a janela avisa sob cada campo ("Não escreva nomes de pessoas.", "Não use foto de pessoa.") e o rodapé diz que toda imagem gerada leva a marca invisível SynthID do Google.
- **A partir de um mapa,** o pedido também leva um desenho que o servidor faz com as camadas do mapa, nunca com a imagem enviada, que pode mostrar um segredo.

| Caminho | O desenho | A lista de salas |
| --- | --- | --- |
| Arte de cena e vista isométrica | Chão e paredes dos quadrados que os personagens dos jogadores veem agora, o resto preto; um disco azul por personagem de jogador e um vermelho por NPC que eles veem; uma porta secreta não revelada como parede. Sem porta, número, nome, token escondido, armadilha nem tesouro | Não |
| Mapa texturizado | O mapa inteiro, só chão e paredes, a porta secreta como parede, sem criaturas | Só para uma masmorra gerada: o número e o tamanho de cada sala, nada mais |

- **Os retratos** de NPCs vão só na arte de cena e na vista isométrica, e só os de NPCs que os jogadores veem agora.
  O nome de um NPC e o nome de um personagem de jogador nunca vão.
- **O desenho não é guardado em lugar nenhum;** ele é refeito a cada pedido.
- **No nosso banco** fica a linha do pedido (inventário acima).
  O texto é dado do mestre e vai com a campanha.
  O texto e a chave da API nunca chegam a um log.
- **A imagem que o Google devolve** é guardada como um envio, codificada de novo, com a mesma retenção.
  Uma referência JPEG de 1024 px derivada dela é guardada ao lado e apagada com ela.
- **Só o mestre pede,** e o app é para maiores de 18 anos, então dado de menor nunca vai.

### O que a voz envia ao Google (MR-051)

Só as linhas do narrador e dos NPCs podem ser ouvidas, e só os leitores da linha podem pedir.
O servidor chama o texto para fala da API Gemini (`generateContent`, que não guarda interação na nossa conta; pelos termos dos serviços pagos o Google ainda pode guardar dados por pouco tempo ou em cache, em qualquer país).

- **O que vai:** o texto da linha, como o mestre escreveu, e o nome de uma voz de uma lista curta. O texto é livre: o mestre é avisado no compositor para não escrever nomes de pessoas reais.
- **O que nunca vai:** a linha de um jogador (as linhas dos jogadores são só texto), um nome, um e-mail, um ID de conta ou de campanha, os leitores ou a imagem.
- **Uma vez por linha:** o áudio é feito na primeira vez que um leitor toca em "Ouvir" e guardado no armazenamento de arquivos ao lado da linha, então o texto é enviado uma vez. É apagado com a campanha.
- **Só os leitores ouvem:** a rota do áudio responde `404` a quem não pode ler a linha (RN-36).
- **A chave e o texto nunca chegam a um log.** Um teto diário para o servidor todo limita a conta do Gemini.

### Logs no ELK

O ELK que guarda os logs da API no ambiente de ensaio (e em produção, se vier) apaga cada linha depois de 14 dias, dentro do teto de 30 dias dos logs ([Operação](../operations/logs.md#locally-with-elk)).
Só a equipe de operação o lê, por um papel somente de leitura ou pelo Kibana com login.
As linhas guardam `user_id` e `campaign_id`, e nenhum e-mail, nome ou texto livre.
Apagar uma conta não apaga linhas já escritas; elas vencem sozinhas.
Operado por nós, o ELK não é um operador; se a produção usar o Elastic Cloud, ele entra na tabela acima.

## Menores de idade

O app é para maiores de 18 anos, como dizem os termos.
Ele não guarda data de nascimento nem declaração de idade.
Os convites são pessoais, e o app não tem busca de mesas, perfil público nem chat com estranhos.
Um menor numa mesa joga sem conta própria: o mestre cuida da ficha, sem dado pessoal do menor.
Antes de o app abrir ao público, um advogado precisa avaliar as regras brasileiras para menores online (ECA Digital, Lei 15.211/2025), e uma verificação de idade precisa seguir a orientação final da ANPD.
O GDPR fixa uma idade mínima só quando a base é o consentimento, e a base aqui é o contrato.

## Incidentes

- Avaliamos em até 24 horas se um incidente pode causar risco ou dano relevante.
  Um hash de senha ou uma sessão vazados contam como "dados de autenticação" nos critérios da ANPD.
- Avisamos a ANPD e as pessoas afetadas em até 72 horas corridas, por mensagem no app e por e-mail.
  A lei dá 3 dias úteis; 72 horas cabem nisso e no prazo do GDPR.
- Registramos todo incidente por 5 anos, mesmo os que não precisam de aviso.

O runbook está no repositório privado.

## Perguntas de impacto

Todo PR responde a estas.
Um "sim" pede uma seção curta de riscos e medidas no PR.
Dois "sim" pedem um relatório de impacto à proteção de dados (RIPD) antes do merge.

1. Ele coleta um tipo novo de dado pessoal (um dado sensível, um documento, uma foto de pessoa, uma localização)?
2. Envolve uma criança ou um adolescente, ou verificação de idade?
3. Envia dados a um terceiro novo, ou para fora do Brasil?
4. Decide algo sobre pessoas de forma automática, monta um perfil ou usa IA com dados pessoais?
5. Observa comportamento (analytics, rastreamento)?
6. Torna algo público ou fácil de achar (perfil público, busca de mesas, chat)?
7. Muda a escala (abrir para outras mesas)?
8. Usa dados para uma finalidade nova, ou cruza bancos?

## A definir

- ImprovMX: o contrato e o mecanismo de transferência, ou a troca dele por e-mail na conta Google da organização.
- O contrato da LGPD com a Cockroach Labs.
- Se os backups do CockroachDB Cloud ficam em São Paulo e são guardados por no máximo 30 dias (conferir no console); o plano está em avaliação ([Operação](../operations/reference.md)).
- O mecanismo de transferência para a Gemini API do Google (Resolução CD/ANPD nº 19/2024).
- Revisão de um advogado do aviso de privacidade, dos termos e do tratamento do texto livre que fica com o mestre, antes de o app abrir para outras mesas.
- Exportação e exclusão de conta em autoatendimento, e apagar os arquivos de imagem de uma campanha quando ela é apagada (o prefixo `campaigns/<id>/`). Até existirem, o operador faz a pedido.
- Antes de ler PDFs de regras com IA (MR-027, depois do MVP): o operador, o que sai do servidor e os direitos autorais dos livros oficiais.
  O PDF seria guardado só enquanto fosse processado e apagado logo depois.

## O aviso de privacidade e os termos

O aviso de privacidade é a página `/privacy` e os termos de uso são `/terms`.
São páginas públicas (sem login), ligadas no rodapé de toda página, em português simples, datadas ("Última atualização") e com uma caixa de resumo.
O texto mora em `web/src/app/pages/legal/` e precisa dizer o que este documento diz: uma mudança no inventário, nos operadores, nos prazos de retenção ou nos cookies só está pronta quando a página também muda.

O aviso tem 14 seções: quem cuida dos dados, os dados que usamos e por quê, os dados da conta Google, onde os dados ficam e os operadores, retenção, cookies, o que não fazemos, quem vê o quê, direitos, exclusão da conta, segurança e incidentes, a idade mínima, mudanças e o contato.
Os termos têm 12.
A exportação e a exclusão da conta são tratadas por e-mail, e as duas páginas dizem isso.

## Ver também

- [Arquitetura](../architecture/README.md): módulos, login e logs.
- [Modelo de dados](../data/README.md): as tabelas citadas aqui.
- [Operação](../operations/README.md): segredos, logs, armazenamento e custos.
- [Regras de negócio](produto/regras.md): RN-10 e RN-11, que protegem o que um jogador vê; RN-16, exclusão de conta; RN-17, login sem o Google.
- `docs/adr/` (repositório privado): ADR-0002 (sessão), ADR-0009 (login do jogador sem o Google), ADR-0010 (privacidade), ADR-0011 (papéis por campanha).
