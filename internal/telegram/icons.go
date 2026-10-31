package telegram

import (
	"fmt"
	"strings"

	"github.com/damnary/nba-digest/internal/core"
)

var teamIcons = map[core.League]map[core.TeamCode]string{
	core.LeagueNBA: {
		"ATL":  "5330390620080984564",
		"BOS":  "5328223817670088640",
		"BKN":  "5330280436989973178",
		"CHA":  "5328045520692731293",
		"CHI":  "5327957619892053134",
		"CLE":  "5328083681477153886",
		"DAL":  "5327988908728802300",
		"DEN":  "5328014180316370335",
		"DET":  "5328204224029285487",
		"GS":   "5327879511616805165",
		"HOU":  "5328043484878231380",
		"IND":  "5330485040642018498",
		"LAC":  "5330237302633418382",
		"LAL":  "5328279742439249741",
		"MEM":  "5328317602575965655",
		"MIA":  "5328226669528372123",
		"MIL":  "5327768465237368036",
		"MIN":  "5330149775494891755",
		"NO":   "5327955038616704046",
		"NY":   "5328313711335595660",
		"OKC":  "5328044154893130223",
		"ORL":  "5327831661386161230",
		"PHI":  "5328170070449346361",
		"PHX":  "5328161841292007850",
		"POR":  "5330297406405758038",
		"SAC":  "5330385487595067040",
		"SA":   "5330403049716339074",
		"TOR":  "5327880830171765773",
		"UTAH": "5328220875617489739",
		"WSH":  "5330326470449468713",
	},
}

const (
	iconOpenTag  = "<tg-emoji "
	iconCloseTag = "</tg-emoji>"
)

func stripIcons(text string) string {
	var b strings.Builder
	for {
		open := strings.Index(text, iconOpenTag)
		if open < 0 {
			break
		}
		closed := strings.Index(text[open:], iconCloseTag)
		if closed < 0 {
			break
		}
		b.WriteString(text[:open])
		text = strings.TrimPrefix(text[open+closed+len(iconCloseTag):], " ")
	}
	b.WriteString(text)
	return b.String()
}

func icon(team core.Team) string {
	id, ok := teamIcons[team.League][team.Code]
	if !ok {
		return ""
	}
	return fmt.Sprintf("<tg-emoji emoji-id=%q>\U0001F3C0</tg-emoji>", id)
}
