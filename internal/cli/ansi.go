package cli

import "fmt"

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
