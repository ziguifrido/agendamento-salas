# Guia de contribuição

## Objetivo

Manter uma agenda de salas simples, rápida e segura, com Go, SQLite e páginas HTML server-side.

## Convenções

- Prefira biblioteca padrão e SQL parametrizado; `modernc.org/sqlite` é a única dependência Go direta de produção.
- HTMX é servido localmente em `web/templates/static/js/htmx.js`; não adicione outros frameworks JavaScript, ORMs ou camadas de repositório sem necessidade comprovada.
- Preserve a validação de horário no servidor e adicione um teste pequeno para regras novas.
- Use HTML semântico, foco visível e controles com pelo menos 44px para toque.

## Fluxo

Rode `go fmt ./...`, `go test ./...` e `go vet ./...`. Valide manualmente criação, conflito e cancelamento. Pull requests precisam manter esses comandos verdes e explicar qualquer dependência nova.

Todo commit deve partir de uma solicitação explícita do usuário. O projeto segue versionamento semântico: consulte e atualize o arquivo `VERSION` em cada lançamento, e crie a respectiva tag Git no formato `vMAJOR.MINOR.PATCH`.

## Decisões

As migrações são idempotentes no código; a aplicação não possui autenticação até que exista requisito de identidade e autorização. A agenda diária mantém sua data em cookie de sessão; notificações e dados de formulário após erro usam cookie temporário e não devem ser colocados na URL.
