// Code Generation by ChatGPT o4-mini-high with this specification:
// 1. Core Layout
//   - Sections in order:
//     NAME
//     USAGE
//     DESCRIPTION
//     COMMANDS   (only user-defined, hide built-in "help")
//     OPTIONS
//
// 2. Section Headers
//   - "NAME:", "USAGE:", "DESCRIPTION:", "COMMANDS:", "OPTIONS:" in bold green.
//   - Category headings (e.g. "Chord Options", "Gateway Options", "Global Options") in bold cyan, indented 2 spaces.
//
// 3. Wrapping & Indentation
//   - Wrap all free-form text (DESCRIPTION and each flag’s usage) at 160 columns.
//   - DESCRIPTION lines indented 3 spaces.
//   - Flags indented 2 spaces.
//   - Two-space gap between flag label and usage.
//   - Wrapped continuation lines of flag usage get an extra 2-space indent.
//
// 4. Flag Grouping & Ordering
//   - Group flags by their `Category` field (via reflection).
//   - Alphabetically sort categories (empty category labeled "Global Options").
//   - Within each category, preserve the order from the `Flags` slice.
//
// 5. Label Alignment
//   - Compute the maximum width of any flag label (e.g. "--foo BAR") across all categories.
//   - Right-pad every other label to that width so all usage columns start in the same column.
//
// 6. Help-flag & Help-command Suppression
//   - Skip any flag whose rendered label starts with `--help`.
//   - Skip the built-in `help` subcommand unless explicitly defined.
//
// 7. Color & Readability
//   - Use `fatih/color` for ANSI colors:
//   - Bright green for top-level headers.
//   - Cyan for category headings.
//   - Insert blank lines before each category and between major sections for breathing room.
package util

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

func getTermWidth(defaultWidth int) int {
	// 1) Direct system call
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}

	// 2) Environment override (e.g. in CI)
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if c, err := strconv.Atoi(cols); err == nil && c > 0 {
			return c
		}
	}

	// 3) fallback
	return defaultWidth
}

