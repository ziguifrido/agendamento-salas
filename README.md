# Reserva de Salas de Reunião

> Uma agenda leve para consultar a disponibilidade e reservar salas de reunião — construída com Go, SQLite e HTML renderizado no servidor.

[![Último commit](https://img.shields.io/github/last-commit/ziguifrido/agendamento-salas?label=%C3%BAltimo%20commit&color=6C8EBF)](https://github.com/ziguifrido/agendamento-salas/commits/main)
[![Atividade de commits](https://img.shields.io/github/commit-activity/m/ziguifrido/agendamento-salas?label=atividade&color=6C8EBF)](https://github.com/ziguifrido/agendamento-salas/commits/main)
[![Tamanho do repositório](https://img.shields.io/github/repo-size/ziguifrido/agendamento-salas?label=tamanho&color=6C8EBF)](https://github.com/ziguifrido/agendamento-salas)
[![Versão](https://img.shields.io/badge/vers%C3%A3o-v0.2.0-6C8EBF)](VERSION)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![SQLite](https://img.shields.io/badge/SQLite-embutido-003B57?logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![PWA](https://img.shields.io/badge/PWA-instal%C3%A1vel-5A0FC8?logo=pwa&logoColor=white)](https://web.dev/learn/pwa/)
[![Licença MIT](https://img.shields.io/badge/licen%C3%A7a-MIT-6C8EBF)](LICENSE)

## Visão geral

O **Reserva de Salas de Reunião** foi pensado para equipes que precisam de uma forma direta de organizar o uso de espaços compartilhados, sem depender de uma aplicação complexa. Ele funciona com um único binário e um banco SQLite local.

### Principais recursos

- Agenda por **dia** ou por **semana**, com navegação entre datas.
- Busca por sala, responsável ou título da reserva.
- Filtro de sala, troca de visualização e navegação atualizados imediatamente.
- Cadastro e gestão de salas em dialogs: visualizar, editar e excluir.
- Criação, detalhamento e cancelamento de reservas.
- Bloqueio de horários sobrepostos para a mesma sala.
- Notificações empilhadas, descartáveis e temporárias.
- Interface responsiva, acessível e instalável como PWA.
- Dados persistidos em SQLite, sem ORM e com consultas parametrizadas.

## Comece rapidamente

### Pré-requisitos

- [Go](https://go.dev/dl/) 1.24 ou superior; ou
- [Docker Compose](https://docs.docker.com/compose/), para executar em contêiner.

### Executar localmente

```bash
go run ./cmd/server
```

Abra [http://localhost:8080](http://localhost:8080). Na primeira execução, o banco será criado automaticamente em `data/reservas.db`.

### Executar com Docker

```bash
docker compose up --build
```

A aplicação ficará disponível em [http://localhost:8080](http://localhost:8080). O volume nomeado `salas-data` mantém as reservas entre reinicializações dos contêineres.

Para encerrar:

```bash
docker compose down
```

## Como usar

1. Abra a engrenagem no rodapé do painel lateral e selecione **Cadastrar sala**.
2. Use **Gerenciar salas** no mesmo menu para visualizar, editar ou excluir uma sala. Salas com agendamentos não podem ser excluídas.
3. Na agenda, selecione **Nova reserva**.
4. Escolha a sala, responsável, título, data e intervalo de horário.
5. Confirme a reserva. Se já houver ocupação no período, a aplicação informa o conflito e preserva o formulário para correção.

Use a busca para localizar reservas e alterne entre as visões diária e semanal conforme necessário. Clique em uma reserva para ver seus detalhes; o botão **Cancelar** remove uma reserva existente após confirmação.

## Configuração

| Variável | Padrão | Descrição |
| --- | --- | --- |
| `ADDR` | `:8080` | Endereço e porta em que o servidor escuta. |
| `DATABASE_PATH` | `data/reservas.db` | Caminho do arquivo SQLite. |

Exemplo com banco e porta personalizados:

```bash
ADDR=:3000 DATABASE_PATH=/var/lib/salas/reservas.db go run ./cmd/server
```

## Dados, regras e segurança

- As tabelas e índices são criados automaticamente de forma idempotente na inicialização.
- Reservas devem ter data válida, não anterior ao dia atual e horário de início menor que o de término.
- Uma sala não pode ter reservas com horários sobrepostos no mesmo dia.
- A data, visualização e filtro de sala da agenda são guardados em cookies de sessão.
- Mensagens e dados de formulário após erro usam um cookie temporário, consumido no próximo carregamento; por isso não são incluídos na URL. A pilha visual de notificações é mantida no `sessionStorage` até o descarte individual.
- As respostas incluem cabeçalhos de proteção para conteúdo, frame e origem de referência. Requisições `POST` com origem externa são recusadas.

> **Nota:** ainda não há autenticação ou autorização. Use a aplicação apenas em um ambiente confiável até que exista uma fonte de identidade definida.

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
