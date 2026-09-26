# agent-account-switchers

CLIs em Go para trocar contas de agentes de código: **`ocgs`** (OpenCode Go), **`ccs`** (Command Code / `cmd`) e **`switcher-ui`** (interface web local).

- **`ocgs`** — OpenCode Go (`~/.local/share/opencode/account.json` + `auth.json`)
- **`ccs`** — Command Code / `cmd` (`~/.commandcode/auth.json` + `ccs-accounts.json`)
- **`switcher-ui`** — UI web local (Tailwind) para trocar as contas de todos os provedores

Outros provedores (Copilot, Zen, …) não são alterados. Sessões OpenCode já abertas continuam com a conta antiga; o switch vale no próximo launch.

## Install

Clone [puppe1990/agent-account-switchers](https://github.com/puppe1990/agent-account-switchers) e, na raiz do repo:

```bash
go install ./cmd/ocgs ./cmd/ccs ./cmd/switcher-ui
go build -o ocgs ./cmd/ocgs
go build -o ccs ./cmd/ccs
go build -o switcher-ui ./cmd/switcher-ui
```

Override: `OPENCODE_DATA_DIR=/caminho ocgs list`, `CCS_DATA_DIR=/caminho ccs list`.

## Comandos

```
ocgs                     # lista contas Go (→ na ativa)
ocgs list
ocgs trabalho            # troca para "trabalho"
ocgs switch trabalho
ocgs add trabalho sk-…   # cria, não ativa
ocgs save trabalho       # nomeia a conta Go ativa
ocgs remove trabalho     # apaga conta inativa
ocgs verify              # GET na API Go com a chave ativa
```

```
ccs                      # lista contas Command Code (→ na ativa)
ccs list
ccs trabalho             # troca para "trabalho"
ccs switch trabalho
ccs add trabalho user-…  # cria, não ativa
ccs save trabalho        # snapshot da auth.json atual
ccs remove trabalho      # apaga conta inativa
ccs verify               # GET /alpha/whoami com a chave ativa
```

Arquivos de credencial são gravados com modo `0600`. A CLI nunca imprime a API key. Sessões já abertas do `cmd` ou do OpenCode continuam com a conta antiga.

## Interface web

`switcher-ui` sobe um servidor local com uma página Tailwind para trocar as contas do OpenCode Go, do Command Code, do Grok e do Codex com um clique.

```sh
switcher-ui                 # abre http://127.0.0.1:8765
switcher-ui --port 9000     # outra porta
switcher-ui --no-open       # não abrir o navegador
```

- Escuta só em loopback (`127.0.0.1`); ninguém na rede acessa.
- A página usa o Tailwind via CDN (precisa de internet para o estilo).
- Endpoints: `GET /api/accounts`, `POST /api/switch` (`{"service","name"}`), `POST /api/login` (`{"service"}`) e `GET /api/login?service=…`.
- Nunca expõe chaves — só o nome e qual conta está ativa.
- `SWITCHER_UI_PORT` define a porta; `OPENCODE_DATA_DIR`/`CCS_DATA_DIR`/`GROK_HOME`/`CODEX_HOME` os diretórios.
- O serviço Grok lê os perfis de `~/.grok/accounts` (formato do [grok-accounts](https://github.com/puppe1990/grok-accounts)) e troca reescrevendo `~/.grok/auth.json`.
- O serviço Codex lê as contas do registry do [codex-auth](https://github.com/Loongphy/codex-auth) em `~/.codex/accounts/registry.json` e troca reescrevendo `~/.codex/auth.json`.
- No card do Codex, o botão **login anônimo** roda `codex login` em segundo plano, abre a URL de autenticação numa janela anônima (Brave primeiro; depois Chrome/Edge, e por fim o navegador padrão) e registra a conta no registry ao final. É o jeito de entrar com outra conta sem deslogar a atual — deixe o `switcher-ui` aberto até concluir o login.

## Qualidade

Lint, formatação e testes rodam no CI (`.github/workflows/ci.yml`) e num hook de pre-commit.

```sh
pnpm install                 # deps de formatação (prettier)
make verify                  # gofmt + prettier --check + go vet + golangci-lint + go test + go build
make test                    # só os testes

git config core.hooksPath .githooks   # ativa o pre-commit (gofmt + prettier + go test)
```
