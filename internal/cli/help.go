package cli

import (
	"regexp"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/ioutil"
)

// flagName matches the flag column of a pflag usage line: "  -v, --verbose"
// or "      --dry-run". The escape codes it adds are invisible, so pflag's
// column alignment survives.
var flagName = regexp.MustCompile(`(?m)^(\s+)((?:-\w, )?--[\w-]+)`)

func colorFlags(usages string) string {
	return flagName.ReplaceAllStringFunc(usages, func(m string) string {
		sub := flagName.FindStringSubmatch(m)
		return sub[1] + ioutil.Cyan(sub[2])
	})
}

// usageTemplate is cobra v1.10.2's defaultUsageTemplate with headings, command
// names and flag names passed through the color helpers. Without color every
// helper returns its input, so the output is byte for byte cobra's own.
const usageTemplate = `{{title "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{title "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{title "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{title "Available Commands:"}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cmdName (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{title .Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{cmdName (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{title "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{cmdName (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{title "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces | flags}}{{end}}{{if .HasAvailableInheritedFlags}}

{{title "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces | flags}}{{end}}{{if .HasHelpSubCommands}}

{{title "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{cmdName (rpad .CommandPath .CommandPathPadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// setColorHelp gives cmd, and every command under it, the colored usage text.
func setColorHelp(cmd *cobra.Command) {
	cobra.AddTemplateFuncs(map[string]any{
		"title":   ioutil.Title,
		"cmdName": ioutil.Cyan,
		"flags":   colorFlags,
	})
	cmd.SetUsageTemplate(usageTemplate)
}
