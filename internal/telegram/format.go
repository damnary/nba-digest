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
		return "Привет! Я присылаю утренний дайджест матчей и уведомления по ходу игры.\n\n" +
			"Выбери команды, за которыми следишь:"

	case core.ReplyTeams:
		return selectionSummary(reply) + "\n\nНажми на команду, чтобы добавить или убрать её:"

	case core.ReplyTeamAdded:
		return fmt.Sprintf("Добавил %s.\n\n%s", reply.Team.Name, selectionSummary(reply))

	case core.ReplyTeamRemoved:
		return fmt.Sprintf("Убрал %s.\n\n%s", reply.Team.Name, selectionSummary(reply))

	case core.ReplyAlerts:
		if reply.Enabled {
			return "Уведомления по ходу матча включены."
		}
		return "Уведомления по ходу матча выключены. Утренний дайджест продолжит приходить."

	case core.ReplyStopped:
		return "Отписал от всего. Захочешь вернуться — просто напиши /start."

	case core.ReplyUnknownTeam:
		return "Не знаю такую команду. Посмотри список через /teams."

	default:
		return "Что я умею:\n" +
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
		names[i] = team.Name
	}
	return "Твои команды: " + strings.Join(names, ", ") + "."
}

func teamKeyboard(options []core.TeamOption) *keyboard {
	kb := &keyboard{}

	var row []button
	for _, option := range options {
		label := option.Team.Name
		if option.Selected {
			label = "✅ " + label
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
