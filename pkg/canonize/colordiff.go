package canonize

import "strings"

type Color = string

const (
	colorRed   Color = "\033[31m"
	colorGreen Color = "\033[32m"
	colorCyan  Color = "\033[36m"
	colorReset Color = "\033[0m"
)

func addColorsToDiff(text string) string {
	var coloredText strings.Builder
	for l := range strings.Lines(text) {
		if strings.HasPrefix(l, "+") {
			coloredText.WriteString(colorGreen)
		} else if strings.HasPrefix(l, "-") {
			coloredText.WriteString(colorRed)
		} else if strings.HasPrefix(l, "@@") {
			coloredText.WriteString(colorCyan)
		}
		coloredText.WriteString(l)
		coloredText.WriteString(colorReset)
	}
	return coloredText.String()
}
