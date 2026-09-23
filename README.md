# ocgs / ccs

CLIs em Go para trocar contas de agentes de código.

- **`ocgs`** — OpenCode Go (`~/.local/share/opencode/account.json` + `auth.json`)
- **`ccs`** — Command Code / `cmd` (`~/.commandcode/auth.json` + `ccs-accounts.json`)

Outros provedores (Copilot, OpenAI, Zen, …) não são alterados. Sessões OpenCode já abertas continuam com a conta antiga; o switch vale no próximo launch.

## Install

Neste repo:

```bash
go install ./cmd/ocgs ./cmd/ccs
go build -o ocgs ./cmd/ocgs
go build -o ccs ./cmd/ccs
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
