# CLI `ccs` — troca de contas Command Code (`cmd`)

Data: 2026-09-23  
Status: approved (execução autônoma)

## Objetivo

CLI em Go para listar, adicionar, salvar, trocar, remover e verificar contas do **Command Code** (`cmd`). O Command Code 1.64 guarda uma única sessão em `~/.commandcode/auth.json`; o `ccs` mantém um ledger nomeado ao lado e, no switch, reescreve esse `auth.json`.

Comando: `ccs`.  
Módulo: `agent-account-switchers` (mesmo repo do `ocgs`).  
Go 1.24+, Cobra, `testing` + `gofakeit`.

## Fora de escopo

- Troca de BYOK / outros providers em `config.json`
- Sessões, taste, MCP, settings, hooks
- Picker interativo
- Auto-rotate de quota
- Sessões `cmd` já abertas (vale no próximo launch)

## Arquitetura

```
cmd/ccs/             main: Store + HTTP + stdout
internal/ccstore/    auth.json + ccs-accounts.json
internal/cccli/      Cobra (mesmo vocabulário do ocgs)
internal/ccverify/   GET /alpha/whoami, cliente injetável
```

Diretório:

1. `$CCS_DATA_DIR` se definido
2. `~/.commandcode`

Arquivos:

- `<dir>/auth.json` — sessão ativa do Command Code (`apiKey`, `userId`, `userName`, `keyName`, `authenticatedAt`), modo `0600`
- `<dir>/ccs-accounts.json` — ledger `{version:1, accounts:{nome: Auth}, active: nome}`, modo `0600`

O `Store` não grava `config.json`, `settings.json` nem sessões.

## Comandos

Mesma UX do `ocgs`: sem args = list; primeiro token desconhecido = `switch`.

| Invocação | Efeito |
|---|---|
| `ccs` / `ccs list` | Nomes do ledger; `→` se `apiKey` bate com `auth.json`. Sem keys no stdout. |
| `ccs switch` sem nome | `informe a conta: ccs switch <nome>` |
| `ccs trabalho` / `ccs switch trabalho` | Copia o snapshot para `auth.json` e marca `active`. |
| `ccs add trabalho <chave>` | Cria snapshot inativo. Não grava `auth.json`. |
| `ccs save trabalho` | Snapshot da `auth.json` atual no nome. |
| `ccs remove trabalho` | Apaga do ledger. Recusa se o `apiKey` é o da `auth.json` ativa. |
| `ccs verify` | GET `https://api.commandcode.ai/alpha/whoami` com `Authorization: Bearer <apiKey>`. |

Nomes: trim, não vazios, sem `/`, case-sensitive.

`add` preenche `keyName=cli-manual-entry`, `authenticatedAt` agora, `userName` = nome do perfil.

## Erros (stderr, exit 1, sem key)

- Store/auth ausente no `save`/`verify`: `não achei o auth do Command Code em <dir>. Rode cmd login.`
- Nome inexistente: `conta %q não existe. Contas Command Code: …`
- Nome duplicado no add/save (outra chave): `já existe conta Command Code chamada %q. Use outro nome ou remova a atual.`
- `add` sem chave: `informe a chave: ccs add <nome> <chave>`
- Nome inválido: igual ao ocgs
- `remove` da ativa: `%q está ativa. Troque com ccs switch <outra> antes de remover.`
- `verify` sem chave: `não há chave Command Code ativa para verificar.`
- HTTP 401/403: `a chave ativa foi recusada pela API Command Code (HTTP %d). Troque de conta ou gere outra chave.`
- Outro HTTP / rede: mesmo molde do ocgs, texto “Command Code”

## Testes

TDD, `t.TempDir()`, `gofakeit`, `httptest`. Nunca o HOME real. Não imprimir keys.
