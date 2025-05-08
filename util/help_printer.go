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
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

const (
	// indentation and wrapping parameters
	descIndent   = "   "
	flagIndent   = "  "
	gapBetween   = 2
	cmdNameWidth = 20
	minWrap      = 40
	maxWrap      = 160
)

// template for the entire help output
const helpTpl = `{{ section "NAME:" }}
   {{ .Name }} - {{ .Usage }}

{{ section "USAGE:" }}
   {{ .UsageLine }}
{{ if .Version }}
{{ section "VERSION:" }}
   {{ .Version }}
{{ end }}
{{- if .DescriptionLines }}
{{ section "DESCRIPTION:" }}
{{- range .DescriptionLines }}
   {{ . }}
{{- end }}
{{ end }}
{{- if .Commands }}
{{ section "COMMANDS:" }}
{{- range .Commands }}
   {{ .Name | padCmd }}  {{ .Usage }}
{{- end }}
{{ end }}
{{ section .OptionsHeader }}
{{- range .FlagGroups }}
  {{- if .Category }}
{{ header .Category }}
  {{- end }}
  {{- range .Flags }}
  {{- if $.Stacked }}
{{ .Label }}
     {{- range .UsageLines }}
{{ $.UsageIndent }}{{ . }}
     {{- end }}
  {{ else }}
{{ pad .Label $.MaxLabel }}  {{ index .UsageLines 0 }}
   {{- range $i, $line := .UsageLines }}
     {{- if gt $i 0 }}
{{ $.UsageIndent }}{{ $line }}
     {{- end }}
  {{- end }}
   {{- end }}
  {{- end }}
{{ end }}
{{- if .CopyrightLines }}
{{ section "COPYRIGHT:" }}
{{- range .CopyrightLines }}
   {{ . }}
{{- end }}
{{ end }}`

var (
	// colors
	sectionColor = color.New(color.FgGreen, color.Bold).SprintFunc()
	headerColor  = color.New(color.FgCyan, color.Bold).SprintFunc()

	// template.FuncMap for coloring, padding, and headers
	funcMap = template.FuncMap{
		"section": func(s string) string { return sectionColor(s) },
		"header":  func(s string) string { return flagIndent + headerColor(s) },
		"pad": func(label string, width int) string {
			if n := width - len(label); n > 0 {
				return label + strings.Repeat(" ", n)
			}
			return label
		},
		"padCmd": func(name string) string {
			if n := cmdNameWidth - len(name); n > 0 {
				return name + strings.Repeat(" ", n)
			}
			return name
		},
	}

	// parsed template
	helpT = template.Must(template.New("help").Funcs(funcMap).Parse(helpTpl))
)

// helpData is the model passed to the template.
type helpData struct {
	Name             string
	Usage            string
	UsageLine        string
	Version          string
	DescriptionLines []string
	Commands         []cmdItem
	OptionsHeader    string
	FlagGroups       []flagGroup
	MaxLabel         int
	UsageIndent      string
	CopyrightLines   []string

	Stacked bool
}

type cmdItem struct {
	Name  string
	Usage string
}

type flagGroup struct {
	Category string
	Flags    []flagItem
}

type flagItem struct {
	Label      string
	UsageLines []string
}

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

// PrettierHelpPrinter installs the templated HelpPrinter into urfave/cli.
func PrettierHelpPrinter() {
	original := cli.HelpPrinter
	cli.HelpPrinter = func(w io.Writer, templ string, data interface{}) {
		d := buildHelpData(data)
		if err := helpT.ExecuteTemplate(w, "help", d); err != nil {
			// fallback to default on error
			original(w, templ, data)
		}
	}
}

// wrap splits text into lines of at most width, preserving paragraph breaks.
func wrap(text string, width int) []string {
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
func flagCategory(f cli.Flag) string {
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
func flagHidden(f cli.Flag) bool {
	v := reflect.ValueOf(f)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if fld := v.FieldByName("Hidden"); fld.IsValid() && fld.Kind() == reflect.Bool {
		return fld.Bool()
	}
	return false
}

// buildHelpData extracts flags, commands, and metadata into helpData.
func buildHelpData(data interface{}) *helpData {
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
		return &helpData{Name: helpName, Usage: usage}
	}

	termWidth := getTermWidth(maxWrap)
	// compute wrap width
	wrapWidth := min(maxWrap, termWidth) - 2

	// usage line
	var usageLine string
	if isApp {
		usageLine = helpName + " [global options] command [command options]"
	} else {
		usageLine = helpName
		if len(flags) > 0 {
			usageLine += " [command options]"
		}
		if argsUsage != "" {
			usageLine += " " + argsUsage
		}
	}

	d := &helpData{
		Name:      helpName,
		Usage:     usage,
		UsageLine: usageLine,
	}

	// version
	if isApp {
		d.Version = app.Version
	}

	// description
	if desc != "" {
		d.DescriptionLines = wrap(desc, wrapWidth-len(descIndent))
	}

	// commands
	for _, c := range cmds {
		if c.Hidden || c.Name == "help" {
			continue
		}
		d.Commands = append(d.Commands, cmdItem{
			Name:  c.FullName(),
			Usage: c.Usage,
		})
	}

	// options header
	if isApp {
		d.OptionsHeader = "GLOBAL OPTIONS:"
	} else {
		d.OptionsHeader = "OPTIONS:"
	}

	// group flags by category
	byCat := make(map[string][]cli.Flag)
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
		if _, ok := byCat[cat]; !ok {
			cats = append(cats, cat)
		}
		byCat[cat] = append(byCat[cat], f)
	}
	sort.Strings(cats)

	// compute max label width
	for _, cat := range cats {
		for _, f := range byCat[cat] {
			lbl := strings.SplitN(strings.TrimRight(f.String(), "\n"), "\t", 2)[0]
			if l := len(lbl); l > d.MaxLabel {
				d.MaxLabel = l + 2
			}
		}
	}

	// usage indent for continuations
	d.UsageIndent = flagIndent + strings.Repeat(" ", d.MaxLabel+gapBetween)
	wlen := wrapWidth - len(flagIndent) - d.MaxLabel - gapBetween
	if wlen < minWrap {
		d.Stacked = true
		d.UsageIndent = flagIndent + "  "
	}

	// assemble flag groups
	for _, cat := range cats {
		group := flagGroup{Category: cat}
		for _, f := range byCat[cat] {
			raw := strings.TrimRight(f.String(), "\n")
			parts := strings.SplitN(raw, "\t", 2)
			label := flagIndent + parts[0]
			usageText := ""
			if len(parts) > 1 {
				usageText = parts[1]
			}
			var lines []string
			if d.Stacked {
				lines = wrap(usageText, termWidth-4)
			} else {
				lines = wrap(usageText, wlen)
			}
			group.Flags = append(group.Flags, flagItem{
				Label:      label,
				UsageLines: lines,
			})
		}
		d.FlagGroups = append(d.FlagGroups, group)
	}

	// copyright
	if isApp && strings.TrimSpace(app.Copyright) != "" {
		for line := range strings.SplitSeq(app.Copyright, "\n") {
			d.CopyrightLines = append(d.CopyrightLines, line)
		}
	}

	return d
}
