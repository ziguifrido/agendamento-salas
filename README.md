# Reservas de Salas

Aplicação leve para reservar salas de reunião, feita com Go, SQLite, HTML e CSS. A interface é responsiva e instalável como PWA.

## Executar

Requer Go 1.24+. Rode `go run ./cmd/server` e acesse `http://localhost:8080`. O banco fica em `data/reservas.db`; defina `DATABASE_PATH` para mudar o local.

Com Docker: `docker compose up --build`. O volume `salas-data` preserva as reservas. Para backup, copie o arquivo SQLite com a aplicação parada ou use `sqlite3 reservas.db .backup backup.db`.

## Desenvolvimento

`go test ./...` valida datas, horários, navegação diária e preservação do formulário. Não há ORM: consultas SQL são parametrizadas e o índice de agenda atende a busca por sala, data e horário.

## Sessão

A data selecionada da agenda é mantida em um cookie de sessão. Mensagens de sistema e os dados do formulário após erro usam um cookie temporário, consumido na próxima página; por isso não aparecem ao recarregar ou compartilhar a URL.

## Estrutura

- `cmd/server`: servidor, regras e migração SQLite.
- `web/templates`: páginas HTML renderizadas pelo servidor.
- `web/templates/static`: CSS, JavaScript (incluindo HTMX local), manifesto, service worker e ícones da PWA.

## Roadmap

Adicionar autenticação e permissões quando houver uma fonte de identidade definida. Licença [MIT](LICENSE).
