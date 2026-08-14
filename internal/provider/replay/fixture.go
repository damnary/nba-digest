package replay

import (
	"fmt"
	"time"
)

type fixture struct {
	League   string     `json:"league"`
	GameID   string     `json:"game_id"`
	StartsAt time.Time  `json:"starts_at"`
	Home     teamJSON   `json:"home"`
	Away     teamJSON   `json:"away"`
	Plays    []playJSON `json:"plays"`
	BoxScore []lineJSON `json:"box_score"`
}

type teamJSON struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type playJSON struct {
	Seq           int    `json:"seq"`
	Period        int    `json:"period"`
	Clock         string `json:"clock"`
	Team          string `json:"team"`
	PlayerID      string `json:"player_id"`
	PlayerName    string `json:"player_name"`
	Points        int    `json:"points"`
	HomeScore     int    `json:"home_score"`
	AwayScore     int    `json:"away_score"`
	OffsetSeconds int    `json:"offset_seconds"`
}

type lineJSON struct {
	PlayerID string `json:"player_id"`
	Name     string `json:"name"`
	Team     string `json:"team"`
	Points   int    `json:"points"`
	Rebounds int    `json:"rebounds"`
	Assists  int    `json:"assists"`
}

func (f fixture) validate() error {
	switch {
	case f.League == "":
		return fmt.Errorf("league is empty")
	case f.GameID == "":
		return fmt.Errorf("game_id is empty")
	case f.Home.Code == "" || f.Away.Code == "":
		return fmt.Errorf("both teams must have a code")
	case len(f.Plays) == 0:
		return fmt.Errorf("no plays recorded")
	}

	prev := -1
	for i, p := range f.Plays {
		if p.OffsetSeconds < prev {
			return fmt.Errorf("play %d goes back in time", i)
		}
		prev = p.OffsetSeconds
	}
	return nil
}
