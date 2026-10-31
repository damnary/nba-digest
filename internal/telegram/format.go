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

	switch reply.Kind {
	case core.ReplyMenu, core.ReplyAlerts, core.ReplyHelp:
		return text, mainMenu(reply)
	case core.ReplyStandings:
		return text, backOnly()
	}

	if len(reply.Teams) == 0 {
		return text, nil
	}
	return text, teamKeyboard(reply.Teams)
}

func mainMenu(reply core.Reply) *keyboard {
	alerts := "Уведомления: выкл"
	if reply.Enabled {
		alerts = "Уведомления: вкл"
	}

	return &keyboard{Rows: [][]button{
		{
			{Text: "Мои команды", Data: teamsData},
			{Text: "Таблица", Data: standingsData},
		},
		{{Text: alerts, Data: alertsData}},
	}}
}

func backOnly() *keyboard {
	return &keyboard{Rows: [][]button{{{Text: "‹ Назад", Data: menuData}}}}
}

func replyText(reply core.Reply) string {
	switch reply.Kind {
	case core.ReplyWelcome:
		return fmt.Sprintf("<b>Дайджест матчей %s</b>\n\n", reply.League.Title()) +
			"Каждое утро присылаю итоги ночных матчей: счёт, лучшие игроки, кто вытащил концовку.\n\n" +
			"Выбери команды, за которыми следишь:"

	case core.ReplyMenu:
		return "<b>Что показать?</b>\n\n" + followedLine(reply)

	case core.ReplyStandings:
		return standingsText(reply)

	case core.ReplyTeams:
		return selectionSummary(reply) + "\n\nНажми на команду, чтобы добавить или убрать её:"

	case core.ReplyTeamAdded:
		return fmt.Sprintf("Добавил <b>%s</b>\n\n%s", esc(reply.Team.Name), selectionSummary(reply))

	case core.ReplyTeamRemoved:
		return fmt.Sprintf("Убрал <b>%s</b>\n\n%s", esc(reply.Team.Name), selectionSummary(reply))

	case core.ReplyAlerts:
		if reply.Enabled {
			return "<b>Уведомления включены</b>\n\n" +
				"Напишу ночью, если у твоей команды концовка на волоске — " +
				"разница в пять очков или меньше за три минуты до сирены."
		}
		return "<b>Уведомления выключены</b>\n\nУтренний дайджест продолжит приходить."

	case core.ReplyStopped:
		return "<b>Отписал от всего</b>\n\nЗахочешь вернуться — напиши /start"

	case core.ReplyUnknownTeam:
		return "Не знаю такую команду.\n\nПосмотри список через /teams"

	default:
		return "<b>Что я умею</b>\n\n" +
			"Каждое утро присылаю итоги ночных матчей. " +
			"Всё остальное — кнопками ниже.\n\n" +
			"Отписаться от всего — /stop"
	}
}

func followedLine(reply core.Reply) string {
	if len(reply.Followed) == 0 {
		return "Ты пока не следишь ни за одной командой."
	}

	codes := make([]string, len(reply.Followed))
	for i, c := range reply.Followed {
		codes[i] = esc(string(c))
	}
	return "Твои команды: " + strings.Join(codes, ", ")
}

var conferenceNames = map[string]string{
	"Eastern Conference": "Восток",
	"Western Conference": "Запад",
}

func conferenceName(raw string) string {
	if ru, ok := conferenceNames[raw]; ok {
		return ru
	}
	return raw
}

func standingsText(reply core.Reply) string {
	if reply.Standings.IsEmpty() {
		return "<b>Таблица</b>\n\nСейчас таблица недоступна, попробуй позже."
	}

	followed := make(map[core.TeamCode]bool, len(reply.Followed))
	for _, c := range reply.Followed {
		followed[c] = true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Таблица %s</b>", reply.League.Title())

	for _, conf := range reply.Standings.Conferences {
		fmt.Fprintf(&b, "\n\n<b>%s</b>\n<blockquote>", esc(conferenceName(conf.Name)))

		for i, t := range conf.Teams {
			if cut := cutLabel(reply.League, t.Seed); cut != "" && i > 0 {
				fmt.Fprintf(&b, "\n<i>%s</i>", cut)
			}

			row := fmt.Sprintf("<code>%2d %-4s %2d-%-2d %3s</code>",
				t.Seed, esc(string(t.Team.Code)), t.Wins, t.Losses, esc(streak(t.Streak)))
			if followed[t.Team.Code] {
				row = "<b>" + row + "</b>"
			}

			prefix := "\n"
			if i == 0 {
				prefix = ""
			}
			fmt.Fprintf(&b, "%s%s%s", prefix, withSpace(icon(t.Team)), row)
		}
		b.WriteString("</blockquote>")
	}
	return b.String()
}

func cutLabel(league core.League, seed int) string {
	if league != core.LeagueNBA {
		return ""
	}
	switch seed {
	case 7:
		return "плей-ин"
	case 11:
		return "вне зоны"
	}
	return ""
}

func streak(raw string) string {
	if len(raw) < 2 {
		return ""
	}
	switch raw[0] {
	case 'W':
		return "+" + raw[1:]
	case 'L':
		return "−" + raw[1:]
	}
	return ""
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
