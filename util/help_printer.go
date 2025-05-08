// Code Generation by ChatGPT o4-mini-high with this specification:
// 1. Core Layout
//   - Sections in order:
//     NAME
//     USAGE
//     VERSION   (app level only)
//     DESCRIPTION
//     COMMANDS   (only user-defined, hide built-in "help")
//     OPTIONS
//     COPYRIGHT (app level only)
//
// 2. Section Headers
//   - "NAME:", "USAGE:", "VERSION:", "DESCRIPTION:", "COMMANDS:", "OPTIONS:" and "COPYRIGHT:" in bold green.
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

// getTermWidth returns the terminal width or defaultWidth if unavailable.
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

// PrettierHelpPrinter installs a custom HelpPrinter for urfave/cli/v2.
func PrettierHelpPrinter() {
	// wrap splits text into lines of at most width, preserving paragraph breaks.
	wrap := func(text string, width int) []string {
		var lines []string
		for _, para := range strings.Split(text, "\n\n") {
			words := strings.Fields(para)
			if len(words) == 0 {
				lines = append(lines, "")
				continue
			}
			line := words[0]
			for _, w := range words[1:] {
				if len(line)+1+len(w) > width {
					lines = append(lines, line)
					line = w
				} else {
					line += " " + w
				}
			}
			lines = append(lines, line, "")
		}
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		return lines
	}

	// flagCategory extracts the Category field from a cli.Flag via reflection.
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
	// flagHidden extracts the Hidden field from a cli.Flag via reflection.
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
	sectionColor := color.New(color.FgGreen, color.Bold).SprintFunc()
	headerColor := color.New(color.FgCyan, color.Bold).SprintFunc()

	const (
		descIndent  = "   "
		flagIndent  = "  "
		gapBetween  = 2
		extraIndent = "  "
	)
	wrapWidth := min(160, getTermWidth(160)) - 4

	// override the default HelpPrinter
	cli.HelpPrinter = func(w io.Writer, templ string, data interface{}) {
		var (
			flags     []cli.Flag
			cmds      []*cli.Command
			helpName  string
			usage     string
			desc      string
			argsUsage string
			isApp     bool
			app       *cli.App
		)

		switch v := data.(type) {
		case *cli.App:
			flags, cmds, helpName, usage, desc = v.Flags, v.Commands, v.HelpName, v.Usage, v.Description
			isApp, app = true, v
		case *cli.Command:
			flags, cmds, helpName, usage, desc = v.Flags, v.Subcommands, v.HelpName, v.Usage, v.Description
			argsUsage = v.ArgsUsage
		default:
			cli.HelpPrinter(w, templ, data)
			return
		}

		// NAME
		fmt.Fprintf(w, "%s\n%s%s - %s\n\n",
			sectionColor("NAME:"), descIndent, helpName, usage,
		)

		// USAGE
		fmt.Fprintf(w, "%s\n%s", sectionColor("USAGE:"), descIndent)
		if isApp {
			fmt.Fprintf(w, "%s [global options] command [command options]\n\n", helpName)
		} else {
			fmt.Fprintf(w, "%s", helpName)
			if len(flags) > 0 {
				fmt.Fprint(w, " [command options]")
			}
			if argsUsage != "" {
				fmt.Fprintf(w, " %s", argsUsage)
			}
			fmt.Fprint(w, "\n\n")
		}

		// VERSION (app level)
		if isApp && app.Version != "" {
			fmt.Fprintf(w, "%s\n%s%s\n\n",
				sectionColor("VERSION:"), descIndent, app.Version,
			)
		}

		// DESCRIPTION
		if desc != "" {
			fmt.Fprintln(w, sectionColor("DESCRIPTION:"))
			for _, line := range wrap(desc, wrapWidth-len(descIndent)) {
				fmt.Fprintf(w, "%s%s\n", descIndent, line)
			}
			fmt.Fprint(w, "\n")
		}

		// COMMANDS (omit hidden and built-in help)
		var visibleCmds []*cli.Command
		for _, c := range cmds {
			if !c.Hidden && c.Name != "help" {
				visibleCmds = append(visibleCmds, c)
			}
		}
		if len(visibleCmds) > 0 {
			fmt.Fprintln(w, sectionColor("COMMANDS:"))
			for _, c := range visibleCmds {
				fmt.Fprintf(w, "%s%-20s  %s\n", descIndent, c.FullName(), c.Usage)
			}
			fmt.Fprint(w, "\n")
		}

		// OPTIONS / GLOBAL OPTIONS
		if len(flags) == 0 {
			// no flags, skip
			return
		}
		optsHeader := "OPTIONS:"
		if isApp {
			optsHeader = "GLOBAL OPTIONS:"
		}
		fmt.Fprintln(w, sectionColor(optsHeader))

		// group flags by category
		byCat := map[string][]cli.Flag{}
		var cats []string
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
			if _, seen := byCat[cat]; !seen {
				cats = append(cats, cat)
			}
			byCat[cat] = append(byCat[cat], f)
		}
		sort.Strings(cats)

		// compute max label width for alignment
		maxLabel := 0
		for _, cat := range cats {
			for _, f := range byCat[cat] {
				lbl := strings.SplitN(strings.TrimRight(f.String(), "\n"), "\t", 2)[0]
				if l := len(lbl); l > maxLabel {
					maxLabel = l
				}
			}
		}

		// render each category
		for _, cat := range cats {
			if cat != "" {
				fmt.Fprintf(w, "%s%s\n", flagIndent, headerColor(cat))
			}
			for _, f := range byCat[cat] {
				raw := strings.TrimRight(f.String(), "\n")
				parts := strings.SplitN(raw, "\t", 2)
				label, usageText := parts[0], ""
				if len(parts) > 1 {
					usageText = parts[1]
				}
				wrapLen := wrapWidth - len(flagIndent) - maxLabel - gapBetween
				lines := wrap(usageText, wrapLen)

				// first line
				pad := maxLabel - len(label)
				fmt.Fprintf(w, "%s%s%s%s%s\n",
					flagIndent, label,
					strings.Repeat(" ", pad),
					strings.Repeat(" ", gapBetween),
					lines[0],
				)
				// continuation lines
				contIndent := flagIndent +
					strings.Repeat(" ", maxLabel+gapBetween) +
					extraIndent
				for _, cont := range lines[1:] {
					fmt.Fprintf(w, "%s%s\n", contIndent, cont)
				}
			}
			fmt.Fprint(w, "\n")
		}

		// COPYRIGHT (app level)
		if isApp && strings.TrimSpace(app.Copyright) != "" {
			fmt.Fprintln(w, sectionColor("COPYRIGHT:"))
			for _, line := range strings.Split(app.Copyright, "\n") {
				fmt.Fprintf(w, "%s%s\n", descIndent, line)
			}
		}
	}
}
