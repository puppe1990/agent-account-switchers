# CLI `ocgs` — troca de contas OpenCode Go

Data: 2026-09-23  
Status: draft para revisão

## Objetivo

CLI em Go para listar, adicionar, salvar, trocar, remover e verificar contas **somente** do provedor OpenCode Go (`serviceID` / chave `opencode-go`). Outros provedores no store do OpenCode permanecem intactos.

Comando do binário: `ocgs`.  
Módulo: `agent-account-switchers`.  
Go: 1.24+. Dependências de produção: Cobra. Testes: `testing` da stdlib + `gofakeit`.

## Fora de escopo

- Troca de Copilot, OpenAI, Zen, OpenRouter ou qualquer outro provedor
- Store paralelo de chaves (`~/.opencode/keys`, config próprio)
- Picker interativo (prompt/TUI)
- Auto-rotate em 429 / quota
- Sessões OpenCode já abertas (o switch vale para o próximo launch)
- Impressão de API keys no stdout/stderr

## Arquitetura

O `main` só monta dependências. A lógica mora em pacotes internos.

```
cmd/ocgs/          main: Store + stdout + cliente HTTP
internal/store/    account.json v2 + sync de auth.json
internal/cli/      Cobra: list, switch, add, save, remove, verify
internal/verify/   GET na API Go, cliente HTTP injetável
```

Diretório de dados (igual ao OpenCode 1.18 neste Mac):

1. `$OPENCODE_DATA_DIR` se definido (só para testes/override explícito)
2. `$XDG_DATA_HOME/opencode` se `XDG_DATA_HOME` estiver definido
3. `~/.local/share/opencode`

Arquivos:

- `<dataDir>/account.json` — store v2 (`version`, `accounts`, `active`)
- `<dataDir>/auth.json` — mapa legado provedor → credential

Nenhum pacote interno chama `os.UserHomeDir`. O diretório entra no construtor do `Store`. Escrita atômica (arquivo temp no mesmo diretório + rename), modo `0600`.

## Modelo de dados

`account.json` (v2), trecho relevante:

```json
{
  "version": 2,
  "accounts": {
    "<id>": {
      "id": "<id>",
      "serviceID": "opencode-go",
      "description": "trabalho",
      "credential": { "type": "api", "key": "sk-..." }
    }
  },
  "active": {
    "opencode-go": "<id>"
  }
}
```

O nome da conta é `description`. IDs novos: 26 caracteres Crockford Base32 (formato ULID), gerados com `crypto/rand` + timestamp, únicos no mapa `accounts`.

`auth.json`, chave `opencode-go`:

```json
{
  "opencode-go": { "type": "api", "key": "sk-..." }
}
```

O `Store` só cria, atualiza, ativa e apaga entradas com `serviceID == "opencode-go"`. Demais chaves de `accounts`, `active` e `auth.json` são relidas e regravadas sem alteração.

## Comandos

Cobra no `internal/cli`. Subcomandos oficiais; se o primeiro argumento não for subcomando nem flag, o root trata como `switch <nome>`.

| Invocação | Efeito |
|---|---|
| `ocgs` / `ocgs list` | Lista contas Go. Marca a ativa com `→`. Não imprime chaves. |
| `ocgs switch` sem nome | Erro de uso: `informe a conta: ocgs switch <nome>` |
| `ocgs trabalho` / `ocgs switch trabalho` | Ativa a conta cujo `description` é `trabalho`: atualiza `active["opencode-go"]` e copia o credential para `auth.json["opencode-go"]`. Se duas contas Go tiverem o mesmo `description`, recusa e pede `save` para desambiguar. |
| `ocgs add trabalho sk-…` | Cria conta nova. **Não** ativa. `description` único entre contas Go. |
| `ocgs save trabalho` | Grava o `description` da conta Go **ativa** como `trabalho`. Se a ativa já tem esse nome, sucesso sem escrita. |
| `ocgs remove trabalho` | Remove a conta do mapa. Recusa se for a ativa. Se duas contas Go tiverem o mesmo nome, recusa. |
| `ocgs verify` | Lê a chave ativa e faz GET `https://opencode.ai/zen/go/v1/models` com `Authorization: Bearer <key>`. HTTP 2xx = ok. |

