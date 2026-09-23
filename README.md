# ocgs

CLI em Go para trocar contas **OpenCode Go** no store nativo do OpenCode 1.18 (`~/.local/share/opencode/account.json` + `auth.json`).

Outros provedores (Copilot, OpenAI, Zen, …) não são alterados. Sessões OpenCode já abertas continuam com a conta antiga; o switch vale no próximo launch.

## Install

Neste repo:

```bash
go install ./cmd/ocgs
go build -o ocgs ./cmd/ocgs
```

Override do diretório: `OPENCODE_DATA_DIR=/caminho ocgs list`.

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

Arquivos de credencial são gravados com modo `0600`. A CLI nunca imprime a API key.
