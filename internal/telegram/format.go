package telegram

import (
	"fmt"
	"strings"

	"github.com/damnary/nba-digest/internal/core"
)

const teamsPerRow = 2

type button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

type keyboard struct {
	Rows [][]button `json:"inline_keyboard"`
}

func render(reply core.Reply) (string, *keyboard) {
	text := replyText(reply)

	if len(reply.Teams) == 0 {
		return text, nil
	}
	return text, teamKeyboard(reply.Teams)
}

func replyText(reply core.Reply) string {
	switch reply.Kind {
	case core.ReplyWelcome:
		return "<b>Дайджест матчей WNBA</b>\n\n" +
			"Утром присылаю итоги вчерашних матчей, по ходу игры — уведомления о переломных моментах.\n\n" +
			"Выбери команды, за которыми следишь:"

	case core.ReplyTeams:
		return selectionSummary(reply) + "\n\nНажми на команду, чтобы добавить или убрать её:"

	case core.ReplyTeamAdded:
		return fmt.Sprintf("Добавил <b>%s</b>\n\n%s", esc(reply.Team.Name), selectionSummary(reply))

	case core.ReplyTeamRemoved:
		return fmt.Sprintf("Убрал <b>%s</b>\n\n%s", esc(reply.Team.Name), selectionSummary(reply))

	case core.ReplyAlerts:
		if reply.Enabled {
			return "<b>Уведомления включены</b>\n\nБуду писать по ходу матчей твоих команд."
		}
		return "<b>Уведомления выключены</b>\n\nУтренний дайджест продолжит приходить."

	case core.ReplyStopped:
		return "<b>Отписал от всего</b>\n\nЗахочешь вернуться — напиши /start"

	case core.ReplyUnknownTeam:
		return "Не знаю такую команду.\n\nПосмотри список через /teams"

	default:
		return "<b>Что я умею</b>\n\n" +
			"/teams — выбрать команды\n" +
			"/alerts on | off — уведомления по ходу матча\n" +
			"/stop — отписаться от всего"
	}
}

func selectionSummary(reply core.Reply) string {
	selected := reply.Selected()
	if len(selected) == 0 {
		return "Сейчас ты не следишь ни за одной командой."
	}

	names := make([]string, len(selected))
	for i, team := range selected {
		names[i] = esc(team.Name)
	}
	return "<b>Твои команды</b>\n" + strings.Join(names, "\n")
}

func teamKeyboard(options []core.TeamOption) *keyboard {
	kb := &keyboard{}

	var row []button
	for _, option := range options {
		label := option.Team.Name
		if option.Selected {
			label = "✓ " + label
		}
		row = append(row, button{Text: label, Data: togglePrefix + string(option.Team.Code)})

		if len(row) == teamsPerRow {
			kb.Rows = append(kb.Rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		kb.Rows = append(kb.Rows, row)
	}
	return kb
}
