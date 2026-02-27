package cli

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ansi wraps text with an SGR escape code.
func ansi(code int, text string) string {
	return fmt.Sprintf("\033[%dm%s\033[0m", code, text)
}

// bold renders text in bold.
func bold(text string) string {
	return ansi(1, text)
}

// dim renders text in dim/faint style.
func dim(text string) string {
	return ansi(2, text)
}

// fg wraps text with a 256-color foreground escape.
func fg(color int, text string) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", color, text)
}

// fgBold wraps text with a 256-color foreground and bold.
func fgBold(color int, text string) string {
	return fmt.Sprintf("\033[1;38;5;%dm%s\033[0m", color, text)
}

// stdoutIsTTY returns true when stdout is a terminal.
func stdoutIsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// maybeBold returns bold text in TTY mode, plain text otherwise.
func maybeBold(tty bool, text string) string {
	if tty {
		return bold(text)
	}
	return text
}

// maybeDim returns dim text in TTY mode, plain text otherwise.
func maybeDim(tty bool, text string) string {
	if tty {
		return dim(text)
	}
	return text
}

// maybeFgBold returns colored bold text in TTY mode, plain text otherwise.
func maybeFgBold(tty bool, color int, text string) string {
	if tty {
		return fgBold(color, text)
	}
	return text
}
