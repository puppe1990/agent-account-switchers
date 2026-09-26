# UI web `switcher-ui` — trocar contas pelo navegador

Data: 2026-09-25
Status: aprovado (execução autônoma)

## Objetivo

Interface web local e mínima para listar e trocar as contas ativas do **OpenCode Go** (`ocgs`), do **Command Code** (`ccs`), do **Grok** e do **Codex** sem decorar comandos. Mesmo efeito dos CLIs: reescreve o `auth.json`/`account.json` correspondente; sessões já abertas valem no próximo launch. No Codex, o card também oferece o login numa janela anônima para adicionar outra conta.

## Fora de escopo

- Adicionar/remover/verificar contas pela UI — exceto o login do Codex, que roda `codex login` e registra a conta no registry
- Outros provedores (Copilot, OpenAI API key, …)
- Autenticação, multiusuário, exposição em rede
- Build do Tailwind (usa CDN)

## Arquitetura

```
cmd/switcher-ui/     main: resolve dirs, adapters, listen 127.0.0.1, abre browser
internal/webui/      Server HTTP + handlers + index.html (embed)
internal/grokstore/  leitura dos perfis ~/.grok/accounts + switch do auth.json
internal/codexstore/ leitura do registry do codex-auth + switch/import do ~/.codex/auth.json
internal/codexlogin/ roda `codex login`, abre a URL numa janela anônima e acompanha o status
```

- `webui.Service` é a fatia mínima de cada switcher: `List() ([]Account, error)` e `Switch(name) error`.
- `webui.Login` é opcional no `NamedService`: `Start()/Status()` para fluxos de login interativos; a view do serviço anuncia `login: true` para a UI mostrar o botão.
- `Account{Name, Active}` — sem credenciais; a UI nunca vê chaves.
- `cmd/switcher-ui` define adapters finos sobre `store.Store` (Go), `ccstore.Store` (Command Code), `grokstore.Store` (Grok) e `codexstore.Store`/`codexlogin.Manager` (Codex), convertendo os `Entry` de cada pacote para `webui.Account`.
- `grokstore` é independente do módulo `github.com/puppe1990/grok-accounts` (os pacotes de lá são `internal/`, não importáveis): relê o mesmo formato de perfil e reescreve `~/.grok/auth.json`. Só lista e troca — não adiciona/remove.
- `codexstore` relê o `accounts/registry.json` do `codex-auth` (schema 3) e troca copiando `accounts/<key>.auth.json` sobre `~/.codex/auth.json`, atualizando `active_account_key` no registry. Antes de sobrescrever, sincroniza a auth viva no perfil salvo correspondente para não perder tokens renovados pelo Codex (`Sync`); ao final de um login, importa a conta nova (`ImportCurrent`, claims do `id_token` → registro).
- `codexlogin` roda `codex login` em segundo plano, esconde o navegador padrão do CLI (shim de `open`/`xdg-open` no PATH) e abre a URL de autenticação numa janela anônima do Brave (fallback: Chrome/Edge/Chromium, depois o navegador padrão). Ao terminar, chama `ImportCurrent`.

## HTTP

Servidor `net/http` (stdlib), sem dependências novas. Escuta só `127.0.0.1`.

| Rota                       | Efeito                                                                    |
| -------------------------- | ------------------------------------------------------------------------- |
| `GET /`                    | `index.html` embutido (`//go:embed`), Tailwind via CDN                    |
| `GET /api/accounts`        | JSON `[{id,label,login?,accounts:[{name,active}]}]`                       |
| `POST /api/switch`         | body `{"service","name"}` → troca e devolve a lista atualizada            |
| `POST /api/login`          | body `{"service"}` → inicia o login e devolve o status                    |
| `GET /api/login?service=…` | status do login: `{state,message,url?}` (`idle`/`running`/`done`/`error`) |

Erros: JSON `{"error": "..."}` com status 400 (corpo/nome, serviço sem login), 404 (serviço), 422 (falha do switcher), 500 (leitura). Corpo limitado a 64 KiB e campos desconhecidos rejeitados.

## UI

Página única, tema escuro, um card por serviço. Conta ativa destacada; clicar em outra dispara o switch e refaz a lista. Serviços com `login: true` ganham o botão **login anônimo**: a página inicia o fluxo, acompanha o status a cada 2s (retomando o acompanhamento se a página recarregar) e, no fim, recarrega as contas. Feedback por mensagem de status, com link de fallback quando a janela anônima não abre. Todo texto vindo do backend é escapado no JS.

## Testes

`httptest` + services falsos (`internal/webui/webui_test.go`): lista, switch ok, serviço inexistente, nome vazio, corpo inválido, erro do switcher, erro de leitura, HTML servido, 404, flag `login` e rotas de login (start, status, serviço sem login). `codexstore` e `codexlogin` testam registry/auth em `t.TempDir()` e o fluxo de login com um comando falso no lugar do `codex`. Sem tocar em HOME real.
