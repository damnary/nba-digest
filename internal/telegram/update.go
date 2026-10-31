package telegram

import (
	"strings"

	"github.com/damnary/nba-digest/internal/core"
)

const (
	togglePrefix  = "t:"
	menuData      = "m"
	teamsData     = "teams"
	standingsData = "st"
	alertsData    = "al"
)

type chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type incoming struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      chat   `json:"chat"`
}

type Update struct {
	UpdateID    int64     `json:"update_id"`
	Message     *incoming `json:"message"`
	ChannelPost *incoming `json:"channel_post"`

	CallbackQuery *struct {
		ID      string `json:"id"`
		Data    string `json:"data"`
		Message *struct {
			MessageID int64 `json:"message_id"`
			Chat      chat  `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

type intent struct {
	command    core.Command
	callbackID string
	messageID  int64
	edit       bool
	valid      bool

	foreignChat int64
}

func parseUpdate(u Update) intent {
	switch {
	case u.CallbackQuery != nil && u.CallbackQuery.Message != nil:
		if !isPrivate(u.CallbackQuery.Message.Chat) {
			return intent{foreignChat: u.CallbackQuery.Message.Chat.ID}
		}
		cmd := parseCallback(u.CallbackQuery.Data)
		cmd.ChatID = u.CallbackQuery.Message.Chat.ID
		return intent{
			command:    cmd,
			callbackID: u.CallbackQuery.ID,
			messageID:  u.CallbackQuery.Message.MessageID,
			edit:       true,
			valid:      true,
		}

	case u.Message != nil && u.Message.Text != "":
		if !isPrivate(u.Message.Chat) {
			return intent{foreignChat: u.Message.Chat.ID}
		}
		cmd := parseText(u.Message.Text)
		cmd.ChatID = u.Message.Chat.ID
		return intent{command: cmd, messageID: u.Message.MessageID, valid: true}

	case u.ChannelPost != nil:
		return intent{foreignChat: u.ChannelPost.Chat.ID}
	}
	return intent{}
}

func isPrivate(c chat) bool { return c.Type == "" || c.Type == "private" }
func parseCallback(data string) core.Command {
	if code, ok := strings.CutPrefix(data, togglePrefix); ok && code != "" {
		return core.Command{Kind: core.CommandToggleTeam, Team: core.TeamCode(code)}
	}

	switch data {
	case menuData:
		return core.Command{Kind: core.CommandMenu}
	case teamsData:
		return core.Command{Kind: core.CommandTeams}
	case standingsData:
		return core.Command{Kind: core.CommandStandings}
	case alertsData:
		return core.Command{Kind: core.CommandToggleAlerts}
	}
	return core.Command{Kind: core.CommandUnknown}
}
func parseText(text string) core.Command {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return core.Command{Kind: core.CommandUnknown}
	}

	name, _, _ := strings.Cut(fields[0], "@")
	args := fields[1:]

	switch strings.ToLower(name) {
	case "/start":
		return core.Command{Kind: core.CommandStart}
	case "/menu":
		return core.Command{Kind: core.CommandMenu}
	case "/teams":
		return core.Command{Kind: core.CommandTeams}
	case "/standings", "/table":
		return core.Command{Kind: core.CommandStandings}
	case "/team":
		if len(args) == 0 {
			return core.Command{Kind: core.CommandTeams}
		}
		return core.Command{
			Kind: core.CommandToggleTeam,
			Team: core.TeamCode(strings.ToUpper(args[len(args)-1])),
		}
	case "/alerts":
		if len(args) == 0 {
			return core.Command{Kind: core.CommandUnknown}
		}
		switch strings.ToLower(args[0]) {
		case "on":
			return core.Command{Kind: core.CommandAlerts, Enable: true}
		case "off":
			return core.Command{Kind: core.CommandAlerts, Enable: false}
		}
		return core.Command{Kind: core.CommandUnknown}
	case "/stop":
		return core.Command{Kind: core.CommandStop}
	case "/help":
		return core.Command{Kind: core.CommandHelp}
	}
	return core.Command{Kind: core.CommandUnknown}
}
