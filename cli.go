package carrier

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

// CLIResult is the output of one carrier invocation.
type CLIResult struct {
	Code   int
	Stdout string
	Stderr string
}

// CLIRunner executes a binary with an argument list. It must not use a shell.
type CLIRunner func(ctx context.Context, binary string, args []string) (CLIResult, error)

// CLI runs catalogued carrier commands.
type CLI struct {
	binary string
	run    CLIRunner
	strict bool
}

// CLIConfig configures a CLI runner.
type CLIConfig func(*CLI)

// WithCLIBinary overrides the carrier executable name.
func WithCLIBinary(binary string) CLIConfig {
	return func(cli *CLI) {
		cli.binary = binary
	}
}

// WithCLIRunner replaces process execution. Tests use this.
func WithCLIRunner(runner CLIRunner) CLIConfig {
	return func(cli *CLI) {
		cli.run = runner
	}
}

// WithCLIStrict reports unknown commands before a process starts. The default is strict.
func WithCLIStrict(strict bool) CLIConfig {
	return func(cli *CLI) {
		cli.strict = strict
	}
}

// NewCLI returns a runner for the carrier binary.
func NewCLI(opts ...CLIConfig) (*CLI, error) {
	cli := &CLI{binary: "carrier", run: execCLI, strict: true}
	for _, opt := range opts {
		opt(cli)
	}
	if strings.TrimSpace(cli.binary) == "" || strings.Contains(cli.binary, "\x00") {
		return nil, invalidRequest("invalid_cli", "The carrier binary path is not valid.")
	}
	if cli.run == nil {
		cli.run = execCLI
	}
	return cli, nil
}

// Describe returns one catalog entry, such as "carrier subscribers list".
func (c *CLI) Describe(invocation string) (CLICommand, error) {
	commands, err := CLICommands()
	if err != nil {
		return CLICommand{}, err
	}
	for _, command := range commands {
		if command.Invocation == invocation {
			return command, nil
		}
	}
	return CLICommand{}, invalidRequest("unknown_command", "Unknown command "+invocation+".")
}

// Run executes carrier with args. Strict mode rejects commands missing from the catalog.
func (c *CLI) Run(ctx context.Context, args []string) (CLIResult, error) {
	argv, err := validateCLIArgs(args)
	if err != nil {
		return CLIResult{}, err
	}
	if c.strict {
		command, err := commandForArgs(argv)
		if err != nil {
			return CLIResult{}, err
		}
		if command == nil {
			joined := strings.Join(argv, " ")
			if joined == "" {
				joined = "(empty)"
			}
			return CLIResult{}, invalidRequest("unknown_command", "Unknown command carrier "+joined+".")
		}
	}
	return c.run(ctx, c.binary, argv)
}

func commandForArgs(args []string) (*CLICommand, error) {
	commands, err := CLICommands()
	if err != nil {
		return nil, err
	}
	var best *CLICommand
	bestLen := -1
	for i := range commands {
		tokens := strings.Split(commands[i].Invocation, " ")[1:]
		if len(tokens) > len(args) || len(tokens) <= bestLen {
			continue
		}
		match := true
		for index, token := range tokens {
			if args[index] != token {
				match = false
				break
			}
		}
		if match {
			best = &commands[i]
			bestLen = len(tokens)
		}
	}
	return best, nil
}

func validateCLIArgs(args []string) ([]string, error) {
	argv := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.Contains(arg, "\x00") {
			return nil, invalidRequest("invalid_argument", "CLI arguments must be strings without NUL bytes.")
		}
		argv = append(argv, arg)
	}
	return argv, nil
}

func execCLI(ctx context.Context, binary string, args []string) (CLIResult, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CLIResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.Code = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return result, nil
		}
		return CLIResult{}, apiFailure(0, "cli_not_found", "Could not run "+binary+".", "api_error")
	}
	return result, nil
}
