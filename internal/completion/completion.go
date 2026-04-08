package completion

import (
	"fmt"
	"strings"
)

var commandNames = []string{
	"inspect",
	"compress",
	"config",
	"index",
	"status",
	"clean",
	"version",
	"help",
	"completion",
}

func Commands() []string {
	return append([]string{}, commandNames...)
}

func Generate(shell string) (string, error) {
	joined := strings.Join(commandNames, " ")

	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "bash":
		return fmt.Sprintf(`_mantis_completions() {
  local cur prev words cword
  _init_completion || return

  if [[ $cword -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "%s" -- "$cur") )
  fi
}

complete -F _mantis_completions mantis
`, joined), nil
	case "zsh":
		return fmt.Sprintf(`#compdef mantis

_mantis() {
  local -a commands
  commands=(
    %s
  )
  _describe 'command' commands
}

compdef _mantis mantis
`, zshCommands(commandNames)), nil
	case "fish":
		return fmt.Sprintf(`complete -c mantis -f
%s
`, fishCommands(commandNames)), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish)", shell)
	}
}

func zshCommands(commands []string) string {
	var quoted []string
	for _, command := range commands {
		quoted = append(quoted, fmt.Sprintf("'%s'", command))
	}
	return strings.Join(quoted, "\n    ")
}

func fishCommands(commands []string) string {
	var lines []string
	for _, command := range commands {
		lines = append(lines, fmt.Sprintf("complete -c mantis -n '__fish_use_subcommand' -a %q", command))
	}
	return strings.Join(lines, "\n")
}