Nomes: não vazios, sem `/`, espaços nas pontas removidos. Comparação **case-sensitive**.

`add` não grava `auth.json`. Só `switch` (e o positional equivalente) sincroniza o legado.

`save` na conta ativa que já se chama `trabalho` é no-op de sucesso.

## Tratamento de erros

Stderr + exit 1. Sem stack. Sem API key na mensagem.

| Situação | Mensagem |
|---|---|
| Store ausente | `não achei o store do OpenCode em <dir>. Abra o OpenCode uma vez ou rode opencode auth login.` |
| JSON inválido | `account.json em <path> não é um JSON v2 válido: <motivo>` |
| Nome inexistente | `conta "trabalho" não existe. Contas Go: pessoal, cliente.` (lista os nomes existentes; se vazia, diz que não há contas Go) |
| Nome duplicado no store (switch/remove) | `há 2 contas Go chamadas "default". Renomeie a ativa com ocgs save <nome-único>.` |
| Nome duplicado em add/save | `já existe conta Go chamada "trabalho". Use outro nome ou remova a atual.` |
| `add` sem key / key vazia | `informe a chave: ocgs add <nome> <chave>` |
| Nome inválido | `nome de conta inválido: "<nome>". Use um nome sem "/" e não vazio.` |
| `save` sem conta Go ativa | `não há conta OpenCode Go ativa para salvar. Faça login ou ocgs add … e ocgs switch.` |
| `remove` da conta ativa | `"trabalho" está ativa. Troque com ocgs switch <outra> antes de remover.` |
| `verify` sem chave ativa | `não há chave Go ativa para verificar.` |
| API recusou (401/403) | `a chave ativa foi recusada pela API OpenCode Go (HTTP 401). Troque de conta ou gere outra chave.` |
| API outro HTTP | `a API OpenCode Go respondeu HTTP <code> ao verificar a chave ativa.` |
| Rede/timeout | `não consegui falar com a API OpenCode Go em <url>: <erro>. O switch local não depende disso.` |
| Falha ao gravar | `não consegui gravar <path>: <erro>. Nada foi alterado.` |

Write atômico: se o rename falhar, o arquivo original permanece.

## Testes (TDD + faker)

Lei: nenhum código de produção sem teste falhando antes.

- `gofakeit` gera `description` (username/letter) e keys (`sk-` + alfanumérico). Cada teste usa dados frescos; fixtures fixas só para o envelope v2 e provedores vizinhos.
- `Store` recebe `t.TempDir()` populado com `account.json` + `auth.json` de fixture. Nunca lê o HOME real.
- Fixture mínima inclui pelo menos um provedor não-Go (ex.: `openai`) para assertir que `active["openai"]` e `auth.json["openai"]` não mudam.
- `verify` usa `httptest.Server`; o cliente aponta para essa URL. Timeout curto e injetável.
- CLI: `bytes.Buffer` em stdout/stderr, `Run(args)` no Cobra. Assert de texto visível e de exit via `error`.
- Um comportamento por teste. Nomes descrevem o comportamento (`Switch_updatesActiveAndAuthJSON`).

Cobertura mínima (cada item vira teste RED→GREEN):

1. List marca a conta ativa e omite a key
2. Switch por subcomando e por positional atualizam `active` + `auth.json`
3. Switch preserva outros provedores
4. Add cria conta inativa
5. Add recusa nome duplicado
6. Save renomeia a ativa
7. Remove apaga conta inativa
8. Remove recusa a ativa
9. Erros da tabela acima (store ausente, nome inexistente, save sem ativa, verify 401)
10. Verify 2xx no `httptest`

## Qualidade

- `go test ./...` é o gate
- Arquivos de credencial `0600`
- Sem log da key
- README curto: install (`go install` / build), comandos, aviso de que sessões já abertas não mudam

## Decisões já fechadas

- Escopo: só OpenCode Go
- Entrada de contas: `save` (snapshot da ativa) e `add` (key crua)
- UX: `ocgs list` / `ocgs switch nome` e também `ocgs nome`; sem args = list
- Persistência: `account.json` nativo + sync de `auth.json`
- Verify: comando opcional, não roda no switch
- Forma: pacote de domínio + CLI fino (Cobra)
