# Reserva de Salas de Reunião

> Uma agenda leve para consultar a disponibilidade e reservar salas de reunião — construída com Go, SQLite e HTML renderizado no servidor.

[![Último commit](https://img.shields.io/github/last-commit/ziguifrido/agendamento-salas?label=%C3%BAltimo%20commit&color=6C8EBF)](https://github.com/ziguifrido/agendamento-salas/commits/main)
[![Atividade de commits](https://img.shields.io/github/commit-activity/m/ziguifrido/agendamento-salas?label=atividade&color=6C8EBF)](https://github.com/ziguifrido/agendamento-salas/commits/main)
[![Tamanho do repositório](https://img.shields.io/github/repo-size/ziguifrido/agendamento-salas?label=tamanho&color=6C8EBF)](https://github.com/ziguifrido/agendamento-salas)
[![Versão](https://img.shields.io/badge/vers%C3%A3o-v0.3.0-6C8EBF)](VERSION)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![SQLite](https://img.shields.io/badge/SQLite-embutido-003B57?logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![PWA](https://img.shields.io/badge/PWA-instal%C3%A1vel-5A0FC8?logo=pwa&logoColor=white)](https://web.dev/learn/pwa/)
[![Licença MIT](https://img.shields.io/badge/licen%C3%A7a-MIT-6C8EBF)](LICENSE)

## Visão geral

O **Reserva de Salas de Reunião** foi pensado para equipes que precisam de uma forma direta de organizar o uso de espaços compartilhados, sem depender de uma aplicação complexa. Ele funciona com um único binário e um banco SQLite local.

### Principais recursos

- Agenda por **dia** ou por **semana**, com navegação entre datas.
- Modo apresentação e atualização automática da agenda a cada 60 segundos.
- Temas claro, escuro e automático, seguindo a preferência do sistema quando selecionado.
- Busca por sala, responsável ou título da reserva.
- Filtro de sala, troca de visualização e navegação atualizados imediatamente.
- Cadastro e gestão de salas em dialogs: visualizar, editar e excluir.
- Login exclusivo pelo Google, com restrição opcional de domínio e papéis de administrador e usuário.
- Solicitação de reservas por usuários e aprovação ou rejeição por administradores.
- Usuários podem cancelar apenas as próprias solicitações enquanto pendentes.
- Administradores podem criar, editar e cancelar reservas não encerradas.
- Agendamentos encerrados escurecidos e agendamentos em andamento destacados.
- Bloqueio de horários sobrepostos para a mesma sala.
- Notificações empilhadas, descartáveis e temporárias.
- Interface responsiva, acessível e instalável como PWA.
- Dados persistidos em SQLite, sem ORM e com consultas parametrizadas.

## Comece rapidamente

### Pré-requisitos

- [Go](https://go.dev/dl/) 1.24 ou superior; ou
- [Docker Compose](https://docs.docker.com/compose/), para executar em contêiner.

### Executar localmente

Crie um cliente OAuth 2.0 do tipo **Aplicativo da Web** no Google Cloud e cadastre `http://localhost:8080/auth/google/callback` como URI de redirecionamento autorizada. Copie o modelo de configuração:

```bash
cp .env.template .env
```

Edite o `.env` e preencha `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL` e ao menos um e-mail em `INITIAL_ADMIN_USERS`. A aplicação carrega esse arquivo automaticamente; variáveis já exportadas no ambiente têm precedência. O `.env` contém segredos e está ignorado pelo Git e pelo contexto de build do Docker.

No primeiro acesso, entre com um dos e-mails de `INITIAL_ADMIN_USERS`; ele será criado e promovido automaticamente a administrador. Depois disso, os demais papéis podem ser gerenciados pela interface. Use `ALLOWED_EMAIL_DOMAIN="empresa.com.br"` para restringir o login ao domínio da organização.

```bash
go run ./cmd/server
```

Abra [http://localhost:8080](http://localhost:8080). Na primeira execução, o banco será criado automaticamente em `data/reservas.db`.

### Executar com Docker

O Docker Compose usa o mesmo arquivo `.env`. Depois de configurá-lo, execute:

```bash
docker compose up --build
```

A aplicação ficará disponível em [http://localhost:8080](http://localhost:8080). O volume nomeado `salas-data` mantém as reservas entre reinicializações dos contêineres.

Para encerrar:

```bash
docker compose down
```

## Como usar

1. Entre com uma conta Google autorizada.
2. Usuários selecionam **Solicitar reserva**; o pedido fica pendente e não bloqueia definitivamente o horário.
3. Administradores usam **Solicitações pendentes** para aprovar ou rejeitar. Apenas reservas aprovadas ocupam o horário.
4. Em **Salas**, todos podem consultar os detalhes; administradores também podem cadastrar, editar e excluir.
5. Administradores usam **Gerenciar usuários** para pesquisar, filtrar e alterar papéis.
6. Use o botão com seta para recolher ou ampliar o painel lateral conforme necessário.

Use a busca para localizar reservas e alterne entre as visões diária e semanal conforme necessário. Clique em uma reserva para ver seus detalhes. Usuários podem cancelar somente suas próprias solicitações pendentes; administradores podem editar e cancelar reservas não encerradas. Pelo menu de configurações, escolha o tema claro, escuro ou automático e habilite o modo apresentação ou a atualização automática. Ambos verificam a virada do dia a cada 60 segundos e selecionam a data local atual do navegador quando necessário.

## Configuração

| Variável | Padrão | Descrição |
| --- | --- | --- |
| `ADDR` | `:8080` | Endereço e porta em que o servidor escuta. |
| `DATABASE_PATH` | `data/reservas.db` | Caminho do arquivo SQLite. |
| `HOST_PORT` | `8080` | Porta publicada no host pelo Docker Compose. |
| `SERVER_PORT` | `8080` | Porta interna usada pelo servidor no Docker Compose. |
| `GOOGLE_CLIENT_ID` | — | Client ID OAuth 2.0 do Google. Obrigatório. |
| `GOOGLE_CLIENT_SECRET` | — | Client secret OAuth 2.0 do Google. Obrigatório. |
| `GOOGLE_REDIRECT_URL` | — | Callback cadastrado no Google, como `https://salas.exemplo.com/auth/google/callback`. Obrigatório. |
| `ALLOWED_EMAIL_DOMAIN` | vazio | Quando definido, permite somente e-mails desse domínio. |
| `INITIAL_ADMIN_USERS` | vazio | E-mails separados por vírgula promovidos a administrador durante o login; configure ao menos um para o bootstrap inicial. |

Exemplo de valores no `.env`:

```dotenv
GOOGLE_CLIENT_ID=seu-client-id
GOOGLE_CLIENT_SECRET=seu-client-secret
GOOGLE_REDIRECT_URL=http://localhost:8080/auth/google/callback
ALLOWED_EMAIL_DOMAIN=empresa.com.br
INITIAL_ADMIN_USERS=admin@empresa.com.br
```

## Dados, regras e segurança

- As tabelas, colunas e índices são criados automaticamente de forma idempotente na inicialização.
- Reservas devem ter data válida, não anterior ao dia atual e horário de início menor que o de término.
- Uma sala não pode ter reservas aprovadas com horários sobrepostos no mesmo dia; solicitações pendentes são exibidas sem bloquear o horário.
- A data, a busca, a visualização e o filtro de sala da agenda são guardados em cookies de sessão.
- Mensagens e dados de formulário após erro usam um cookie temporário, consumido no próximo carregamento; por isso não são incluídos na URL. A pilha visual de notificações é mantida no `sessionStorage` até o descarte individual.
- As preferências de tema, apresentação, atualização automática e painel lateral são mantidas no `sessionStorage`. O estado temporal dos agendamentos usa o relógio local do navegador.
- Sessões usam tokens aleatórios armazenados apenas como hash no banco e cookies `HttpOnly`, `SameSite=Lax` e `Secure` sob HTTPS.
- Todas as mutações autenticadas exigem token CSRF e permissões são verificadas novamente no backend.
- As respostas incluem cabeçalhos de proteção para conteúdo, frame e origem de referência. Requisições `POST` com origem externa são recusadas.

## Backup do banco

Para evitar cópia durante uma gravação, faça o backup com a aplicação parada ou use o comando de backup do SQLite:

```bash
sqlite3 data/reservas.db '.backup backup-reservas.db'
```

No Docker, primeiro copie ou monte o volume `salas-data` conforme a política de backup do seu ambiente.

## Desenvolvimento

Antes de enviar alterações, execute:

```bash
go fmt ./...
go test ./...
go vet ./...
```

Os testes cobrem validação de datas e horários, navegação diária, início da semana, preservação dos campos de reserva após erros e exclusão de salas.

## Versionamento

O projeto segue [versionamento semântico](https://semver.org/lang/pt-BR/). A versão atual é mantida no arquivo [`VERSION`](VERSION) e cada lançamento deve receber uma tag Git no formato `vMAJOR.MINOR.PATCH`.

- **MAJOR:** mudanças incompatíveis.
- **MINOR:** novos recursos compatíveis.
- **PATCH:** correções compatíveis.

## Estrutura do projeto

```text
.
├── cmd/server/              # servidor HTTP, regras de negócio e migração SQLite
├── web/templates/           # páginas HTML renderizadas no servidor
│   └── static/              # CSS, JavaScript, PWA, ícones e HTMX local
├── Dockerfile               # imagem de produção
├── docker-compose.yml       # execução local com volume persistente
└── data/                    # banco criado em execução local (ignorado pelo Git)
```

## Tecnologias

- [Go](https://go.dev/) e `net/http`
- [SQLite](https://www.sqlite.org/) via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)
- HTML e CSS sem frameworks de interface
- [HTMX](https://htmx.org/), distribuído localmente
- Service worker e manifesto web para instalação como PWA

## Licença

Distribuído sob a [Licença MIT](LICENSE).
