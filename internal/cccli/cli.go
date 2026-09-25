package cccli

import (
	"fmt"
	"io"

	"agent-account-switchers/internal/ccstore"

	"github.com/spf13/cobra"
)

type Verifier interface {
	Verify(key string) error
}

func NormalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	if len(args[0]) > 0 && args[0][0] == '-' {
		return args
	}
	switch args[0] {
	case "list", "switch", "add", "save", "remove", "verify", "help", "completion":
		return args
	default:
		return append([]string{"switch"}, args...)
	}
}

func New(s *ccstore.Store, v Verifier, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "ccs",
		Short:         "Troca contas Command Code",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(s, stdout)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Lista contas Command Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(s, stdout)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "switch [nome]",
		Short: "Ativa uma conta Command Code",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("informe a conta: ccs switch <nome>")
			}
			return s.Switch(args[0])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "add <nome> <chave>",
		Short: "Adiciona uma conta Command Code",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("informe a chave: ccs add <nome> <chave>")
			}
			return s.Add(args[0], args[1])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "save <nome>",
		Short: "Salva a conta Command Code ativa com um nome",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("informe a conta: ccs save <nome>")
			}
			return s.Save(args[0])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "remove <nome>",
		Short: "Remove uma conta Command Code inativa",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("informe a conta: ccs remove <nome>")
			}
			return s.Remove(args[0])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verifica a chave Command Code ativa na API",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := s.ActiveKey()
			if err != nil {
				return err
			}
			if err := v.Verify(key); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "chave Command Code ativa é válida.")
			return err
		},
	})
	return root
}

func runList(s *ccstore.Store, stdout io.Writer) error {
	entries, err := s.List()
	if err != nil {
		return err
	}
	for _, e := range entries {
		prefix := "  "
		if e.Active {
			prefix = "→ "
		}
		if _, err := fmt.Fprintf(stdout, "%s%s\n", prefix, e.Name); err != nil {
			return err
		}
	}
	return nil
}
