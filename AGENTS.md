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

As migrações são idempotentes no código. A autenticação usa exclusivamente o fluxo OAuth 2.0 web-server do Google, implementado com a biblioteca padrão. O e-mail verificado é a chave lógica do usuário; tokens opacos de sessão são armazenados somente como hash no SQLite e enviados por cookie `HttpOnly`, `SameSite=Lax` e `Secure` sob HTTPS. Toda mutação autenticada exige CSRF e toda autorização é refeita no backend.

O RBAC possui apenas os papéis `admin` e `user`. Administradores gerenciam salas, reservas, aprovações e papéis; usuários consultam a agenda, solicitam reservas e cancelam apenas solicitações próprias pendentes. Reservas pendentes não bloqueiam horários; a aprovação deve continuar atômica em relação a conflitos. `ALLOWED_EMAIL_DOMAIN` restringe o domínio no backend e `INITIAL_ADMIN_USERS` promove os e-mails de bootstrap durante o login. Mudanças futuras não devem aceitar papel, identidade ou propriedade enviados pelo cliente como fonte de autorização.

A data, a visualização e o filtro de sala da agenda usam cookies de sessão. Dados de formulário e a origem de erros usam cookie temporário; a pilha visual de notificações usa `sessionStorage`. Nenhum desses estados deve ser colocado na URL.

O cadastro e a gestão de salas são dialogs da agenda. Preserve a proteção contra exclusão de salas com agendamentos e a confirmação antes da exclusão.

O menu de configurações concentra os temas claro, escuro e automático, além do modo apresentação e da atualização automática. A preferência de tema usa `sessionStorage` e o modo automático acompanha o sistema operacional sem recarregar a página. A agenda assina `/events` (Server-Sent Events) em todas as abas; quando o servidor notifica uma mudança em salas ou reservas, o cliente busca a página em segundo plano e troca apenas as regiões afetadas (agenda, selects de sala e lista de gerenciamento), sem recarregar a página — o reload completo é apenas fallback e ocorre na troca de dia. Fora dos modos apresentação e atualização automática, que aplicam a atualização imediatamente, ela é adiada enquanto houver dialog aberto ou campo de formulário focado, para não descartar dados digitados. Os controles da agenda usam delegação de eventos no `document` para sobreviver à troca de HTML. Na troca de dia, detectada por verificação periódica nesses modos, use a data local do navegador para atualizar o cookie de sessão. A classificação visual de agendamentos encerrados e em andamento também deve usar o relógio do navegador, para respeitar o fuso horário do usuário.