func PrettierHelpPrinter() {
	// --- helpers ---

	// word-wrap to a fixed width, preserving paragraphs
	wrap := func(text string, width int) []string {
		var out []string
		for _, para := range strings.Split(text, "\n\n") {
			words := strings.Fields(para)
			if len(words) == 0 {
				out = append(out, "")
				continue
			}
			line := words[0]
			for _, w := range words[1:] {
				if len(line)+1+len(w) > width {
					out = append(out, line)
					line = w
				} else {
					line += " " + w
				}
			}
			out = append(out, line, "")
		}
		if len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		return out
	}

	// reflect Category & Hidden off each concrete cli.Flag
	flagCategory := func(f cli.Flag) string {
		v := reflect.ValueOf(f)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if fld := v.FieldByName("Category"); fld.IsValid() && fld.Kind() == reflect.String {
			return fld.String()
		}
		return ""
	}
	flagHidden := func(f cli.Flag) bool {
		v := reflect.ValueOf(f)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if fld := v.FieldByName("Hidden"); fld.IsValid() && fld.Kind() == reflect.Bool {
			return fld.Bool()
		}
		return false
	}

	// colors
	sectionColor := color.New(color.FgGreen, color.Bold).SprintFunc() // for NAME/USAGE/DESCRIPTION
	headerColor := color.New(color.FgCyan, color.Bold).SprintFunc()   // for categories

	const (
		descIndent  = "   "
		flagIndent  = "  "
		gapBetween  = 2
		extraIndent = "  "
	)
	var wrapWidth = min(160, getTermWidth(160)) - 4

	// --- override HelpPrinter ---
	cli.HelpPrinter = func(w io.Writer, templ string, data interface{}) {
		// a) extract context
		var (
			flags    []cli.Flag
			cmds     []*cli.Command
			helpName string
			usage    string
			desc     string
		)
		switch v := data.(type) {
		case *cli.App:
			flags, cmds, helpName, usage, desc = v.Flags, v.Commands, v.HelpName, v.Usage, v.Description
		case *cli.Command:
			flags, cmds, helpName, usage, desc = v.Flags, v.Subcommands, v.HelpName, v.Usage, v.Description
		default:
			cli.HelpPrinter(w, templ, data)
			return
		}

		// b) NAME
		fmt.Fprintf(w, "%s\n%s%s - %s\n\n",
			sectionColor("NAME:"),
			descIndent, helpName, usage,
		)

		// c) USAGE
		fmt.Fprintf(w, "%s\n%s%s", sectionColor("USAGE:"), descIndent, helpName)
		if len(flags) > 0 {
			fmt.Fprintf(w, " [command options]")
		}
		fmt.Fprint(w, "\n\n")

		// d) DESCRIPTION
		if desc != "" {
			fmt.Fprintf(w, "%s\n", sectionColor("DESCRIPTION:"))
			for _, line := range wrap(desc, wrapWidth-len(descIndent)) {
				fmt.Fprintf(w, "%s%s\n", descIndent, line)
			}
			fmt.Fprint(w, "\n")
		}

		// e) COMMANDS (skip built-in help)
		realCmds := []*cli.Command{}
		for _, c := range cmds {
			if c.Hidden || c.Name == "help" {
				continue
			}
			realCmds = append(realCmds, c)
		}
		if len(realCmds) > 0 {
			fmt.Fprintln(w, sectionColor("COMMANDS:"))
			for _, c := range realCmds {
				fmt.Fprintf(w, "%s%-20s  %s\n",
					descIndent, c.FullName(), c.Usage)
			}
			fmt.Fprint(w, "\n")
		}

		// f) OPTIONS header (now colored)
		if len(flags) == 0 {
			return
		}
		fmt.Fprintf(w, "%s\n\n", sectionColor("OPTIONS:"))

		// g) Group flags…
		byCat := map[string][]cli.Flag{}
		order := []string{}
		for _, f := range flags {
			if flagHidden(f) {
				continue
			}
			raw := strings.TrimRight(f.String(), "\n")
			label := strings.SplitN(raw, "\t", 2)[0]
			if strings.HasPrefix(label, "--help") {
				continue
			}
			cat := flagCategory(f)
			if _, ok := byCat[cat]; !ok {
				order = append(order, cat)
			}
			byCat[cat] = append(byCat[cat], f)
		}
		sort.Strings(order)

		// h) Max label width…
		maxLabel := 0
		for _, cat := range order {
			for _, f := range byCat[cat] {
				lbl := strings.SplitN(strings.TrimRight(f.String(), "\n"), "\t", 2)[0]
				if l := len(lbl); l > maxLabel {
					maxLabel = l
				}
			}
		}

		// i) Render groups with indented, colored headings
		for _, cat := range order {
			header := cat
			if header == "" {
				header = "Global Options"
			}
			// two-space indent for category
			fmt.Fprintf(w, "%s%s\n", flagIndent, headerColor(header))

			for _, f := range byCat[cat] {
				raw := strings.TrimRight(f.String(), "\n")
				parts := strings.SplitN(raw, "\t", 2)
				label := parts[0]
				usageText := ""
				if len(parts) > 1 {
					usageText = parts[1]
				}

				// wrap usage
				wrapLen := wrapWidth - len(flagIndent) - maxLabel - gapBetween
				lines := wrap(usageText, wrapLen)

				// first line
				pad := maxLabel - len(label)
				fmt.Fprintf(w, "%s%s%s%s%s\n",
					flagIndent,
					label,
					strings.Repeat(" ", pad),
					strings.Repeat(" ", gapBetween),
					lines[0],
				)

				// continuation lines get extra indent
				contIndent := flagIndent +
					strings.Repeat(" ", maxLabel+gapBetween) +
					extraIndent
				for _, cont := range lines[1:] {
					fmt.Fprintf(w, "%s%s\n", contIndent, cont)
				}
			}
			fmt.Fprint(w, "\n")
		}
	}
}
