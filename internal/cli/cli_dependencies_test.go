package cli

import (
	"io"
	"strings"
)

func (c *CLI) SetInput(input io.Reader) {
	if input == nil {
		input = strings.NewReader("")
	}
	c.stdin = input
}

func (c *CLI) SetSetupPrompter(prompter SetupPrompter) { c.setupPrompter = prompter }
