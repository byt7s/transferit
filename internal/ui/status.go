package ui

import (
	"fmt"
	"os"
)

func ShowStatus(pretty bool, title, detail string) {
	if pretty {
		Draw(LayoutStatus(ContentWidth(), title, detail))
		return
	}
	fmt.Fprintf(os.Stderr, "\r\033[K  %s\n", detail)
}
