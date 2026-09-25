# UI web `switcher-ui` — trocar contas pelo navegador

Data: 2026-09-25
Status: aprovado (execução autônoma)

## Objetivo

Interface web local e mínima para listar e trocar as contas ativas do **OpenCode Go** (`ocgs`) e do **Command Code** (`ccs`) sem decorar comandos. Mesmo efeito dos CLIs: reescreve o `auth.json`/`account.json` correspondente; sessões já abertas valem no próximo launch.

## Fora de escopo

- Adicionar/remover/verificar contas pela UI (só listar e trocar)
- Grok / outros provedores (repo separado `grok-accounts`)
- Autenticação, multiusuário, exposição em rede
- Build do Tailwind (usa CDN)

## Arquitetura

```
cmd/switcher-ui/     main: resolve dirs, adapters, listen 127.0.0.1, abre browser
internal/webui/      Server HTTP + handlers + index.html (embed)
```

- `webui.Service` é a fatia mínima de cada switcher: `List() ([]Account, error)` e `Switch(name) error`.
- `Account{Name, Active}` — sem credenciais; a UI nunca vê chaves.
- `cmd/switcher-ui` define dois adapters finos sobre `store.Store` (Go) e `ccstore.Store` (Command Code), convertendo os `Entry` de cada pacote para `webui.Account`.

## HTTP

Servidor `net/http` (stdlib), sem dependências novas. Escuta só `127.0.0.1`.

| Rota                | Efeito                                                         |
| ------------------- | -------------------------------------------------------------- |
| `GET /`             | `index.html` embutido (`//go:embed`), Tailwind via CDN         |
| `GET /api/accounts` | JSON `[{id,label,accounts:[{name,active}]}]`                   |
| `POST /api/switch`  | body `{"service","name"}` → troca e devolve a lista atualizada |

Erros: JSON `{"error": "..."}` com status 400 (corpo/nome), 404 (serviço), 422 (falha do switcher), 500 (leitura). Corpo limitado a 64 KiB e campos desconhecidos rejeitados.

## UI

Página única, tema escuro, um card por serviço. Conta ativa destacada; clicar em outra dispara o switch e refaz a lista. Feedback por mensagem de status. Todo texto vindo do backend é escapado no JS.

## Testes

`httptest` + services falsos (`internal/webui/webui_test.go`): lista, switch ok, serviço inexistente, nome vazio, corpo inválido, erro do switcher, erro de leitura, HTML servido, 404. Sem tocar em HOME real.
