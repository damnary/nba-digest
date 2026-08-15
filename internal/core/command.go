package core

type CommandKind string

const (
	CommandStart      CommandKind = "start"
	CommandHelp       CommandKind = "help"
	CommandTeams      CommandKind = "teams"
	CommandToggleTeam CommandKind = "toggle_team"
	CommandAlerts     CommandKind = "alerts"
	CommandStop       CommandKind = "stop"
	CommandUnknown    CommandKind = "unknown"
)

type Command struct {
	ChatID int64
	Kind   CommandKind
	League League
	Team   TeamCode
	Enable bool
}

type ReplyKind string

const (
	ReplyWelcome     ReplyKind = "welcome"
	ReplyTeams       ReplyKind = "teams"
	ReplyTeamAdded   ReplyKind = "team_added"
	ReplyTeamRemoved ReplyKind = "team_removed"
	ReplyAlerts      ReplyKind = "alerts"
	ReplyStopped     ReplyKind = "stopped"
	ReplyHelp        ReplyKind = "help"
	ReplyUnknownTeam ReplyKind = "unknown_team"
)

type TeamOption struct {
	Team     Team
	Selected bool
}

type Reply struct {
	Kind    ReplyKind
	Teams   []TeamOption
	Team    Team
	Enabled bool
}

func (r Reply) Selected() []Team {
	var out []Team
	for _, o := range r.Teams {
		if o.Selected {
			out = append(out, o.Team)
		}
	}
	return out
}
