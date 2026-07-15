# Guia de contribuição

## Objetivo

Manter uma agenda de salas simples, rápida e segura, com Go, SQLite e páginas HTML server-side.

## Convenções

- Prefira biblioteca padrão e SQL parametrizado; `modernc.org/sqlite` é a única dependência Go direta de produção.
- HTMX é servido localmente em `web/templates/static/js/htmx.js`; não adicione outros frameworks JavaScript, ORMs ou camadas de repositório sem necessidade comprovada.
- Preserve a validação de horário no servidor e adicione um teste pequeno para regras novas.
- Use HTML semântico, foco visível e controles com pelo menos 44px para toque.
- Ao alterar arquivos estáticos, atualize a versão do cache em `web/templates/static/sw.js`.

## Fluxo

Rode `go fmt ./...`, `go test ./...` e `go vet ./...`. Valide manualmente criação, conflito, cancelamento, troca de filtros, gestão de salas e notificações. Pull requests precisam manter esses comandos verdes e explicar qualquer dependência nova.

Todo commit deve partir de uma solicitação explícita do usuário. O projeto segue versionamento semântico: consulte e atualize o arquivo `VERSION` em cada lançamento, e crie a respectiva tag Git no formato `vMAJOR.MINOR.PATCH`.

## Decisões

As migrações são idempotentes no código; a aplicação não possui autenticação até que exista requisito de identidade e autorização. A data, a visualização e o filtro de sala da agenda usam cookies de sessão. Dados de formulário e a origem de erros usam cookie temporário; a pilha visual de notificações usa `sessionStorage`. Nenhum desses estados deve ser colocado na URL.

O cadastro e a gestão de salas são dialogs da agenda. Preserve a proteção contra exclusão de salas com agendamentos e a confirmação antes da exclusão.

O menu de configurações concentra o modo apresentação e a atualização automática. Ambos recarregam a agenda a cada 30 segundos; na troca de dia, use a data local do navegador para atualizar o cookie de sessão. A classificação visual de agendamentos encerrados e em andamento também deve usar o relógio do navegador, para respeitar o fuso horário do usuário.
